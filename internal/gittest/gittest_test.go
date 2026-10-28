package gittest_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/epicentre/gitdrift/internal/gittest"
)

func TestCommitAndClone(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	remote := env.Remote("origin.git")
	a := env.Clone(remote, "a")

	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	sha := a.CommitAt("f.txt", "one", "first", when)

	if len(sha) != 40 {
		t.Fatalf("sha = %q, want 40 hex chars", sha)
	}

	a.Git("push", "--quiet", "-u", "origin", "main")

	b := env.Clone(remote, "b")
	if got := b.Git("rev-parse", "HEAD"); got != sha {
		t.Fatalf("clone HEAD = %q, want %q", got, sha)
	}

	if got := b.Git("log", "-1", "--format=%an <%ae> %ct"); got != "Test <test@example.com> "+strconv.FormatInt(when.Unix(), 10) {
		t.Fatalf("log = %q", got)
	}
}

func TestInitUsesIsolatedConfig(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	r := env.Init("fresh")

	if got := r.Git("symbolic-ref", "--short", "HEAD"); got != "main" {
		t.Fatalf("default branch = %q, want main", got)
	}

	env.GlobalConfig("user.name", "Other")

	if got := r.Git("config", "user.name"); got != "Other" {
		t.Fatalf("user.name = %q, want Other", got)
	}
}

func TestRunnerCarriesIsolationEnv(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	r := env.Init("fresh")

	out, err := env.Runner().Run(t.Context(), r.Path, "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(string(out)); got != r.Path {
		t.Fatalf("toplevel = %q, want %q", got, r.Path)
	}

	found := false

	for _, kv := range env.Runner().Env {
		if strings.HasPrefix(kv, "GIT_CONFIG_NOSYSTEM=") {
			found = true
		}
	}

	if !found {
		t.Fatal("Runner.Env lacks GIT_CONFIG_NOSYSTEM")
	}
}

func TestWriteCreatesParents(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	r := env.Init("fresh")
	r.Write("a/b/c.txt", "x")

	if _, err := os.Stat(filepath.Join(r.Path, "a", "b", "c.txt")); err != nil {
		t.Fatal(err)
	}
}
