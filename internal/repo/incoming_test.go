package repo_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/badge"
	"github.com/epicentre/gitdrift/internal/gittest"
	"github.com/epicentre/gitdrift/internal/repo"
)

func TestIncomingFilesAndRules(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	remote, app := pushedMain(t, env)

	mate := env.Clone(remote, "teammate")
	mate.Commit("go.sum", "", "bump deps")
	mate.Commit("Dockerfile", "FROM scratch", "add image")
	mate.Commit("docs/my notes é.md", "n", "notes")
	mate.Git("push", "--quiet", "origin", "main")

	app.Git("fetch", "--quiet", "origin")
	app.Git("config", "--local", "--add", "gitdrift.badge", "docker=Dockerfile")
	app.Git("config", "--local", "--add", "gitdrift.badge", "x")

	g := env.Runner()

	loc, err := repo.Locate(t.Context(), g, app.Path)
	if err != nil {
		t.Fatal(err)
	}

	st, err := repo.ReadStatus(t.Context(), g, loc)
	if err != nil {
		t.Fatal(err)
	}

	files, err := repo.IncomingFiles(t.Context(), g, loc, st)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"go.sum", "Dockerfile", "docs/my notes é.md"} {
		if !slices.Contains(files, want) {
			t.Errorf("IncomingFiles = %q, missing %q", files, want)
		}
	}

	rules, err := repo.BadgeRules(t.Context(), g, loc)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]string{"deps", "docker"}, badge.Match(rules, files)); diff != "" {
		t.Errorf("labels mismatch (-want +got):\n%s", diff)
	}
}

// TestIncomingFilesListsBothSidesOfRename checks that a rename lists the
// path it removes as well as the one it adds: a pull touches both.
func TestIncomingFilesListsBothSidesOfRename(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	remote, app := pushedMain(t, env)

	mate := env.Clone(remote, "teammate")
	mate.Git("mv", "base.txt", "moved.txt")
	mate.Git("commit", "--quiet", "-m", "move base")
	mate.Git("push", "--quiet", "origin", "main")

	app.Git("fetch", "--quiet", "origin")

	g := env.Runner()

	loc, err := repo.Locate(t.Context(), g, app.Path)
	if err != nil {
		t.Fatal(err)
	}

	st, err := repo.ReadStatus(t.Context(), g, loc)
	if err != nil {
		t.Fatal(err)
	}

	files, err := repo.IncomingFiles(t.Context(), g, loc, st)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]string{"base.txt", "moved.txt"}, files); diff != "" {
		t.Errorf("IncomingFiles mismatch (-want +got):\n%s", diff)
	}
}

func TestIncomingFilesNotBehind(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	_, app := pushedMain(t, env)
	g := env.Runner()

	loc, err := repo.Locate(t.Context(), g, app.Path)
	if err != nil {
		t.Fatal(err)
	}

	st, err := repo.ReadStatus(t.Context(), g, loc)
	if err != nil {
		t.Fatal(err)
	}

	files, err := repo.IncomingFiles(t.Context(), g, loc, st)
	if err != nil {
		t.Fatal(err)
	}

	if files != nil {
		t.Errorf("IncomingFiles = %q, want nil", files)
	}

	rules, err := repo.BadgeRules(t.Context(), g, loc)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(badge.Defaults(), rules); diff != "" {
		t.Errorf("rules without config mismatch (-want +got):\n%s", diff)
	}
}
