package workspace_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/badge"
	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/gittest"
	"github.com/epicentre/gitdrift/internal/ops"
	"github.com/epicentre/gitdrift/internal/repo"
	"github.com/epicentre/gitdrift/internal/workspace"
)

// pushedClone creates a clone of a fresh origin with one pushed commit and a
// teammate clone of the same remote.
func pushedClone(t *testing.T, env *gittest.Env, name string) (clone, mate *gittest.Repo) {
	t.Helper()

	remote := env.Remote(name + "-origin.git")
	clone = env.Clone(remote, name)
	clone.Commit("README.md", "hello\n", "initial")
	clone.Git("push", "--quiet", "-u", "origin", "main")

	mate = env.Clone(remote, name+"-mate")

	return clone, mate
}

func discover(t *testing.T, env *gittest.Env) []workspace.Group {
	t.Helper()

	groups, err := workspace.Discover(t.Context(), env.Runner(), env.WorkDir(), workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	return groups
}

func newWorkspace(t *testing.T, env *gittest.Env) *workspace.Workspace {
	t.Helper()

	return workspace.New(env.Runner(), discover(t, env), workspace.DefaultOptions())
}

func TestDefaultOptions(t *testing.T) {
	t.Parallel()

	want := workspace.Options{Concurrency: 8, FetchTimeout: 60 * time.Second}
	if got := workspace.DefaultOptions(); got != want {
		t.Fatalf("DefaultOptions = %+v, want %+v", got, want)
	}
}

func TestRefreshFetchesAllRemotes(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, _ := pushedClone(t, env, "payments")

	upstream := env.Remote("upstream.git")
	clone.Git("push", "--quiet", upstream, "main")
	clone.Git("remote", "add", "upstream", upstream)
	clone.Git("fetch", "--quiet", "upstream")
	clone.Git("config", "--local", "gitdrift.base", "upstream/main")

	other := env.Clone(upstream, "upstream-mate")
	other.Commit("one.txt", "1", "one")
	other.Commit("two.txt", "2", "two")
	other.Git("push", "--quiet", "origin", "main")

	w := newWorkspace(t, env)
	g := w.Groups()[0]

	snaps := w.Refresh(t.Context(), g.CommonDir, true)
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}

	s := snaps[0]
	if s.Err != nil {
		t.Fatal(s.Err)
	}

	if s.Base.Ref != "upstream/main" || s.Base.Behind != 2 {
		t.Fatalf("Base = %+v, want upstream/main behind 2", s.Base)
	}

	if !s.Fetch.Done || len(s.Fetch.Errors) != 0 {
		t.Fatalf("Fetch = %+v, want done without errors", s.Fetch)
	}
}

func TestRefreshUnreachableRemote(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, mate := pushedClone(t, env, "payments")
	missing := filepath.Join(env.WorkDir(), "no-such-remote")
	clone.Git("remote", "add", "broken", missing)
	mate.Commit("new.txt", "x", "teammate commit")
	mate.Git("push", "--quiet", "origin", "main")

	w := newWorkspace(t, env)

	start := time.Now()
	snaps := w.Refresh(t.Context(), w.Groups()[0].CommonDir, true)

	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("Refresh took %s", d)
	}

	// The status line shows git's own fatal line, not the wrapped error.
	s := snaps[0]
	want := map[string]string{"broken": "fatal: '" + missing + "' does not appear to be a git repository"}

	if diff := cmp.Diff(want, s.Fetch.Errors); diff != "" {
		t.Fatalf("Fetch.Errors mismatch (-want +got):\n%s", diff)
	}

	if !slices.Contains(drift.AttentionReasons(s), "fetch failed: broken") {
		t.Fatalf("AttentionReasons = %v", drift.AttentionReasons(s))
	}

	if s.Status.Behind != 1 {
		t.Fatalf("Behind = %d, want 1", s.Status.Behind)
	}

	if got := drift.PullBlockers(s); len(got) != 0 {
		t.Fatalf("PullBlockers = %v, want none", got)
	}
}

