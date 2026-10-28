package workspace_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/gittest"
	"github.com/epicentre/gitdrift/internal/workspace"
)

func initRepo(t *testing.T, env *gittest.Env, name string) *gittest.Repo {
	t.Helper()

	r := env.Init(name)
	r.Commit("README.md", "hi\n", "initial")

	return r
}

func memberNames(g workspace.Group) []string {
	var names []string
	for _, m := range g.Members {
		names = append(names, m.Name)
	}

	return names
}

func TestDiscover(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	a := initRepo(t, env, "a")
	initRepo(t, env, "b")

	work := env.WorkDir()
	if err := os.Mkdir(filepath.Join(work, "c"), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(work, "d"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	a.Git("worktree", "add", "--quiet", "-b", "wt", filepath.Join(work, "a-wt"))

	groups, err := workspace.Discover(t.Context(), env.Runner(), work, workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2: %+v", len(groups), groups)
	}

	if got := memberNames(groups[0]); len(got) != 2 || got[0] != "a" || got[1] != "a-wt" {
		t.Fatalf("first group members = %v, want [a a-wt]", got)
	}

	if got := memberNames(groups[1]); len(got) != 1 || got[0] != "b" {
		t.Fatalf("second group members = %v, want [b]", got)
	}
}

func TestDiscoverRootIsRepo(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	root := initRepo(t, env, "root")
	initRepo(t, env, "root/x")

	groups, err := workspace.Discover(t.Context(), env.Runner(), root.Path, workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2: %+v", len(groups), groups)
	}

	if got := groups[0].Members[0].Path; got != root.Path {
		t.Fatalf("first group = %s, want root %s", got, root.Path)
	}

	if got := groups[1].Members[0].Name; got != "x" {
		t.Fatalf("second group = %s, want x", got)
	}
}

func TestDiscoverSymlinkDedup(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	a := initRepo(t, env, "a")

	if err := os.Symlink(a.Path, filepath.Join(env.WorkDir(), "a-link")); err != nil {
		t.Fatal(err)
	}

	groups, err := workspace.Discover(t.Context(), env.Runner(), env.WorkDir(), workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(groups) != 1 || len(groups[0].Members) != 1 || groups[0].Members[0].Path != a.Path {
		t.Fatalf("groups = %+v, want one group with only %s", groups, a.Path)
	}
}

func TestDiscoverNone(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	empty := filepath.Join(env.WorkDir(), "empty")

	if err := os.MkdirAll(empty, 0o750); err != nil {
		t.Fatal(err)
	}

	_, err := workspace.Discover(t.Context(), env.Runner(), empty, workspace.DiscoverOptions{})
	if !errors.Is(err, workspace.ErrNoRepos) {
		t.Fatalf("err = %v, want ErrNoRepos", err)
	}
}

func TestDiscoverSkipsUnusableChildren(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	a := initRepo(t, env, "a")
	work := env.WorkDir()

	locked := filepath.Join(work, "locked")
	if err := os.Mkdir(locked, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) }) //nolint:gosec // restore access so TempDir can clean up

	if err := os.Symlink(filepath.Join(work, "missing"), filepath.Join(work, "dangling")); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(filepath.Join(work, "loop-b"), filepath.Join(work, "loop-a")); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(filepath.Join(work, "loop-a"), filepath.Join(work, "loop-b")); err != nil {
		t.Fatal(err)
	}

	groups, err := workspace.Discover(t.Context(), env.Runner(), work, workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(groups) != 1 || groups[0].Members[0].Path != a.Path {
		t.Fatalf("groups = %+v, want only %s", groups, a.Path)
	}
}

// nestedTree builds repos at a, x/b, x/y/c, x/y/z/w/d and x/y/z/w/v/e.
func nestedTree(t *testing.T) *gittest.Env {
	t.Helper()

	env := gittest.New(t)
	for _, name := range []string{"a", "x/b", "x/y/c", "x/y/z/w/d", "x/y/z/w/v/e"} {
		initRepo(t, env, name)
	}

	return env
}

type found struct{ Name, Folder string }

func flatten(groups []workspace.Group) []found {
	var out []found

	for _, g := range groups {
		for _, m := range g.Members {
			out = append(out, found{m.Name, m.Folder})
		}
	}

	return out
}

func TestDiscoverNested(t *testing.T) {
	t.Parallel()

	env := nestedTree(t)

	groups, err := workspace.Discover(t.Context(), env.Runner(), env.WorkDir(), workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	want := []found{{"a", ""}, {"b", "x"}, {"c", "x/y"}, {"d", "x/y/z/w"}}
	if diff := cmp.Diff(want, flatten(groups)); diff != "" {
		t.Fatalf("repositories mismatch (-want +got):\n%s", diff)
	}
}

func TestDiscoverDepthOne(t *testing.T) {
	t.Parallel()

	env := nestedTree(t)

	groups, err := workspace.Discover(t.Context(), env.Runner(), env.WorkDir(), workspace.DiscoverOptions{Depth: 1})
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]found{{"a", ""}}, flatten(groups)); diff != "" {
		t.Fatalf("repositories mismatch (-want +got):\n%s", diff)
	}
}

func TestDiscoverStopsAtRepositories(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	initRepo(t, env, "a")
	initRepo(t, env, "a/inner")

	pkg := filepath.Join(env.WorkDir(), "a", "node_modules", "pkg")
	if err := os.MkdirAll(filepath.Join(pkg, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}

	groups, err := workspace.Discover(t.Context(), env.Runner(), env.WorkDir(), workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]found{{"a", ""}}, flatten(groups)); diff != "" {
		t.Fatalf("repositories mismatch (-want +got):\n%s", diff)
	}
}

func TestDiscoverSkipsHiddenAndHeavyDirs(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	for _, name := range []string{".cache/r1", "node_modules/r2", "vendor/r3", "ok/r4"} {
		initRepo(t, env, name)
	}

	groups, err := workspace.Discover(t.Context(), env.Runner(), env.WorkDir(), workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]found{{"r4", "ok"}}, flatten(groups)); diff != "" {
		t.Fatalf("repositories mismatch (-want +got):\n%s", diff)
	}
}

