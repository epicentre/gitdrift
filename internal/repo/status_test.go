package repo_test

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/gittest"
	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	mainRef    = "refs/heads/main"
	mainBranch = "main"
)

func readStatus(t *testing.T, env *gittest.Env, r *gittest.Repo) repo.Status {
	t.Helper()

	g := env.Runner()

	loc, err := repo.Locate(t.Context(), g, r.Path)
	if err != nil {
		t.Fatal(err)
	}

	st, err := repo.ReadStatus(t.Context(), g, loc)
	if err != nil {
		t.Fatal(err)
	}

	return st
}

// teammate returns a second clone of remote that commits and pushes n files.
func teammate(t *testing.T, env *gittest.Env, remote string, n int) string {
	t.Helper()

	mate := env.Clone(remote, "teammate")

	var last string

	for i := range n {
		last = mate.Commit("mate"+string(rune('a'+i))+".txt", "x", "teammate commit")
	}

	mate.Git("push", "--quiet", "origin", "main")

	return last
}

func TestReadStatusClean(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	remote := env.Remote("origin.git")
	r := env.Clone(remote, "payments")
	when := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	sha := r.CommitAt("README.md", "hello\n", "initial", when)
	r.Git("push", "--quiet", "-u", "origin", "main")

	got := readStatus(t, env, r)
	want := repo.Status{
		Branch:            mainBranch,
		HeadSHA:           sha,
		Upstream:          "origin/main",
		UpstreamRef:       "refs/remotes/origin/main",
		UpstreamRemote:    originName,
		UpstreamRemoteRef: mainRef,
		UpstreamSHA:       sha,
		Remotes:           []string{"origin"},
		LastUpdate:        when,
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("status mismatch (-want +got):\n%s", diff)
	}
}

func TestReadStatusAheadBehind(t *testing.T) {
	t.Parallel()

	env, remote, r := newClone(t, "payments")
	last := teammate(t, env, remote, 2)

	r.Commit("local.txt", "x", "local")
	r.Git("fetch", "--quiet")

	got := readStatus(t, env, r)
	if got.Ahead != 1 || got.Behind != 2 {
		t.Errorf("Ahead = %d, Behind = %d, want 1 and 2", got.Ahead, got.Behind)
	}

	if got.UpstreamSHA != last {
		t.Errorf("UpstreamSHA = %q, want %q", got.UpstreamSHA, last)
	}
}

func TestReadStatusLocalChanges(t *testing.T) {
	t.Parallel()

	env, _, r := newClone(t, "payments")
	r.Write(".gitignore", "*.log\n")
	r.Git("add", ".gitignore")
	r.Git("commit", "--quiet", "-m", "ignore logs")

	r.Write("README.md", "changed\n")
	r.Write("staged.txt", "new\n")
	r.Git("add", "staged.txt")
	r.Write("untracked.txt", "u\n")
	r.Write("debug.log", "ignored\n")

	got := readStatus(t, env, r)
	if got.Tracked != 2 || got.Untracked != 1 {
		t.Errorf("Tracked = %d, Untracked = %d, want 2 and 1", got.Tracked, got.Untracked)
	}
}

func TestReadStatusRenameCountsOnce(t *testing.T) {
	t.Parallel()

	env, _, r := newClone(t, "payments")
	r.Git("mv", "README.md", "RENAMED.md")

	got := readStatus(t, env, r)
	if got.Tracked != 1 || got.Untracked != 0 {
		t.Errorf("Tracked = %d, Untracked = %d, want 1 and 0", got.Tracked, got.Untracked)
	}
}

func TestReadStatusDetached(t *testing.T) {
	t.Parallel()

	env, _, r := newClone(t, "payments")
	sha := r.Git("rev-parse", "HEAD")
	r.Git("checkout", "--quiet", "--detach")

	got := readStatus(t, env, r)
	if !got.Detached || got.Branch != "" || got.HeadSHA != sha || got.Upstream != "" {
		t.Errorf("status = %+v", got)
	}
}

func TestReadStatusNoUpstream(t *testing.T) {
	t.Parallel()

	env, _, r := newClone(t, "payments")
	r.Git("checkout", "--quiet", "-b", "feature/local")

	got := readStatus(t, env, r)
	if got.Branch != "feature/local" || got.Upstream != "" || got.UpstreamGone || got.UpstreamSHA != "" {
		t.Errorf("status = %+v", got)
	}
}

func TestReadStatusGoneUpstream(t *testing.T) {
	t.Parallel()

	env, _, r := newClone(t, "payments")
	r.Git("checkout", "--quiet", "-b", "feature/x")
	r.Commit("x.txt", "x", "work")
	r.Git("push", "--quiet", "-u", "origin", "feature/x")
	r.Git("push", "--quiet", "origin", "--delete", "feature/x")
	r.Git("fetch", "--quiet", "--prune")

	got := readStatus(t, env, r)
	if !got.UpstreamGone || got.Upstream != "origin/feature/x" || got.Ahead != 0 || got.Behind != 0 {
		t.Errorf("status = %+v", got)
	}

	if got.UpstreamRef != "refs/remotes/origin/feature/x" || got.UpstreamSHA != "" {
		t.Errorf("UpstreamRef = %q, UpstreamSHA = %q", got.UpstreamRef, got.UpstreamSHA)
	}

	if got.LastUpdate.IsZero() {
		t.Error("LastUpdate is zero; want the HEAD commit time")
	}
}

