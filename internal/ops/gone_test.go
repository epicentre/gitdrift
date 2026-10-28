package ops_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/gittest"
	"github.com/epicentre/gitdrift/internal/ops"
	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	goneBranch = "feature/x"
	originName = "origin"
)

// goneFixture is a clone whose feature/x was squash-merged and deleted on the
// remote, pruned locally. The clone is back on main.
type goneFixture struct {
	t      *testing.T
	env    *gittest.Env
	remote string
	app    *gittest.Repo
	mate   *gittest.Repo
	g      *git.Runner
	loc    repo.Location
	tip    string
}

func newGoneFixture(t *testing.T) *goneFixture {
	t.Helper()

	env := gittest.New(t)
	remote := env.Remote("origin.git")
	app := env.Clone(remote, "app")
	app.Commit("README.md", "hello\n", "initial")
	app.Git("push", "--quiet", "-u", originName, "main")

	app.Git("switch", "--quiet", "-c", goneBranch)
	app.Commit("x1.txt", "1", "feature one")
	tip := app.Commit("x2.txt", "2", "feature two")
	app.Git("push", "--quiet", "-u", originName, goneBranch)
	app.Git("switch", "--quiet", "main")

	mate := env.Clone(remote, "teammate")
	mate.Write("x1.txt", "1")
	mate.Write("x2.txt", "2")
	mate.Git("add", "--", "x1.txt", "x2.txt")
	mate.Git("commit", "--quiet", "-m", "feature x (squashed)")
	mate.Git("push", "--quiet", originName, "main")
	mate.Git("push", "--quiet", originName, "--delete", goneBranch)

	app.Git("fetch", "--prune", "--quiet", originName)

	g := env.Runner()

	loc, err := repo.Locate(t.Context(), g, app.Path)
	if err != nil {
		t.Fatal(err)
	}

	return &goneFixture{t: t, env: env, remote: remote, app: app, mate: mate, g: g, loc: loc, tip: tip}
}

func (f *goneFixture) request() ops.DeleteRequest {
	return ops.DeleteRequest{
		Loc: f.loc, Branch: goneBranch, SHA: f.tip, Remote: originName, RemoteRef: "refs/heads/" + goneBranch,
	}
}

func (f *goneFixture) branchTip() (string, bool) {
	f.t.Helper()

	out, err := f.app.TryGit("rev-parse", "--verify", "--quiet", "refs/heads/"+goneBranch)
	if err != nil {
		return "", false
	}

	return strings.TrimSpace(out), true
}

func TestDeleteGoneSquashMerged(t *testing.T) {
	t.Parallel()

	f := newGoneFixture(t)

	got := ops.DeleteGone(t.Context(), f.g, f.request())
	if !got.Deleted || got.Skipped != "" || got.Err != nil {
		t.Fatalf("outcome = %+v, want deleted", got)
	}

	if _, ok := f.branchTip(); ok {
		t.Fatal("branch still exists")
	}

	want := "git branch feature/x " + f.tip
	if cmd := ops.RestoreCommand(got); cmd != want {
		t.Fatalf("RestoreCommand = %q, want %q", cmd, want)
	}

	f.app.Git("branch", "feature/x", f.tip)

	if tip, ok := f.branchTip(); !ok || tip != f.tip {
		t.Fatalf("restored tip = %q (exists %v), want %q", tip, ok, f.tip)
	}
}

