package repo_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/gittest"
	"github.com/epicentre/gitdrift/internal/repo"
)

func readWork(t *testing.T, env *gittest.Env, r *gittest.Repo) repo.Work {
	t.Helper()

	g := env.Runner()

	loc, err := repo.Locate(t.Context(), g, r.Path)
	if err != nil {
		t.Fatal(err)
	}

	w, err := repo.ReadWork(t.Context(), g, loc)
	if err != nil {
		t.Fatal(err)
	}

	return w
}

// pushedMain returns a remote with one commit on main and a clone of it.
func pushedMain(t *testing.T, env *gittest.Env) (string, *gittest.Repo) {
	t.Helper()

	remote := env.Remote("origin.git")
	seed := env.Clone(remote, "seed")
	seed.Commit("base.txt", "base", "base")
	seed.Git("push", "--quiet", "-u", "origin", "main")

	return remote, env.Clone(remote, "app")
}

// squashMergedFeature pushes feature/x with two commits, lets a teammate apply
// the same change as one commit on main and delete the branch on the remote
// (the GitLab squash flow), then prunes locally. It returns the branch tip.
func squashMergedFeature(t *testing.T, env *gittest.Env, remote string, app *gittest.Repo, noFF bool) string {
	t.Helper()

	app.Git("switch", "--quiet", "-c", "feature/x")
	app.Commit("x1.txt", "1", "feature one")
	tip := app.Commit("x2.txt", "2", "feature two")
	app.Git("push", "--quiet", "-u", "origin", "feature/x")

	mate := env.Clone(remote, "teammate")

	if noFF {
		mate.Git("fetch", "--quiet", "origin")
		mate.Git("merge", "--quiet", "--no-ff", "-m", "merge feature/x", "origin/feature/x")
	} else {
		mate.Write("x1.txt", "1")
		mate.Write("x2.txt", "2")
		mate.Git("add", "--", "x1.txt", "x2.txt")
		mate.Git("commit", "--quiet", "-m", "feature x (squashed)")
	}

	mate.Git("push", "--quiet", "origin", "main")
	mate.Git("push", "--quiet", "origin", "--delete", "feature/x")

	app.Git("fetch", "--prune", "--quiet", "origin")

	return tip
}

func TestReadWorkSquashMergedGone(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	remote, app := pushedMain(t, env)
	tip := squashMergedFeature(t, env, remote, app, false)

	w := readWork(t, env, app)

	want := []repo.GoneBranch{{
		Name: "feature/x", SHA: tip, Remote: originName, RemoteRef: "refs/heads/feature/x", OnlyHere: 2,
	}}

	if diff := cmp.Diff(want, w.Gone, cmp.FilterPath(func(p cmp.Path) bool { return p.String() == "LastCommit" }, cmp.Ignore())); diff != "" {
		t.Fatalf("Gone mismatch (-want +got):\n%s", diff)
	}

	if w.Gone[0].LastCommit.IsZero() {
		t.Error("LastCommit is zero")
	}

	if w.OnlyHereCommits != 2 {
		t.Errorf("OnlyHereCommits = %d, want 2", w.OnlyHereCommits)
	}

	if len(w.LocalOnly) != 0 {
		t.Errorf("LocalOnly = %v, want none (gone branches are listed in Gone)", w.LocalOnly)
	}
}

func TestReadWorkMergeCommitGone(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	remote, app := pushedMain(t, env)
	squashMergedFeature(t, env, remote, app, true)

	w := readWork(t, env, app)

	if len(w.Gone) != 1 || w.Gone[0].Name != "feature/x" || w.Gone[0].OnlyHere != 0 {
		t.Fatalf("Gone = %+v, want feature/x with OnlyHere 0", w.Gone)
	}

	if w.OnlyHereCommits != 0 {
		t.Errorf("OnlyHereCommits = %d, want 0", w.OnlyHereCommits)
	}
}

func TestReadWorkLocalOnly(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	_, app := pushedMain(t, env)

	app.Commit("local.txt", "l", "unpushed")
	app.Git("branch", "--no-track", "spike", "origin/main")
	app.Git("switch", "--quiet", "spike")
	app.Commit("s1.txt", "1", "spike one")
	app.Commit("s2.txt", "2", "spike two")

	w := readWork(t, env, app)

	want := []repo.LocalOnlyBranch{{Name: mainBranch, Commits: 1}, {Name: "spike", Commits: 2, NoUpstream: true}}
	if diff := cmp.Diff(want, w.LocalOnly); diff != "" {
		t.Fatalf("LocalOnly mismatch (-want +got):\n%s", diff)
	}

	if w.OnlyHereCommits != 3 {
		t.Errorf("OnlyHereCommits = %d, want 3", w.OnlyHereCommits)
	}

	if w.NoRemote {
		t.Error("NoRemote = true, want false")
	}
}

