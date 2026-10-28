package ops_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/ops"
)

func TestSetAndResetBase(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	ctx := t.Context()

	if err := ops.SetBase(ctx, f.g, f.loc, "develop"); err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(f.repo.Git("config", "--local", "--get", "gitdrift.base")); got != "develop" {
		t.Fatalf("gitdrift.base = %q, want develop", got)
	}

	if err := ops.ResetBase(ctx, f.g, f.loc); err != nil {
		t.Fatal(err)
	}

	if _, err := f.repo.TryGit("config", "--local", "--get", "gitdrift.base"); err == nil {
		t.Fatal("gitdrift.base still set after reset")
	}

	if err := ops.ResetBase(ctx, f.g, f.loc); err != nil {
		t.Fatalf("second ResetBase = %v, want nil", err)
	}

	if err := ops.SetBase(ctx, f.g, f.loc, "-x"); !errors.Is(err, git.ErrNotAllowed) {
		t.Fatalf("SetBase(-x) = %v, want ErrNotAllowed", err)
	}
}

func TestResetBaseMultipleValues(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.repo.Git("config", "--local", "--add", "gitdrift.base", "develop")
	f.repo.Git("config", "--local", "--add", "gitdrift.base", "staging")

	if err := ops.ResetBase(t.Context(), f.g, f.loc); err == nil {
		t.Fatal("ResetBase = nil while the base is still set")
	}

	if got := f.repo.Git("config", "--local", "--get-all", "gitdrift.base"); !strings.Contains(got, "develop") {
		t.Fatalf("values changed: %q", got)
	}
}

// slowGit returns a copy of g whose git announces each call by creating
// marker, then waits a second before running.
func slowGit(t *testing.T, g *git.Runner, marker string) *git.Runner {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "slowgit.sh")
	script := "#!/bin/sh\ntouch '" + marker + "'\nsleep 1\nexec '" + g.Binary + "' \"$@\"\n"

	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec // test script must be executable
		t.Fatal(err)
	}

	slow := *g
	slow.Binary = bin

	return &slow
}

// TestBaseWritesFinishOnCancel cancels the caller while git config runs: the
// write completes instead of being killed with config.lock held.
func TestBaseWritesFinishOnCancel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(f *fixture)
		write   func(ctx context.Context, f *fixture, g *git.Runner) error
		want    string
	}{
		{
			name: "set",
			write: func(ctx context.Context, f *fixture, g *git.Runner) error {
				return ops.SetBase(ctx, g, f.loc, "develop")
			},
			want: "develop",
		},
		{
			name:    "reset",
			prepare: func(f *fixture) { f.repo.Git("config", "--local", "gitdrift.base", "develop") },
			write:   func(ctx context.Context, f *fixture, g *git.Runner) error { return ops.ResetBase(ctx, g, f.loc) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			if tt.prepare != nil {
				tt.prepare(f)
			}

			marker := filepath.Join(t.TempDir(), "started")
			g := slowGit(t, f.g, marker)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			done := make(chan error, 1)

			go func() { done <- tt.write(ctx, f, g) }()

			waitForFile(t, marker)
			cancel()

			if err := <-done; err != nil {
				t.Fatalf("write = %v, want it to finish", err)
			}

			got, _ := f.repo.TryGit("config", "--local", "--get", "gitdrift.base")
			if strings.TrimSpace(got) != tt.want {
				t.Fatalf("gitdrift.base = %q, want %q", got, tt.want)
			}

			if locks := lockFiles(t, filepath.Join(f.repo.Path, ".git")); len(locks) != 0 {
				t.Fatalf("lock files left: %v", locks)
			}
		})
	}
}

func TestBaseWritesRefuseCancelledCaller(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.repo.Git("config", "--local", "gitdrift.base", "develop")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := ops.SetBase(ctx, f.g, f.loc, "staging"); !errors.Is(err, context.Canceled) {
		t.Errorf("SetBase = %v, want context.Canceled", err)
	}

	if err := ops.ResetBase(ctx, f.g, f.loc); !errors.Is(err, context.Canceled) {
		t.Errorf("ResetBase = %v, want context.Canceled", err)
	}

	if got := strings.TrimSpace(f.repo.Git("config", "--local", "--get", "gitdrift.base")); got != "develop" {
		t.Fatalf("gitdrift.base = %q, want develop", got)
	}
}

// waitForFile polls until path exists.
func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)

	for {
		if _, err := os.Stat(path); err == nil {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("%s was never created", path)
		}

		time.Sleep(10 * time.Millisecond)
	}
}