// TestDiscoverSymlinkLoop checks that a link back to the root is not walked
// again: x/b is located once, through the shallowest route, and never again
// through x/loop/x/b, which the depth limit alone would allow.
func TestDiscoverSymlinkLoop(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	initRepo(t, env, "x/b")

	if err := os.Symlink("..", filepath.Join(env.WorkDir(), "x", "loop")); err != nil {
		t.Fatal(err)
	}

	groups, calls, err := workspace.DiscoverCounting(t.Context(), env.Runner(), env.WorkDir(), workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]found{{"b", "x"}}, flatten(groups)); diff != "" {
		t.Fatalf("repositories mismatch (-want +got):\n%s", diff)
	}

	if calls != 2 {
		t.Fatalf("locate calls = %d, want 2 (the root and x/b)", calls)
	}
}

// TestDiscoverBareRepoWithWorktrees checks the layout of a bare clone kept
// in proj/.bare, a proj/.git file pointing at it and the worktrees inside
// proj: proj is no work tree, so it is walked like a plain directory, its
// worktrees are listed, and .bare stays skipped.
func TestDiscoverBareRepoWithWorktrees(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	seed := initRepo(t, env, "seed")

	root := filepath.Join(env.WorkDir(), "projects")
	proj := filepath.Join(root, "proj")

	seed.Git("clone", "--bare", "--quiet", seed.Path, filepath.Join(proj, ".bare"))

	if err := os.WriteFile(filepath.Join(proj, ".git"), []byte("gitdir: ./.bare\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	seed.Git("-C", proj, "worktree", "add", "--quiet", "main", "main")
	seed.Git("-C", proj, "worktree", "add", "--quiet", "-b", "feature", "feature", "main")

	groups, err := workspace.Discover(t.Context(), env.Runner(), root, workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]found{{"feature", "proj"}, {"main", "proj"}}, flatten(groups)); diff != "" {
		t.Fatalf("repositories mismatch (-want +got):\n%s", diff)
	}

	if len(groups) != 1 {
		t.Fatalf("groups = %d, want the two worktrees in one group: %+v", len(groups), groups)
	}
}

func TestDiscoverSymlinkedDuplicateKeepsShallowest(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	r := initRepo(t, env, "deep/one/r")

	if err := os.Symlink(r.Path, filepath.Join(env.WorkDir(), "r-link")); err != nil {
		t.Fatal(err)
	}

	groups, err := workspace.Discover(t.Context(), env.Runner(), env.WorkDir(), workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]found{{"r", ""}}, flatten(groups)); diff != "" {
		t.Fatalf("repositories mismatch (-want +got):\n%s", diff)
	}
}