func TestDeleteGoneSkips(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(f *goneFixture, req *ops.DeleteRequest)
		want   string
		prefix bool
	}{
		{
			name:   "branch missing",
			mutate: func(f *goneFixture, _ *ops.DeleteRequest) { f.app.Git("branch", "-D", goneBranch) },
			want:   "branch no longer exists",
		},
		{
			name: "tip moved after review",
			mutate: func(f *goneFixture, _ *ops.DeleteRequest) {
				f.app.Git("switch", "--quiet", goneBranch)
				f.app.Commit("x3.txt", "3", "late commit")
				f.app.Git("switch", "--quiet", "main")
			},
			want: "branch changed since review",
		},
		{
			name: "checked out in a linked worktree",
			mutate: func(f *goneFixture, _ *ops.DeleteRequest) {
				f.app.Git("worktree", "add", "--quiet", filepath.Join(f.env.WorkDir(), "linked"), goneBranch)
			},
			want: "branch is checked out",
		},
		{
			name:   "checked out in the main worktree",
			mutate: func(f *goneFixture, _ *ops.DeleteRequest) { f.app.Git("switch", "--quiet", goneBranch) },
			want:   "branch is checked out",
		},
		{
			name: "upstream exists again",
			mutate: func(f *goneFixture, _ *ops.DeleteRequest) {
				f.app.Git("update-ref", "refs/remotes/origin/"+goneBranch, f.tip)
			},
			want: "upstream exists again",
		},
		{
			name: "remote still has the branch",
			mutate: func(f *goneFixture, _ *ops.DeleteRequest) {
				f.mate.Git("push", "--quiet", originName, f.tip+":refs/heads/"+goneBranch)
				// Local tracking ref stays absent, as after a manual cleanup.
			},
			want: "branch still exists on remote",
		},
		{
			name:   "no upstream",
			mutate: func(f *goneFixture, _ *ops.DeleteRequest) { f.app.Git("branch", "--unset-upstream", goneBranch) },
			want:   "branch has no remote-tracking upstream",
		},
		{
			name:   "local upstream",
			mutate: func(f *goneFixture, _ *ops.DeleteRequest) { f.app.Git("branch", "--set-upstream-to=main", goneBranch) },
			want:   "branch has no remote-tracking upstream",
		},
		{
			name:   "empty remote",
			mutate: func(_ *goneFixture, req *ops.DeleteRequest) { req.Remote = "" },
			want:   "could not verify remote: branch has no remote or remote ref",
		},
		{
			name: "remote unreachable",
			mutate: func(f *goneFixture, _ *ops.DeleteRequest) {
				f.app.Git("remote", "set-url", originName, filepath.Join(f.env.WorkDir(), "missing.git"))
			},
			want:   "could not verify remote: ",
			prefix: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newGoneFixture(t)
			req := f.request()
			tt.mutate(f, &req)

			wantTip, existed := f.branchTip()

			got := ops.DeleteGone(t.Context(), f.g, req)
			if got.Deleted || got.Err != nil {
				t.Fatalf("outcome = %+v, want skipped", got)
			}

			if tt.prefix && !strings.HasPrefix(got.Skipped, tt.want) || !tt.prefix && got.Skipped != tt.want {
				t.Fatalf("Skipped = %q, want %q", got.Skipped, tt.want)
			}

			if tip, ok := f.branchTip(); ok != existed || tip != wantTip {
				t.Fatalf("branch changed: tip %q exists %v, want %q exists %v", tip, ok, wantTip, existed)
			}
		})
	}
}

func TestDeleteGoneSiblingCheckedOut(t *testing.T) {
	t.Parallel()

	f := newGoneFixture(t)
	f.app.Git("branch", goneBranch+"-2", "main")
	f.app.Git("worktree", "add", "--quiet", filepath.Join(f.env.WorkDir(), "sibling"), goneBranch+"-2")

	got := ops.DeleteGone(t.Context(), f.g, f.request())
	if !got.Deleted {
		t.Fatalf("outcome = %+v, want deleted", got)
	}
}

func TestDeleteGoneCancelledDuringLsRemote(t *testing.T) {
	t.Parallel()

	f := newGoneFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	// The upload-pack hook cancels the caller from inside the network window
	// by touching a file; poll for it instead.
	hook := filepath.Join(t.TempDir(), "hook.sh")
	marker := filepath.Join(t.TempDir(), "started")
	script := "#!/bin/sh\ntouch '" + marker + "'\nsleep 5\nexec git upload-pack \"$@\"\n"

	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil { //nolint:gosec // test hook must be executable
		t.Fatal(err)
	}

	f.app.Git("config", "--local", "remote.origin.uploadpack", hook)

	go func() {
		for {
			if _, err := os.Stat(marker); err == nil {
				cancel()

				return
			}

			time.Sleep(10 * time.Millisecond)
		}
	}()

	got := ops.DeleteGone(ctx, f.g, f.request())
	if got.Skipped != "" || !errors.Is(got.Err, context.Canceled) {
		t.Fatalf("outcome = %+v, want Err context.Canceled and no skip", got)
	}

	if _, ok := f.branchTip(); !ok {
		t.Fatal("branch was deleted")
	}
}

