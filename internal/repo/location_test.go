package repo_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/epicentre/gitdrift/internal/gittest"
	"github.com/epicentre/gitdrift/internal/repo"
)

// newClone returns an isolated environment and a clone of a remote that holds
// one pushed commit on main.
func newClone(t *testing.T, name string) (*gittest.Env, string, *gittest.Repo) {
	t.Helper()

	env := gittest.New(t)
	remote := env.Remote("origin.git")
	r := env.Clone(remote, name)
	r.Commit("README.md", "hello\n", "initial")
	r.Git("push", "--quiet", "-u", "origin", "main")

	return env, remote, r
}

func assertResolved(t *testing.T, label, path string) {
	t.Helper()

	if !filepath.IsAbs(path) {
		t.Errorf("%s = %q, want absolute", label, path)
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		t.Errorf("%s = %q, want symlink-resolved (%q, %v)", label, path, resolved, err)
	}
}

func TestLocate(t *testing.T) {
	t.Parallel()

	env, _, r := newClone(t, "payments")
	g := env.Runner()

	loc, err := repo.Locate(t.Context(), g, r.Path)
	if err != nil {
		t.Fatal(err)
	}

	assertResolved(t, "Path", loc.Path)
	assertResolved(t, "GitDir", loc.GitDir)
	assertResolved(t, "CommonDir", loc.CommonDir)

	if loc.Path != r.Path {
		t.Errorf("Path = %q, want %q", loc.Path, r.Path)
	}

	if loc.Name != "payments" {
		t.Errorf("Name = %q, want payments", loc.Name)
	}

	if loc.ID != repo.ID(loc.Path) {
		t.Errorf("ID = %q, want %q", loc.ID, loc.Path)
	}

	if want := filepath.Join(r.Path, ".git"); loc.GitDir != want || loc.CommonDir != want {
		t.Errorf("GitDir = %q, CommonDir = %q, want %q", loc.GitDir, loc.CommonDir, want)
	}
}

func TestLocateErrors(t *testing.T) {
	t.Parallel()

	env, _, r := newClone(t, "payments")
	r.Write("sub/file.txt", "x")

	missing := filepath.Join(env.WorkDir(), "missing")
	plain := t.TempDir() // outside every repository

	tests := []struct {
		name string
		dir  string
		want error
	}{
		{"subdirectory", filepath.Join(r.Path, "sub"), repo.ErrNotToplevel},
		{"plain directory", plain, repo.ErrNotRepo},
		{"missing directory", missing, repo.ErrNotRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := repo.Locate(t.Context(), env.Runner(), tt.dir)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestLocateLinkedWorktree(t *testing.T) {
	t.Parallel()

	env, _, r := newClone(t, "payments")
	r.Git("worktree", "add", "--quiet", "../wt", "-b", "wt")

	wt := filepath.Join(env.WorkDir(), "wt")

	loc, err := repo.Locate(t.Context(), env.Runner(), wt)
	if err != nil {
		t.Fatal(err)
	}

	if want := filepath.Join(r.Path, ".git"); loc.CommonDir != want {
		t.Errorf("CommonDir = %q, want %q", loc.CommonDir, want)
	}

	if loc.GitDir == loc.CommonDir {
		t.Errorf("GitDir = %q, want it to differ from CommonDir", loc.GitDir)
	}

	if loc.Path != wt || loc.Name != "wt" {
		t.Errorf("Path = %q, Name = %q", loc.Path, loc.Name)
	}
}