func TestRefreshConcurrencyLimit(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	for i := range 6 {
		pushedClone(t, env, "repo"+strconv.Itoa(i))
	}

	opts := workspace.DefaultOptions()
	opts.Concurrency = 2
	w := workspace.New(env.Runner(), discover(t, env), opts)

	var (
		mu      sync.Mutex
		maxSeen int
	)

	workspace.SetOnAcquire(w, func(active int) {
		mu.Lock()
		maxSeen = max(maxSeen, active)
		mu.Unlock()

		time.Sleep(50 * time.Millisecond)
	})

	var wg sync.WaitGroup

	for _, g := range w.Groups() {
		wg.Go(func() { w.Refresh(t.Context(), g.CommonDir, false) })
	}

	wg.Wait()

	mu.Lock()
	got := maxSeen
	mu.Unlock()

	if got != 2 {
		t.Fatalf("max concurrent refreshes = %d, want 2", got)
	}
}

func TestRefreshWorktreeGroup(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, _ := pushedClone(t, env, "payments")
	clone.Git("branch", "feature/checkout")
	clone.Git("worktree", "add", "--quiet", filepath.Join(env.WorkDir(), "payments-wt"), "feature/checkout")

	w := newWorkspace(t, env)

	var group workspace.Group

	for _, g := range w.Groups() {
		if len(g.Members) == 2 {
			group = g
		}
	}

	if len(group.Members) != 2 {
		t.Fatalf("no two-member group in %+v", w.Groups())
	}

	snaps := w.Refresh(t.Context(), group.CommonDir, false)
	if len(snaps) != 2 {
		t.Fatalf("snapshots = %d, want 2", len(snaps))
	}

	if diff := cmp.Diff(snaps[0].Work, snaps[1].Work); diff != "" {
		t.Fatalf("Work differs (-first +second):\n%s", diff)
	}

	branches := []string{snaps[0].Status.Branch, snaps[1].Status.Branch}
	if !slices.Equal(branches, []string{"main", "feature/checkout"}) {
		t.Fatalf("branches = %v", branches)
	}
}

func TestRefreshBadges(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	_, mate := pushedClone(t, env, "payments")
	mate.Commit("go.sum", "", "bump deps")
	mate.Git("push", "--quiet", "origin", "main")

	w := newWorkspace(t, env)

	snaps := w.Refresh(t.Context(), w.Groups()[0].CommonDir, true)
	if snaps[0].Err != nil {
		t.Fatal(snaps[0].Err)
	}

	if diff := cmp.Diff([]string{"deps"}, snaps[0].Badges); diff != "" {
		t.Fatalf("Badges (-want +got):\n%s", diff)
	}
}

// TestRefreshLocalChanges checks that a refresh reads the incoming files, so
// that local changes stop a pull only when it would change their files.
func TestRefreshLocalChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		incoming     string
		wantBlockers []string
		wantCategory drift.Category
	}{
		{"outside incoming files", "go.sum", nil, drift.CategoryBehind},
		{"to an incoming file", "README.md", []string{"local changes to README.md"}, drift.CategoryNeedsAttention},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := gittest.New(t)
			clone, mate := pushedClone(t, env, "payments")
			mate.Commit(tt.incoming, "remote\n", "teammate commit")
			mate.Git("push", "--quiet", "origin", "main")
			clone.Write("README.md", "edited\n")

			w := newWorkspace(t, env)

			s := w.Refresh(t.Context(), w.Groups()[0].CommonDir, true)[0]
			if s.Err != nil {
				t.Fatal(s.Err)
			}

			if !s.IncomingRead || !slices.Equal(s.Incoming, []string{tt.incoming}) {
				t.Fatalf("Incoming = %q (read %v), want [%s]", s.Incoming, s.IncomingRead, tt.incoming)
			}

			if diff := cmp.Diff(tt.wantBlockers, drift.PullBlockers(s)); diff != "" {
				t.Errorf("PullBlockers mismatch (-want +got):\n%s", diff)
			}

			if got := drift.CategoryOf(s); got != tt.wantCategory {
				t.Errorf("CategoryOf = %v, want %v", got, tt.wantCategory)
			}
		})
	}
}