// lockFiles lists the git lock files left under dir.
func lockFiles(t *testing.T, dir string) []string {
	t.Helper()

	var locks []string

	err := filepath.WalkDir(dir, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".lock") {
			locks = append(locks, path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return locks
}

// TestDeleteGoneFinishesOnCancel cancels the caller while branch -D updates
// the ref: the deletion runs to its end instead of being killed with the ref
// locked.
func TestDeleteGoneFinishesOnCancel(t *testing.T) {
	t.Parallel()

	f := newGoneFixture(t)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	hooks := t.TempDir()
	marker := filepath.Join(t.TempDir(), "deleting")
	script := "#!/bin/sh\ncat >/dev/null\nif [ \"$1\" = prepared ]; then\n\ttouch '" + marker + "'\n\tsleep 1\nfi\n"

	if err := os.WriteFile(filepath.Join(hooks, "reference-transaction"), []byte(script), 0o755); err != nil { //nolint:gosec // test hook must be executable
		t.Fatal(err)
	}

	f.app.Git("config", "--local", "core.hooksPath", hooks)

	go func() {
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()

		for {
			if _, err := os.Stat(marker); err == nil {
				cancel()

				return
			}

			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()

	got := ops.DeleteGone(ctx, f.g, f.request())
	if !got.Deleted || got.Skipped != "" || got.Err != nil {
		t.Fatalf("outcome = %+v, want deleted", got)
	}

	if ctx.Err() == nil {
		t.Fatal("the caller was not cancelled while branch -D ran")
	}

	if _, ok := f.branchTip(); ok {
		t.Fatal("branch still exists")
	}

	if locks := lockFiles(t, filepath.Join(f.app.Path, ".git")); len(locks) != 0 {
		t.Fatalf("lock files left: %v", locks)
	}
}

func TestDeleteGoneRecheckAfterLsRemote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// race is the git command the upload-pack hook runs inside the clone
		// while ls-remote is in flight.
		race func(f *goneFixture) []string
		want string
		// tipAfter reports the expected branch tip.
		tipAfter func(f *goneFixture, late string) string
	}{
		{
			name:     "tip moved",
			race:     func(_ *goneFixture) []string { return []string{"update-ref", "refs/heads/" + goneBranch, "LATE"} },
			want:     "branch changed since review",
			tipAfter: func(_ *goneFixture, late string) string { return late },
		},
		{
			name: "checked out",
			race: func(f *goneFixture) []string {
				return []string{"worktree", "add", "--quiet", filepath.Join(f.env.WorkDir(), "late-wt"), goneBranch}
			},
			want:     "branch is checked out",
			tipAfter: func(f *goneFixture, _ string) string { return f.tip },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newGoneFixture(t)

			// Build the late commit on a temporary branch so the hook only
			// moves a ref.
			f.app.Git("switch", "--quiet", "-c", "late-tmp", goneBranch)
			late := f.app.Commit("late.txt", "late", "late commit")
			f.app.Git("switch", "--quiet", "main")

			args := tt.race(f)
			for i, a := range args {
				if a == "LATE" {
					args[i] = late
				}
			}

			quoted := make([]string, len(args))
			for i, a := range args {
				quoted[i] = "'" + a + "'"
			}

			hook := filepath.Join(t.TempDir(), "hook.sh")
			script := "#!/bin/sh\ngit -C '" + f.app.Path + "' " + strings.Join(quoted, " ") + " >/dev/null 2>&1\nexec git upload-pack \"$@\"\n"

			if err := os.WriteFile(hook, []byte(script), 0o755); err != nil { //nolint:gosec // test hook must be executable
				t.Fatal(err)
			}

			f.app.Git("config", "--local", "remote.origin.uploadpack", hook)

			got := ops.DeleteGone(t.Context(), f.g, f.request())
			if got.Deleted || got.Err != nil || got.Skipped != tt.want {
				t.Fatalf("outcome = %+v, want skip %q", got, tt.want)
			}

			if tip, ok := f.branchTip(); !ok || tip != tt.tipAfter(f, late) {
				t.Fatalf("branch tip = %q (exists %v), want %q", tip, ok, tt.tipAfter(f, late))
			}
		})
	}
}

func TestDeleteGoneRemoteStillHasBranch(t *testing.T) {
	t.Parallel()

	// The spec case: the tracking ref was deleted locally while the remote
	// still has the branch.
	env := gittest.New(t)
	remote := env.Remote("origin.git")
	app := env.Clone(remote, "app")
	app.Commit("README.md", "hello\n", "initial")
	app.Git("push", "--quiet", "-u", originName, "main")
	app.Git("switch", "--quiet", "-c", goneBranch)
	tip := app.Commit("x.txt", "1", "feature")
	app.Git("push", "--quiet", "-u", originName, goneBranch)
	app.Git("switch", "--quiet", "main")
	app.Git("update-ref", "-d", "refs/remotes/origin/"+goneBranch)

	g := env.Runner()

	loc, err := repo.Locate(t.Context(), g, app.Path)
	if err != nil {
		t.Fatal(err)
	}

	got := ops.DeleteGone(t.Context(), g, ops.DeleteRequest{
		Loc: loc, Branch: goneBranch, SHA: tip, Remote: originName, RemoteRef: "refs/heads/" + goneBranch,
	})
	if got.Deleted || got.Skipped != "branch still exists on remote" {
		t.Fatalf("outcome = %+v", got)
	}

	if _, err := app.TryGit("rev-parse", "--verify", "--quiet", "refs/heads/"+goneBranch); err != nil {
		t.Fatal("branch was deleted")
	}
}

func TestDeleteGoneSameNamedTag(t *testing.T) {
	t.Parallel()

	f := newGoneFixture(t)
	// A tag with the branch's name must never be resolved instead.
	f.app.Git("tag", "--no-sign", goneBranch, "main")

	got := ops.DeleteGone(t.Context(), f.g, f.request())
	if !got.Deleted {
		t.Fatalf("outcome = %+v, want deleted", got)
	}

	if _, err := f.app.TryGit("rev-parse", "--verify", "--quiet", "refs/tags/"+goneBranch); err != nil {
		t.Fatal("the tag was deleted")
	}
}

func TestRestoreCommand(t *testing.T) {
	t.Parallel()

	const sha = "0123456789abcdef0123456789abcdef01234567"

	tests := []struct {
		name, branch, want string
	}{
		{"plain", "feature/x", "git branch feature/x " + sha},
		{"allowed punctuation", "rel/1.2_a@b+c-d", "git branch rel/1.2_a@b+c-d " + sha},
		{"dollar", "a$(id)", "git branch 'a$(id)' " + sha},
		{"semicolon", "a;rm", "git branch 'a;rm' " + sha},
		{"quote", "it's", `git branch 'it'\''s' ` + sha},
		{"space", "a b", "git branch 'a b' " + sha},
		{"unicode", "feature/caf\u00e9", "git branch 'feature/caf\u00e9' " + sha},
		{"bidi override", "a\u202Eb", `git branch "$(printf '\141\342\200\256\142')" ` + sha},
		{"zero-width space", "a\u200Bb", `git branch "$(printf '\141\342\200\213\142')" ` + sha},
		{"c1 control", "a\u0085", `git branch "$(printf '\141\302\205')" ` + sha},
		{"line separator", "a\u2028", `git branch "$(printf '\141\342\200\250')" ` + sha},
		{"invalid utf-8", "a\xff", `git branch "$(printf '\141\377')" ` + sha},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ops.RestoreCommand(ops.DeleteOutcome{Branch: tt.branch, SHA: sha}); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRestoreCommandPrintfIsExact runs the printf of a restore command and
// compares its output with the branch name, byte for byte.
func TestRestoreCommandPrintfIsExact(t *testing.T) {
	t.Parallel()

	printf, err := exec.LookPath("printf")
	if err != nil {
		t.Skip("no printf binary")
	}

	for _, branch := range []string{"feature/\u202Eevil\u2066x", "a\u00AD\U000E0041b", "x\u0085y\xfe"} {
		cmd := ops.RestoreCommand(ops.DeleteOutcome{Branch: branch, SHA: "0123456789abcdef0123456789abcdef01234567"})

		_, rest, ok := strings.Cut(cmd, `"$(printf '`)
		format, _, found := strings.Cut(rest, `')"`)

		if !ok || !found {
			t.Fatalf("RestoreCommand(%q) = %q, want the printf form", branch, cmd)
		}

		for _, r := range cmd {
			if r < ' ' || r > '~' {
				t.Fatalf("RestoreCommand(%q) = %q, want printable ASCII", branch, cmd)
			}
		}

		out, err := exec.CommandContext(t.Context(), printf, format).Output() //nolint:gosec // the format is the command under test
		if err != nil {
			t.Fatal(err)
		}

		if string(out) != branch {
			t.Errorf("printf %q = %q, want %q", format, out, branch)
		}
	}
}
