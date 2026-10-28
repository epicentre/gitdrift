package ops_test

import (
	"testing"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/gittest"
	"github.com/epicentre/gitdrift/internal/ops"
	"github.com/epicentre/gitdrift/internal/repo"
)

// fixture is a clone with one pushed commit, its remote and a teammate clone.
type fixture struct {
	t    *testing.T
	env  *gittest.Env
	repo *gittest.Repo
	mate *gittest.Repo
	g    *git.Runner
	loc  repo.Location
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	env := gittest.New(t)
	remote := env.Remote("origin.git")
	r := env.Clone(remote, "payments")
	r.Commit("README.md", "hello\n", "initial")
	r.Git("push", "--quiet", "-u", "origin", "main")

	mate := env.Clone(remote, "teammate")
	g := env.Runner()

	loc, err := repo.Locate(t.Context(), g, r.Path)
	if err != nil {
		t.Fatal(err)
	}

	return &fixture{t: t, env: env, repo: r, mate: mate, g: g, loc: loc}
}

// teammatePush makes the teammate commit and push n files to main.
func (f *fixture) teammatePush(n int) {
	f.t.Helper()

	for i := range n {
		f.mate.Commit("mate"+string(rune('a'+i))+".txt", "x", "teammate commit")
	}

	f.mate.Git("push", "--quiet", "origin", "main")
}

// fetch runs ops.Fetch for origin.
func (f *fixture) fetch() {
	f.t.Helper()

	if err := ops.Fetch(f.t.Context(), f.g, f.loc, "origin"); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) status() repo.Status {
	f.t.Helper()

	st, err := repo.ReadStatus(f.t.Context(), f.g, f.loc)
	if err != nil {
		f.t.Fatal(err)
	}

	return st
}

// request builds a PullRequest from the current status and a finished fetch.
func (f *fixture) request() ops.PullRequest {
	f.t.Helper()

	st := f.status()

	return ops.PullRequest{
		Loc:         f.loc,
		Branch:      st.Branch,
		HeadSHA:     st.HeadSHA,
		UpstreamSHA: st.UpstreamSHA,
		Fetch:       drift.FetchState{Done: true},
	}
}
