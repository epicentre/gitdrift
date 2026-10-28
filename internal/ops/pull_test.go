package ops_test

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/ops"
)

func readFile(t *testing.T, f *fixture, name string) string {
	t.Helper()

	root, err := os.OpenRoot(f.repo.Path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = root.Close() })

	b, err := fs.ReadFile(root.FS(), name)
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

func requireOverwriteError(t *testing.T, err error) {
	t.Helper()

	var cmdErr *git.CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("err = %v, want *git.CommandError", err)
	}

	if !strings.Contains(cmdErr.Stderr, "would be overwritten") {
		t.Fatalf("stderr = %q, want it to mention \"would be overwritten\"", cmdErr.Stderr)
	}
}

func TestPullFastForward(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.teammatePush(2)
	f.fetch()

	req := f.request()
	out := ops.Pull(t.Context(), f.g, req)

	if out.Err != nil || out.Skipped != "" || out.Pulled != 2 {
		t.Fatalf("outcome = %+v, want Pulled 2", out)
	}

	if head := f.repo.Git("rev-parse", "HEAD"); head != req.UpstreamSHA {
		t.Fatalf("HEAD = %s, want %s", head, req.UpstreamSHA)
	}

	if got := readFile(t, f, "matea.txt"); got != "x" {
		t.Fatalf("new file content = %q", got)
	}
}

func TestPullRefusesIgnoredOverwrite(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.repo.Commit(".gitignore", ".env\n", "ignore env")
	f.repo.Git("push", "--quiet", "origin", "main")
	f.mate.Git("pull", "--quiet", "--ff-only")
	f.mate.Write(".env", "SECRET=remote\n")
	f.mate.Git("add", "-f", ".env")
	f.mate.Git("commit", "--quiet", "-m", "add env")
	f.mate.Git("push", "--quiet", "origin", "main")

	f.repo.Write(".env", "SECRET=local\n")
	f.fetch()

	before := f.repo.Git("rev-parse", "HEAD")
	out := ops.Pull(t.Context(), f.g, f.request())

	if out.Pulled != 0 || out.Skipped != "" {
		t.Fatalf("outcome = %+v, want git error", out)
	}

	requireOverwriteError(t, out.Err)

	if got := readFile(t, f, ".env"); got != "SECRET=local\n" {
		t.Fatalf(".env = %q, local ignored file was overwritten", got)
	}

	if after := f.repo.Git("rev-parse", "HEAD"); after != before {
		t.Fatalf("HEAD moved from %s to %s", before, after)
	}
}

func TestPullUntrackedConflict(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.mate.Commit("new.txt", "remote\n", "add new")
	f.mate.Git("push", "--quiet", "origin", "main")
	f.repo.Write("new.txt", "local\n")
	f.fetch()

	before := f.repo.Git("rev-parse", "HEAD")
	out := ops.Pull(t.Context(), f.g, f.request())

	if out.Pulled != 0 || out.Skipped != "" {
		t.Fatalf("outcome = %+v, want git error", out)
	}

	requireOverwriteError(t, out.Err)

	if after := f.repo.Git("rev-parse", "HEAD"); after != before {
		t.Fatalf("HEAD moved from %s to %s", before, after)
	}

	if got := readFile(t, f, "new.txt"); got != "local\n" {
		t.Fatalf("new.txt = %q", got)
	}
}

func TestPullSkipsLocalChanges(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.teammatePush(1)
	f.fetch()
	f.repo.Write("README.md", "edited\n")

	out := ops.Pull(t.Context(), f.g, f.request())
	if diff := cmp.Diff(ops.PullOutcome{Skipped: "local changes"}, out); diff != "" {
		t.Fatalf("outcome mismatch (-want +got):\n%s", diff)
	}

	if got := readFile(t, f, "README.md"); got != "edited\n" {
		t.Fatalf("README.md = %q", got)
	}
}

func TestPullSkipsUpstreamChanged(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.teammatePush(1)
	f.fetch()

	req := f.request()

	f.mate.Commit("later.txt", "x", "later")
	f.mate.Git("push", "--quiet", "origin", "main")
	f.fetch()

	before := f.repo.Git("rev-parse", "HEAD")
	out := ops.Pull(t.Context(), f.g, req)

	if diff := cmp.Diff(ops.PullOutcome{Skipped: "upstream changed since review, press r"}, out); diff != "" {
		t.Fatalf("outcome mismatch (-want +got):\n%s", diff)
	}

	if after := f.repo.Git("rev-parse", "HEAD"); after != before {
		t.Fatal("HEAD moved")
	}
}

