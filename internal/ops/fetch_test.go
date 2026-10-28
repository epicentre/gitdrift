package ops_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/epicentre/gitdrift/internal/ops"
)

func TestFetchPrunes(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.repo.Git("push", "--quiet", "origin", "main:refs/heads/feature/x")
	f.fetch()

	if f.repo.Git("for-each-ref", "refs/remotes/origin/feature/x") == "" {
		t.Fatal("remote branch was not fetched")
	}

	f.mate.Git("push", "--quiet", "origin", "--delete", "feature/x")
	f.fetch()

	if got := f.repo.Git("for-each-ref", "refs/remotes/origin/feature/x"); got != "" {
		t.Fatalf("pruned ref still present: %q", got)
	}
}

func TestFetchRefusesUnsafeRefspec(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.repo.Git("config", "--add", "remote.origin.fetch", "+refs/heads/*:refs/heads/*")
	f.repo.Git("branch", "work")
	f.repo.Git("checkout", "--quiet", "work")
	sha := f.repo.Commit("local.txt", "unpushed\n", "unpushed work")
	f.repo.Git("checkout", "--quiet", "--detach", "main")

	err := ops.Fetch(t.Context(), f.g, f.loc, "origin")

	var unsafe *ops.UnsafeRefspecError
	if !errors.As(err, &unsafe) {
		t.Fatalf("err = %v, want *UnsafeRefspecError", err)
	}

	if unsafe.Remote != "origin" || unsafe.Refspec != "+refs/heads/*:refs/heads/*" {
		t.Fatalf("error fields = %+v", unsafe)
	}

	if got := f.repo.Git("rev-parse", "refs/heads/work"); got != sha {
		t.Fatalf("branch work = %s, want %s", got, sha)
	}
}

func TestFetchKeepsLocalTagWithPruneTags(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.repo.Git("config", "fetch.pruneTags", "true")
	f.repo.Git("tag", "local-only")
	f.fetch()

	if got := f.repo.Git("tag", "--list", "local-only"); got != "local-only" {
		t.Fatalf("local-only tag was deleted, list = %q", got)
	}
}

func TestUnsafeRefspec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		refspec string
		unsafe  bool
	}{
		{"default", "+refs/heads/*:refs/remotes/origin/*", false},
		{"no plus", "refs/heads/*:refs/remotes/origin/*", false},
		{"negative", "^refs/heads/skip", false},
		{"no colon", "refs/heads/main", false},
		{"empty destination", "refs/heads/main:", false},
		{"heads destination", "+refs/heads/*:refs/heads/*", true},
		{"tags destination", "refs/tags/*:refs/tags/*", true},
		{"lookalike remotes prefix", "+refs/heads/*:refs/remotesX/*", true},
		{"destination is refs/remotes only", "refs/heads/*:refs/remotes", true},
		{"mirror", "+refs/*:refs/*", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ops.UnsafeRefspec(tt.refspec); got != tt.unsafe {
				t.Fatalf("UnsafeRefspec(%q) = %v, want %v", tt.refspec, got, tt.unsafe)
			}
		})
	}
}

func TestFetchRequiresRemoteURL(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.repo.Git("config", "remote.ghost.fetch", "+refs/heads/*:refs/remotes/ghost/*")

	err := ops.Fetch(t.Context(), f.g, f.loc, "ghost")
	if !errors.Is(err, ops.ErrNoRemoteURL) {
		t.Fatalf("err = %v, want ErrNoRemoteURL", err)
	}
}

func TestFetchLegacyRemoteFile(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	bare := f.env.Remote("ghost.git")

	dir := filepath.Join(f.loc.GitDir, "remotes")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}

	legacy := "URL: " + bare + "\nPull: +refs/heads/*:refs/heads/*\n"
	if err := os.WriteFile(filepath.Join(dir, "ghost"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	f.repo.Git("config", "remote.ghost.fetch", "+refs/heads/*:refs/remotes/ghost/*")
	f.repo.Git("checkout", "--quiet", "-b", "work")
	sha := f.repo.Commit("local.txt", "unpushed\n", "unpushed work")
	f.repo.Git("checkout", "--quiet", "--detach", "main")

	err := ops.Fetch(t.Context(), f.g, f.loc, "ghost")
	if !errors.Is(err, ops.ErrNoRemoteURL) {
		t.Fatalf("err = %v, want ErrNoRemoteURL", err)
	}

	if got := f.repo.Git("rev-parse", "refs/heads/work"); got != sha {
		t.Fatalf("branch work = %s, want %s", got, sha)
	}
}