func TestReadStatusOperation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(r *gittest.Repo)
		want  repo.Operation
	}{
		{"merge", func(r *gittest.Repo) {
			conflict(r)
			conflictGit(r, "merge", "other")
		}, repo.OpMerge},
		{"rebase", func(r *gittest.Repo) {
			conflict(r)
			r.Git("checkout", "--quiet", "other")
			conflictGit(r, "rebase", "main")
		}, repo.OpRebase},
		{"cherry-pick", func(r *gittest.Repo) {
			conflict(r)
			conflictGit(r, "cherry-pick", "other")
		}, repo.OpCherryPick},
		{"revert", func(r *gittest.Repo) {
			r.Commit("f.txt", "two\n", "change")
			r.Commit("f.txt", "three\n", "change again")
			conflictGit(r, "revert", "HEAD~1")
		}, repo.OpRevert},
		{"bisect", func(r *gittest.Repo) {
			r.Commit("f.txt", "two\n", "second")
			r.Git("bisect", "start")
		}, repo.OpBisect},
		{"none", func(*gittest.Repo) {}, repo.OpNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env, _, r := newClone(t, "payments")
			r.Commit("f.txt", "one\n", "add f")
			tt.setup(r)

			if got := readStatus(t, env, r); got.Operation != tt.want {
				t.Errorf("Operation = %q, want %q", got.Operation, tt.want)
			}
		})
	}
}

// conflictGit runs a git command that is expected to stop on a conflict; the
// test asserts the resulting Operation, so the error itself is not needed.
func conflictGit(r *gittest.Repo, args ...string) {
	_, _ = r.TryGit(args...)
}

// conflict leaves main and other with conflicting edits of f.txt; main is
// checked out.
func conflict(r *gittest.Repo) {
	r.Git("checkout", "--quiet", "-b", "other")
	r.Commit("f.txt", "other side\n", "other edit")
	r.Git("checkout", "--quiet", "main")
	r.Commit("f.txt", "main side\n", "main edit")
}

func TestReadStatusUnborn(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	r := env.Init("fresh")

	got := readStatus(t, env, r)
	if !got.Unborn || got.Branch != "main" || got.HeadSHA != "" || got.Detached {
		t.Errorf("status = %+v", got)
	}

	if !got.LastUpdate.IsZero() {
		t.Errorf("LastUpdate = %v, want zero", got.LastUpdate)
	}
}

func TestReadStatusLocalUpstream(t *testing.T) {
	t.Parallel()

	env, _, r := newClone(t, "payments")
	r.Git("branch", "--track", "topic", "main")
	r.Commit("extra.txt", "x", "extra on main")
	r.Git("checkout", "--quiet", "topic")

	got := readStatus(t, env, r)
	if got.UpstreamRemote != "." || got.UpstreamGone || got.Behind != 1 || got.Ahead != 0 {
		t.Errorf("status = %+v", got)
	}

	if got.Upstream != "main" || got.UpstreamRef != mainRef || got.UpstreamRemoteRef != mainRef {
		t.Errorf("upstream fields = %+v", got)
	}
}

func TestReadStatusIgnoresUserConfig(t *testing.T) {
	t.Parallel()

	env, _, r := newClone(t, "payments")
	r.Git("config", "status.showUntrackedFiles", "no")
	r.Git("config", "color.ui", "always")
	r.Git("config", "core.quotepath", "true")
	r.Write("café.txt", "x")

	got := readStatus(t, env, r)
	if got.Untracked != 1 {
		t.Errorf("Untracked = %d, want 1", got.Untracked)
	}
}

func TestReadStatusUnbornWithUpstream(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	remote := env.Remote("origin.git")
	r := env.Clone(remote, "payments") // empty remote: the branch is unborn
	last := teammate(t, env, remote, 1)

	r.Git("fetch", "--quiet")
	r.Git("config", "branch.main.remote", "origin")
	r.Git("config", "branch.main.merge", mainRef)

	got := readStatus(t, env, r)
	if !got.Unborn || got.UpstreamGone || got.Upstream != "origin/main" {
		t.Errorf("status = %+v", got)
	}

	if got.UpstreamSHA != last {
		t.Errorf("UpstreamSHA = %q, want %q", got.UpstreamSHA, last)
	}

	if got.UpstreamRemote != "origin" || got.UpstreamRemoteRef != mainRef {
		t.Errorf("remote = %q, remoteRef = %q", got.UpstreamRemote, got.UpstreamRemoteRef)
	}
}