func TestRefreshBadgeRules(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, _ := pushedClone(t, env, "payments")
	clone.Git("config", "--local", "gitdrift.badge", "api=openapi.yaml")
	clone.Git("worktree", "add", "--quiet", "-b", "feature/checkout", filepath.Join(env.WorkDir(), "payments-wt"))

	w := newWorkspace(t, env)

	grp := w.Groups()[0]
	if len(grp.Members) != 2 {
		t.Fatalf("members = %+v, want the clone and its worktree", grp.Members)
	}

	want := append(badge.Defaults(), badge.Rule{Label: "api", Name: "openapi.yaml"})

	for _, s := range w.Refresh(t.Context(), grp.CommonDir, false) {
		if s.Err != nil {
			t.Fatalf("%s: %v", s.Loc.Name, s.Err)
		}

		if s.Status.Behind != 0 {
			t.Fatalf("%s is behind; the rules must be read without incoming commits", s.Loc.Name)
		}

		if diff := cmp.Diff(want, s.BadgeRules); diff != "" {
			t.Errorf("%s BadgeRules (-want +got):\n%s", s.Loc.Name, diff)
		}
	}
}

func TestRefreshWithoutFetchKeepsFetchState(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	pushedClone(t, env, "payments")
	w := newWorkspace(t, env)
	dir := w.Groups()[0].CommonDir

	first := w.Refresh(t.Context(), dir, true)[0].Fetch
	second := w.Refresh(t.Context(), dir, false)[0].Fetch

	if !first.Done {
		t.Fatalf("first Fetch = %+v, want done", first)
	}

	if diff := cmp.Diff(first, second); diff != "" {
		t.Fatalf("Fetch changed (-first +second):\n%s", diff)
	}
}

func TestRefreshNoRemotesIsDone(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	initRepo(t, env, "solo")
	w := newWorkspace(t, env)

	s := w.Refresh(t.Context(), w.Groups()[0].CommonDir, true)[0]
	if !s.Fetch.Done {
		t.Fatalf("Fetch = %+v, want done", s.Fetch)
	}
}

