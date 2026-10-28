package workspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

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

	groups, err := workspace.Discover(t.Context(), env.Runner(), work)
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

	groups, err := workspace.Discover(t.Context(), env.Runner(), root.Path)
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

	groups, err := workspace.Discover(t.Context(), env.Runner(), env.WorkDir())
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

	_, err := workspace.Discover(t.Context(), env.Runner(), empty)
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

	groups, err := workspace.Discover(t.Context(), env.Runner(), work)
	if err != nil {
		t.Fatal(err)
	}

	if len(groups) != 1 || groups[0].Members[0].Path != a.Path {
		t.Fatalf("groups = %+v, want only %s", groups, a.Path)
	}
}
