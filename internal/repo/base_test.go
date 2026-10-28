package repo_test

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/gittest"
	"github.com/epicentre/gitdrift/internal/repo"
)

func resolveBase(t *testing.T, env *gittest.Env, r *gittest.Repo) repo.Base {
	t.Helper()

	g := env.Runner()
	st := readStatus(t, env, r)

	loc, err := repo.Locate(t.Context(), g, r.Path)
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.ResolveBase(t.Context(), g, loc, st)
	if err != nil {
		t.Fatal(err)
	}

	return got
}

// pushBranch creates branch name at HEAD and pushes it to remote.
func pushBranch(r *gittest.Repo, remote, name string) {
	r.Git("push", "--quiet", remote, "HEAD:refs/heads/"+name)
}

type baseCase struct {
	name  string
	setup func(t *testing.T) (*gittest.Env, *gittest.Repo)
	want  repo.Base
}

const (
	originMain = "origin/main"
	originName = "origin"
)

func autoCases() []baseCase {
	return []baseCase{
		{
			name: "auto via origin/HEAD",
			setup: func(t *testing.T) (*gittest.Env, *gittest.Repo) {
				t.Helper()

				env, _, r := newClone(t, "payments")
				r.Git("remote", "set-head", originName, "main")

				return env, r
			},
			want: repo.Base{Source: repo.SourceAuto, Ref: originMain, OnBase: true},
		},
		{
			name: "auto fallback master",
			setup: func(t *testing.T) (*gittest.Env, *gittest.Repo) {
				t.Helper()

				env := gittest.New(t)
				r := env.Clone(env.Remote("origin.git"), "payments")
				r.Git("branch", "-m", "master")
				r.Commit("README.md", "hello\n", "initial")
				r.Git("push", "--quiet", "-u", originName, "master")
				_, _ = r.TryGit("symbolic-ref", "--delete", "refs/remotes/origin/HEAD")

				return env, r
			},
			want: repo.Base{Source: repo.SourceAuto, Ref: "origin/master", OnBase: true},
		},
		{
			name: "auto single non-origin remote",
			setup: func(t *testing.T) (*gittest.Env, *gittest.Repo) {
				t.Helper()

				env := gittest.New(t)
				r := env.Init("payments")
				r.Commit("README.md", "hello\n", "initial")
				r.Git("remote", "add", "upstream", env.Remote("upstream.git"))
				r.Git("push", "--quiet", "upstream", "main")
				r.Git("fetch", "--quiet", "upstream")

				return env, r
			},
			want: repo.Base{Source: repo.SourceAuto, Ref: "upstream/main", OnBase: true},
		},
		{
			name: "feature behind base",
			setup: func(t *testing.T) (*gittest.Env, *gittest.Repo) {
				t.Helper()

				env, remote, r := newClone(t, "payments")
				r.Git("remote", "set-head", originName, "main")
				r.Git("checkout", "--quiet", "-b", "feature/a")
				r.Commit("a.txt", "a", "feature work")
				r.Git("push", "--quiet", "-u", originName, "feature/a")
				teammate(t, env, remote, 3)
				r.Git("fetch", "--quiet")

				return env, r
			},
			want: repo.Base{Source: repo.SourceAuto, Ref: originMain, Behind: 3},
		},
		{
			name: "no remotes",
			setup: func(t *testing.T) (*gittest.Env, *gittest.Repo) {
				t.Helper()

				env := gittest.New(t)
				r := env.Init("payments")
				r.Commit("README.md", "hello\n", "initial")

				return env, r
			},
			want: repo.Base{Source: repo.SourceAuto},
		},
	}
}

func configuredCases() []baseCase {
	return []baseCase{
		{
			name: "repo setting",
			setup: func(t *testing.T) (*gittest.Env, *gittest.Repo) {
				t.Helper()

				env, _, r := newClone(t, "payments")
				pushBranch(r, originName, "develop")
				r.Git("fetch", "--quiet")
				r.Git("config", "--local", "gitdrift.base", "develop")

				return env, r
			},
			want: repo.Base{Configured: "develop", Source: repo.SourceRepo, Ref: "origin/develop"},
		},
		{
			name: "global setting",
			setup: func(t *testing.T) (*gittest.Env, *gittest.Repo) {
				t.Helper()

				env, _, r := newClone(t, "payments")
				pushBranch(r, originName, "develop")
				r.Git("fetch", "--quiet")
				env.GlobalConfig("gitdrift.base", "develop")

				return env, r
			},
			want: repo.Base{Configured: "develop", Source: repo.SourceGlobal, Ref: "origin/develop"},
		},
		{
			name: "remote-qualified",
			setup: func(t *testing.T) (*gittest.Env, *gittest.Repo) {
				t.Helper()

				env, _, r := newClone(t, "payments")
				r.Git("remote", "add", "upstream", env.Remote("upstream.git"))
				pushBranch(r, "upstream", "release")
				r.Git("fetch", "--quiet", "upstream")
				r.Git("config", "--local", "gitdrift.base", "upstream/release")

				return env, r
			},
			want: repo.Base{Configured: "upstream/release", Source: repo.SourceRepo, Ref: "upstream/release"},
		},
		{
			name: "slashed branch name",
			setup: func(t *testing.T) (*gittest.Env, *gittest.Repo) {
				t.Helper()

				env, _, r := newClone(t, "payments")
				pushBranch(r, originName, "release/2026")
				r.Git("fetch", "--quiet")
				r.Git("config", "--local", "gitdrift.base", "release/2026")

				return env, r
			},
			want: repo.Base{Configured: "release/2026", Source: repo.SourceRepo, Ref: "origin/release/2026"},
		},
		{
			name: "configured but missing",
			setup: func(t *testing.T) (*gittest.Env, *gittest.Repo) {
				t.Helper()

				env, _, r := newClone(t, "payments")
				r.Git("config", "--local", "gitdrift.base", "nope")

				return env, r
			},
			want: repo.Base{Configured: "nope", Source: repo.SourceRepo, NotFound: true},
		},
	}
}

func TestResolveBase(t *testing.T) {
	t.Parallel()

	tests := slices.Concat(autoCases(), configuredCases())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env, r := tt.setup(t)
			got := resolveBase(t, env, r)

			if tt.want.Ref != "" {
				tt.want.SHA = r.Git("rev-parse", "refs/remotes/"+tt.want.Ref)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("base mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRemoteBranches(t *testing.T) {
	t.Parallel()

	env, _, r := newClone(t, "payments")
	pushBranch(r, originName, "develop")
	r.Git("remote", "set-head", originName, "main")
	r.Git("fetch", "--quiet")

	g := env.Runner()

	loc, err := repo.Locate(t.Context(), g, r.Path)
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.RemoteBranches(t.Context(), g, loc)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"origin/develop", originMain}
	if !slices.Equal(got, want) {
		t.Errorf("RemoteBranches = %v, want %v", got, want)
	}
}

func TestReferenceRemote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		st   repo.Status
		want string
	}{
		{"upstream remote", repo.Status{UpstreamRemote: "fork", Remotes: []string{originName, "fork"}}, "fork"},
		{"local upstream falls back to origin", repo.Status{UpstreamRemote: ".", Remotes: []string{"a", originName}}, originName},
		{"only remote", repo.Status{Remotes: []string{"upstream"}}, "upstream"},
		{"ambiguous", repo.Status{Remotes: []string{"a", "b"}}, ""},
		{"none", repo.Status{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := repo.ReferenceRemote(tt.st); got != tt.want {
				t.Errorf("ReferenceRemote = %q, want %q", got, tt.want)
			}
		})
	}
}