func TestWritesWaitForRefresh(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, _ := pushedClone(t, env, "payments")
	w := newWorkspace(t, env)
	g := w.Groups()[0]
	loc := g.Members[0]

	held := make(chan struct{})
	release := make(chan struct{})

	var releaseOnce sync.Once

	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)

	workspace.SetOnAcquire(w, func(int) {
		close(held)
		<-release
	})

	refreshed := make(chan struct{})

	go func() {
		defer close(refreshed)

		w.Refresh(t.Context(), g.CommonDir, false)
	}()

	<-held

	// A cancelled write returns the context error and changes nothing.
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	for range 20 {
		if err := w.SetBase(ctx, loc, "origin/dev"); !errorsIsDeadline(err) {
			t.Fatalf("SetBase err = %v, want deadline exceeded", err)
		}
	}

	cancelled, stop := context.WithCancel(t.Context())
	stop()

	for range 20 {
		if err := w.SetBase(cancelled, loc, "origin/dev"); !errors.Is(err, context.Canceled) {
			t.Fatalf("SetBase err = %v, want canceled", err)
		}
	}

	if got, _ := clone.TryGit("config", "--local", "gitdrift.base"); got != "" {
		t.Fatalf("base = %q after cancelled SetBase, want unset", got)
	}

	// A patient write waits for the refresh and runs after it.
	done := make(chan error, 1)

	go func() { done <- w.SetBase(t.Context(), loc, "origin/main") }()

	select {
	case err := <-done:
		t.Fatalf("SetBase finished while refresh held the lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	unblock()

	if err := <-done; err != nil {
		t.Fatal(err)
	}

	<-refreshed

	if got := clone.Git("config", "--local", "gitdrift.base"); got != "origin/main" {
		t.Fatalf("base = %q, want origin/main", got)
	}
}

func TestWritesUnknownGroup(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	pushedClone(t, env, "payments")
	w := newWorkspace(t, env)
	stray := repo.Location{Path: "/nowhere", CommonDir: "/nowhere/.git"}

	if err := w.SetBase(t.Context(), stray, "x"); err == nil {
		t.Fatal("SetBase on unknown group succeeded")
	}

	if err := w.ResetBase(t.Context(), stray); err == nil {
		t.Fatal("ResetBase on unknown group succeeded")
	}

	if out := w.Pull(t.Context(), ops.PullRequest{Loc: stray}); out.Err == nil {
		t.Fatal("Pull on unknown group succeeded")
	}

	if out := w.DeleteGone(t.Context(), ops.DeleteRequest{Loc: stray, Branch: "b", SHA: "s"}); out.Err == nil ||
		out.Branch != "b" {
		t.Fatalf("DeleteGone outcome = %+v", out)
	}
}

func TestBackendReads(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, _ := pushedClone(t, env, "payments")
	sha := clone.Commit("a.txt", "a", "add a")
	w := newWorkspace(t, env)
	loc := w.Groups()[0].Members[0]

	spec, err := w.CommitSpec(t.Context(), loc, sha)
	if err != nil {
		t.Fatal(err)
	}

	files, err := w.Files(t.Context(), loc, spec)
	if err != nil || len(files) != 1 || files[0].Path != "a.txt" {
		t.Fatalf("Files = %+v, %v", files, err)
	}

	commits, total, err := w.RangeCommits(t.Context(), loc, "origin/main", "HEAD", 10)
	if err != nil || total != 1 || len(commits) != 1 {
		t.Fatalf("RangeCommits = %+v, %d, %v", commits, total, err)
	}

	if _, _, err := w.FileDiff(t.Context(), loc, spec, "a.txt", "", 1<<20); err != nil {
		t.Fatal(err)
	}

	branches, err := w.RemoteBranches(t.Context(), loc)
	if err != nil || !slices.Contains(branches, "origin/main") {
		t.Fatalf("RemoteBranches = %v, %v", branches, err)
	}

	only, err := w.OnlyHereCommits(t.Context(), loc, "main", 10)
	if err != nil || len(only) != 1 {
		t.Fatalf("OnlyHereCommits = %+v, %v", only, err)
	}
}

func errorsIsDeadline(err error) bool { return errors.Is(err, context.DeadlineExceeded) }

func removeDir(t *testing.T, path string) {
	t.Helper()

	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshSurvivesRemovedFirstMember(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, mate := pushedClone(t, env, "payments")
	clone.Git("worktree", "add", "--quiet", "-b", "aaa", filepath.Join(env.WorkDir(), "aaa-wt"))
	mate.Commit("new.txt", "x", "teammate commit")
	mate.Git("push", "--quiet", "origin", "main")

	w := newWorkspace(t, env)

	grp := w.Groups()[0]
	if len(grp.Members) != 2 || grp.Members[0].Name != "aaa-wt" {
		t.Fatalf("unexpected members %+v", grp.Members)
	}

	removeDir(t, grp.Members[0].Path)

	snaps := w.Refresh(t.Context(), grp.CommonDir, true)
	if len(snaps) != 2 {
		t.Fatalf("snapshots = %d, want 2", len(snaps))
	}

	if snaps[0].Err == nil {
		t.Fatal("removed member has no error")
	}

	healthy := snaps[1]
	if healthy.Err != nil {
		t.Fatalf("healthy member Err = %v", healthy.Err)
	}

	if healthy.Status.Behind != 1 || !healthy.Fetch.OK("origin") {
		t.Fatalf("healthy member Behind = %d, Fetch = %+v", healthy.Status.Behind, healthy.Fetch)
	}
}

func TestRefreshNoMemberListsRemotes(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, _ := pushedClone(t, env, "payments")
	w := newWorkspace(t, env)

	removeDir(t, clone.Path)

	for _, s := range w.Refresh(t.Context(), w.Groups()[0].CommonDir, true) {
		if s.Fetch.Done || s.Fetch.OK("origin") {
			t.Fatalf("Fetch = %+v, want not done", s.Fetch)
		}
	}

	// The skipped fetch is not remembered as a success either.
	for _, s := range w.Refresh(t.Context(), w.Groups()[0].CommonDir, false) {
		if s.Fetch.Done {
			t.Fatalf("later Fetch = %+v, want not done", s.Fetch)
		}
	}
}

func TestNewNormalizesOptions(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, _ := pushedClone(t, env, "payments")

	found := discover(t, env)

	w := workspace.New(env.Runner(), append(slices.Clone(found), workspace.Group{CommonDir: "/empty"}),
		workspace.Options{})
	if len(w.Groups()) != len(found) {
		t.Fatalf("groups = %d, want %d (empty group dropped)", len(w.Groups()), len(found))
	}

	s := w.Refresh(t.Context(), clone.Git("rev-parse", "--path-format=absolute", "--git-common-dir"), true)[0]
	if s.Err != nil || len(s.Fetch.Errors) != 0 {
		t.Fatalf("zero options: Err = %v, Fetch = %+v", s.Err, s.Fetch)
	}
}

func TestRefreshFetchTimeout(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, _ := pushedClone(t, env, "payments")

	script := filepath.Join(t.TempDir(), "slow-upload-pack")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 5\nexec git upload-pack \"$@\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(script, 0o700); err != nil { //nolint:gosec // the test script must be executable
		t.Fatal(err)
	}

	clone.Git("config", "--local", "remote.origin.uploadpack", script)

	opts := workspace.DefaultOptions()
	opts.FetchTimeout = time.Second
	w := workspace.New(env.Runner(), discover(t, env), opts)

	start := time.Now()
	s := w.Refresh(t.Context(), w.Groups()[0].CommonDir, true)[0]

	if d := time.Since(start); d > 4*time.Second {
		t.Fatalf("Refresh took %s, want the 1s fetch timeout to apply", d)
	}

	if _, ok := s.Fetch.Errors["origin"]; !ok {
		t.Fatalf("Fetch.Errors = %v, want key origin", s.Fetch.Errors)
	}
}

func TestRefreshRefusedRemote(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, mate := pushedClone(t, env, "payments")
	clone.Git("config", "--add", "remote.origin.fetch", "+refs/heads/*:refs/heads/*")
	mate.Commit("new.txt", "x", "teammate commit")
	mate.Git("push", "--quiet", "origin", "main")

	w := newWorkspace(t, env)
	s := w.Refresh(t.Context(), w.Groups()[0].CommonDir, true)[0]

	refusal := (&ops.UnsafeRefspecError{Remote: "origin", Refspec: "+refs/heads/*:refs/heads/*"}).Error()
	if diff := cmp.Diff(map[string]string{"origin": refusal}, s.Fetch.Errors); diff != "" {
		t.Fatalf("Fetch.Errors mismatch (-want +got):\n%s", diff)
	}

	if got := drift.PullBlockers(s); !slices.Contains(got, "fetch failed: origin") {
		t.Fatalf("PullBlockers = %v, want fetch failed: origin", got)
	}

	if s.Status.Behind != 0 {
		// The refused fetch must not have moved anything.
		t.Fatalf("Behind = %d, want 0 (nothing fetched)", s.Status.Behind)
	}
}

func TestRefreshKeepsFetchErrors(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, _ := pushedClone(t, env, "payments")
	clone.Git("remote", "add", "broken", filepath.Join(env.WorkDir(), "no-such-remote"))

	w := newWorkspace(t, env)
	dir := w.Groups()[0].CommonDir

	first := w.Refresh(t.Context(), dir, true)[0].Fetch
	second := w.Refresh(t.Context(), dir, false)[0].Fetch

	if _, ok := first.Errors["broken"]; !ok {
		t.Fatalf("first Fetch = %+v, want error for broken", first)
	}

	if diff := cmp.Diff(first, second); diff != "" {
		t.Fatalf("Fetch changed (-first +second):\n%s", diff)
	}
}

// recordingGit returns a runner whose git only notes in a file next to it
// that it ran, and the path of that file.
func recordingGit(t *testing.T) (r *git.Runner, calls string) {
	t.Helper()

	dir := t.TempDir()
	script := filepath.Join(dir, "git")

	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$@\" >> \"$(dirname \"$0\")/calls\"\nexit 1\n"),
		0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(script, 0o700); err != nil { //nolint:gosec // the test script must be executable
		t.Fatal(err)
	}

	return &git.Runner{Binary: script}, filepath.Join(dir, "calls")
}