func TestDiscoverRootRepoAndChildren(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	root := initRepo(t, env, "root")
	initRepo(t, env, "root/sub/child")

	groups, err := workspace.Discover(t.Context(), env.Runner(), root.Path, workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	want := []found{{"root", ""}, {"child", "sub"}}
	if diff := cmp.Diff(want, flatten(groups)); diff != "" {
		t.Fatalf("repositories mismatch (-want +got):\n%s", diff)
	}
}

func TestDiscoverLocatesOnlyCandidates(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	work := env.WorkDir()

	for i := range 100 {
		for _, pattern := range []string{"p%03d", "q%03d/p", "r%03d/p/s"} {
			dir := filepath.Join(work, fmt.Sprintf(pattern, i))
			if err := os.MkdirAll(dir, 0o750); err != nil {
				t.Fatal(err)
			}
		}
	}

	initRepo(t, env, "q000/repo1")
	initRepo(t, env, "r000/p/repo2")

	groups, calls, err := workspace.DiscoverCounting(t.Context(), env.Runner(), work, workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(flatten(groups)) != 2 {
		t.Fatalf("repositories = %+v, want 2", flatten(groups))
	}

	if calls != 3 {
		t.Fatalf("locate calls = %d, want 3 (root and two repositories)", calls)
	}
}

func TestDiscoverListsRepoNamedLikeSkippedDir(t *testing.T) {
	t.Parallel()

	for _, opts := range []workspace.DiscoverOptions{{Depth: 1}, {}} {
		env := gittest.New(t)
		initRepo(t, env, "vendor")
		initRepo(t, env, ".dotfiles")

		groups, err := workspace.Discover(t.Context(), env.Runner(), env.WorkDir(), opts)
		if err != nil {
			t.Fatal(err)
		}

		want := []found{{".dotfiles", ""}, {"vendor", ""}}
		if diff := cmp.Diff(want, flatten(groups)); diff != "" {
			t.Fatalf("depth %d: repositories mismatch (-want +got):\n%s", opts.Depth, diff)
		}
	}
}

func TestDiscoverUnreadableRootIsFatal(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	root := filepath.Join(env.WorkDir(), "noread")

	if err := os.MkdirAll(root, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(root, 0o100); err != nil { //nolint:gosec // enter but not list is the point of the test
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(root, 0o750) }) //nolint:gosec // restore access so TempDir can clean up

	_, err := workspace.Discover(t.Context(), env.Runner(), root, workspace.DiscoverOptions{})
	if err == nil || errors.Is(err, workspace.ErrNoRepos) {
		t.Fatalf("err = %v, want a read error other than ErrNoRepos", err)
	}
}

func TestDiscoverSameDepthTieKeepsAlphabeticallyFirstRoute(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	r := initRepo(t, env, "store/r")
	work := env.WorkDir()

	for _, name := range []string{"b-link", "a-link"} {
		if err := os.MkdirAll(filepath.Join(work, name[:1]), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.Symlink(r.Path, filepath.Join(work, name[:1], "r")); err != nil {
			t.Fatal(err)
		}
	}

	groups, err := workspace.Discover(t.Context(), env.Runner(), work, workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]found{{"r", "a"}}, flatten(groups)); diff != "" {
		t.Fatalf("repositories mismatch (-want +got):\n%s", diff)
	}
}

func TestDiscoverCancelledContext(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	initRepo(t, env, "a")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	groups, err := workspace.Discover(ctx, env.Runner(), env.WorkDir(), workspace.DiscoverOptions{})
	if !errors.Is(err, context.Canceled) || groups != nil {
		t.Fatalf("groups = %+v, err = %v, want nil and context.Canceled", groups, err)
	}
}

func TestDiscoverSkipsUnreadableNestedDir(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	initRepo(t, env, "x/ok/a")
	initRepo(t, env, "x/zz/b")

	locked := filepath.Join(env.WorkDir(), "x", "locked")
	if err := os.Mkdir(locked, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) }) //nolint:gosec // restore access so TempDir can clean up

	groups, err := workspace.Discover(t.Context(), env.Runner(), env.WorkDir(), workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	want := []found{{"a", "x/ok"}, {"b", "x/zz"}}
	if diff := cmp.Diff(want, flatten(groups)); diff != "" {
		t.Fatalf("repositories mismatch (-want +got):\n%s", diff)
	}
}