func TestPullSkipsHeadChanged(t *testing.T) {
	t.Parallel()

	t.Run("local commit blocks as diverged", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.teammatePush(1)
		f.fetch()

		req := f.request()

		f.repo.Commit("local.txt", "x", "local work")

		out := ops.Pull(t.Context(), f.g, req)
		if diff := cmp.Diff(ops.PullOutcome{Skipped: "diverged"}, out); diff != "" {
			t.Fatalf("outcome mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("HEAD moved back with ahead 0", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.repo.Commit("second.txt", "x", "second")
		f.repo.Git("push", "--quiet", "origin", "main")
		f.mate.Git("pull", "--quiet", "--ff-only")
		f.mate.Commit("mate.txt", "x", "teammate commit")
		f.mate.Git("push", "--quiet", "origin", "main")
		f.fetch()

		req := f.request()

		f.repo.Git("reset", "--hard", "HEAD~1")

		out := ops.Pull(t.Context(), f.g, req)
		if diff := cmp.Diff(ops.PullOutcome{Skipped: "repository changed since review, press r"}, out); diff != "" {
			t.Fatalf("outcome mismatch (-want +got):\n%s", diff)
		}
	})
}

// TestPullSkipsBranchSwitch switches to another branch at the reviewed commit
// that tracks the same upstream: HEAD and the upstream still match the
// review, but the pull would move a branch the user did not review.
func TestPullSkipsBranchSwitch(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.teammatePush(1)
	f.fetch()

	req := f.request()

	f.repo.Git("switch", "--quiet", "-c", "feature/other")
	f.repo.Git("branch", "--quiet", "--set-upstream-to=origin/main")

	if st := f.status(); st.HeadSHA != req.HeadSHA || st.UpstreamSHA != req.UpstreamSHA || st.Behind != 1 {
		t.Fatalf("status = %+v, want the reviewed HEAD and upstream, behind 1", st)
	}

	out := ops.Pull(t.Context(), f.g, req)
	if diff := cmp.Diff(ops.PullOutcome{Skipped: "repository changed since review, press r"}, out); diff != "" {
		t.Fatalf("outcome mismatch (-want +got):\n%s", diff)
	}

	if head := f.repo.Git("rev-parse", "HEAD"); head != req.HeadSHA {
		t.Fatalf("HEAD = %s, want it unchanged at %s", head, req.HeadSHA)
	}
}

func TestPullSkipsFailedFetch(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.teammatePush(1)
	f.fetch()

	req := f.request()
	req.Fetch = drift.FetchState{Done: true, Errors: map[string]string{"origin": "x"}}

	out := ops.Pull(t.Context(), f.g, req)
	if diff := cmp.Diff(ops.PullOutcome{Skipped: "fetch failed: origin"}, out); diff != "" {
		t.Fatalf("outcome mismatch (-want +got):\n%s", diff)
	}
}

func TestPullIgnoresUserPullConfig(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.env.GlobalConfig("pull.rebase", "true")
	f.env.GlobalConfig("pull.ff", "false")
	f.env.GlobalConfig("merge.ff", "false")
	f.teammatePush(2)
	f.fetch()

	req := f.request()
	out := ops.Pull(t.Context(), f.g, req)

	if out.Err != nil || out.Pulled != 2 {
		t.Fatalf("outcome = %+v, want Pulled 2", out)
	}

	if head := f.repo.Git("rev-parse", "HEAD"); head != req.UpstreamSHA {
		t.Fatalf("HEAD = %s, want %s", head, req.UpstreamSHA)
	}

	if parents := f.repo.Git("rev-list", "--parents", "-n", "1", "HEAD"); len(strings.Fields(parents)) != 2 {
		t.Fatalf("HEAD is a merge commit: %q", parents)
	}
}

func TestPullSkipsStagedOnlyChange(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.teammatePush(1)
	f.fetch()
	f.repo.Write("staged.txt", "staged\n")
	f.repo.Git("add", "--", "staged.txt")

	before := f.repo.Git("rev-parse", "HEAD")
	out := ops.Pull(t.Context(), f.g, f.request())

	if diff := cmp.Diff(ops.PullOutcome{Skipped: "local changes"}, out); diff != "" {
		t.Fatalf("outcome mismatch (-want +got):\n%s", diff)
	}

	if after := f.repo.Git("rev-parse", "HEAD"); after != before {
		t.Fatal("HEAD moved")
	}
}