func TestCloseRefusesNewCalls(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	pushedClone(t, env, "payments")

	fake, calls := recordingGit(t)
	groups := discover(t, env)
	w := workspace.New(fake, groups, workspace.DefaultOptions())
	loc := w.Groups()[0].Members[0]
	ctx := t.Context()

	w.Close()

	if snaps := w.Refresh(ctx, loc.CommonDir, true); snaps != nil {
		t.Errorf("Refresh after Close = %+v, want no snapshots", snaps)
	}

	deleted := w.DeleteGone(ctx, ops.DeleteRequest{Loc: loc, Branch: "feature/checkout", SHA: "abc"})
	if deleted.Branch != "feature/checkout" || deleted.SHA != "abc" {
		t.Errorf("DeleteGone outcome = %+v, want the request's branch and sha", deleted)
	}

	_, _, rangeErr := w.RangeCommits(ctx, loc, "origin/main", "HEAD", 10)
	_, onlyHereErr := w.OnlyHereCommits(ctx, loc, "main", 10)
	_, specErr := w.CommitSpec(ctx, loc, "HEAD")
	_, filesErr := w.Files(ctx, loc, repo.DiffSpec{})
	_, _, diffErr := w.FileDiff(ctx, loc, repo.DiffSpec{}, "README.md", "", 1<<20)
	_, branchesErr := w.RemoteBranches(ctx, loc)

	for _, c := range []struct {
		name string
		err  error
	}{
		{"Pull", w.Pull(ctx, ops.PullRequest{Loc: loc}).Err},
		{"DeleteGone", deleted.Err},
		{"SetBase", w.SetBase(ctx, loc, "origin/main")},
		{"ResetBase", w.ResetBase(ctx, loc)},
		{"RangeCommits", rangeErr},
		{"OnlyHereCommits", onlyHereErr},
		{"CommitSpec", specErr},
		{"Files", filesErr},
		{"FileDiff", diffErr},
		{"RemoteBranches", branchesErr},
	} {
		if !errors.Is(c.err, workspace.ErrClosed) {
			t.Errorf("%s after Close: err = %v, want ErrClosed", c.name, c.err)
		}
	}

	if out, err := os.ReadFile(calls); !errors.Is(err, os.ErrNotExist) { //nolint:gosec // calls is a file in the test's temp dir
		t.Errorf("git ran after Close: %q (%v)", out, err)
	}

	if got := len(w.Groups()); got != len(groups) {
		t.Errorf("Groups after Close = %d, want %d", got, len(groups))
	}
}