func TestReadWorkStashes(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	_, app := pushedMain(t, env)

	app.Write("base.txt", "changed")
	app.Git("stash", "push", "--quiet", "-m", "wip")

	w := readWork(t, env, app)

	if len(w.Stashes) != 1 || w.Stashes[0].Ref != "stash@{0}" {
		t.Fatalf("Stashes = %+v, want one stash@{0}", w.Stashes)
	}

	if w.Stashes[0].SHA == "" || w.Stashes[0].When.IsZero() || w.Stashes[0].Subject == "" {
		t.Errorf("incomplete stash %+v", w.Stashes[0])
	}

	if got, want := w.OnlyHereTotal(), w.OnlyHereCommits+1; got != want {
		t.Errorf("OnlyHereTotal = %d, want %d", got, want)
	}
}

func TestReadWorkNoRemote(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	r := env.Init("solo")
	r.Commit("a.txt", "a", "one")
	r.Commit("b.txt", "b", "two")
	r.Commit("c.txt", "c", "three")

	w := readWork(t, env, r)

	if !w.NoRemote || w.OnlyHereCommits != 3 {
		t.Fatalf("NoRemote = %v, OnlyHereCommits = %d, want true and 3", w.NoRemote, w.OnlyHereCommits)
	}
}

func TestReadWorkLocalUpstreamNotGone(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	_, app := pushedMain(t, env)

	app.Git("branch", "--track", "topic", "main")

	w := readWork(t, env, app)

	if len(w.Gone) != 0 {
		t.Fatalf("Gone = %+v, want none for a local upstream", w.Gone)
	}
}

func TestOnlyHereCommits(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	remote, app := pushedMain(t, env)
	squashMergedFeature(t, env, remote, app, false)

	g := env.Runner()

	loc, err := repo.Locate(t.Context(), g, app.Path)
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.OnlyHereCommits(t.Context(), g, loc, "feature/x", 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 || got[0].Subject != "feature two" || got[1].Subject != "feature one" {
		t.Fatalf("commits = %+v, want feature two then feature one", got)
	}

	limited, err := repo.OnlyHereCommits(t.Context(), g, loc, "feature/x", 1)
	if err != nil {
		t.Fatal(err)
	}

	if len(limited) != 1 || limited[0].Subject != "feature two" {
		t.Fatalf("limited = %+v, want only feature two", limited)
	}
}

// A tag with the same name as a branch must not change what is counted.
func TestReadWorkBranchTagNameCollision(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	_, app := pushedMain(t, env)

	app.Git("tag", "v1")
	app.Git("switch", "--quiet", "-c", "v1")
	app.Commit("v1.txt", "1", "v1 one")
	app.Commit("v2.txt", "2", "v1 two")
	app.Commit("v3.txt", "3", "v1 three")

	w := readWork(t, env, app)

	want := []repo.LocalOnlyBranch{{Name: "v1", Commits: 3, NoUpstream: true}}
	if diff := cmp.Diff(want, w.LocalOnly); diff != "" {
		t.Fatalf("LocalOnly mismatch (-want +got):\n%s", diff)
	}

	g := env.Runner()

	loc, err := repo.Locate(t.Context(), g, app.Path)
	if err != nil {
		t.Fatal(err)
	}

	for _, ref := range []string{"v1", "refs/heads/v1"} {
		got, err := repo.OnlyHereCommits(t.Context(), g, loc, ref, 10)
		if err != nil {
			t.Fatal(err)
		}

		if len(got) != 3 {
			t.Errorf("OnlyHereCommits(%q) = %d commits, want 3", ref, len(got))
		}
	}

	sha := app.Git("rev-parse", "HEAD")

	got, err := repo.OnlyHereCommits(t.Context(), g, loc, sha, 10)
	if err != nil || len(got) != 3 {
		t.Errorf("OnlyHereCommits(sha) = %d commits, err %v, want 3", len(got), err)
	}
}

func TestReadWorkStashSeparatorInMessage(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	_, app := pushedMain(t, env)

	app.Write("base.txt", "changed")
	app.Git("stash", "push", "--quiet", "-m", "wip\x1fodd")

	w := readWork(t, env, app)

	if len(w.Stashes) != 1 || w.Stashes[0].Subject != "On main: wip\x1fodd" {
		t.Fatalf("Stashes = %+v, want one with the separator kept in the subject", w.Stashes)
	}
}