// waitUntilClosed waits until a new call on w reports ErrClosed. Each try is
// a SetBase that, while the workspace is open, waits for the group lock the
// blocked refresh holds and gives up without running git.
func waitUntilClosed(t *testing.T, w *workspace.Workspace, loc repo.Location) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		err := w.SetBase(ctx, loc, "origin/dev")

		cancel()

		switch {
		case errors.Is(err, workspace.ErrClosed):
			return
		case !errors.Is(err, context.DeadlineExceeded):
			t.Fatalf("SetBase while a refresh holds the lock: err = %v", err)
		}
	}

	t.Fatal("new calls still run after Close started")
}

func TestCloseWaitsForCallsInFlight(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, _ := pushedClone(t, env, "payments")
	w := newWorkspace(t, env)
	g := w.Groups()[0]

	held := make(chan struct{})
	release := make(chan struct{})

	var releaseOnce sync.Once

	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)

	workspace.SetOnAcquire(w, func(int) {
		close(held)
		<-release
	})

	refreshed := make(chan []drift.Snapshot, 1)

	go func() { refreshed <- w.Refresh(t.Context(), g.CommonDir, false) }()

	<-held

	// Several concurrent Close calls all wait for the refresh.
	closed := make(chan struct{})

	go func() {
		defer close(closed)

		var closers sync.WaitGroup
		for range 3 {
			closers.Go(w.Close)
		}

		closers.Wait()
	}()

	waitUntilClosed(t, w, g.Members[0])

	select {
	case <-closed:
		t.Fatal("Close returned while a refresh was in flight")
	case <-time.After(150 * time.Millisecond):
	}

	unblock()

	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return after the refresh finished")
	}

	// The call in flight ran to its end.
	snaps := <-refreshed
	if len(snaps) != 1 || snaps[0].Err != nil || snaps[0].Status.Branch != "main" {
		t.Fatalf("in-flight Refresh = %+v, want one healthy snapshot", snaps)
	}

	if got, _ := clone.TryGit("config", "--local", "gitdrift.base"); got != "" {
		t.Fatalf("base = %q, want unset: no write may run after Close", got)
	}

	w.Close() // idempotent: returns at once
}

// waitForFile waits until path exists.
func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)

	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("%s never appeared", path)
}

// TestCloseWaitsForPull quits while a pull's merge runs: cancelling the
// caller does not stop the merge, and Close returns only once it is done.
func TestCloseWaitsForPull(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone, mate := pushedClone(t, env, "payments")
	mate.Commit("new.txt", "x", "teammate commit")
	mate.Git("push", "--quiet", "origin", "main")

	w := newWorkspace(t, env)

	snap := w.Refresh(t.Context(), w.Groups()[0].CommonDir, true)[0]
	if snap.Err != nil || snap.Status.Behind != 1 {
		t.Fatalf("snapshot Err = %v, Behind = %d, want 1 behind", snap.Err, snap.Status.Behind)
	}

	hooks, marks := t.TempDir(), t.TempDir()
	started, finished := filepath.Join(marks, "started"), filepath.Join(marks, "finished")
	script := "#!/bin/sh\ntouch '" + started + "'\nsleep 1\ntouch '" + finished + "'\n"

	if err := os.WriteFile(filepath.Join(hooks, "post-merge"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(filepath.Join(hooks, "post-merge"), 0o700); err != nil { //nolint:gosec // the hook must be executable
		t.Fatal(err)
	}

	clone.Git("config", "--local", "core.hooksPath", hooks)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	req := ops.PullRequest{
		Loc: snap.Loc, Branch: snap.Status.Branch, HeadSHA: snap.Status.HeadSHA, UpstreamSHA: snap.Status.UpstreamSHA, Fetch: snap.Fetch,
	}
	pulled := make(chan ops.PullOutcome, 1)

	go func() { pulled <- w.Pull(ctx, req) }()

	waitForFile(t, started)
	cancel()
	w.Close()

	if _, err := os.Stat(finished); err != nil {
		t.Fatalf("Close returned while the merge was still running: %v", err)
	}

	if out := <-pulled; out.Err != nil || out.Skipped != "" || out.Pulled != 1 {
		t.Fatalf("Pull outcome = %+v, want 1 pulled", out)
	}

	if head, upstream := clone.Git("rev-parse", "HEAD"), clone.Git("rev-parse", "@{upstream}"); head != upstream {
		t.Fatalf("HEAD = %s, want the upstream %s", head, upstream)
	}
}
