package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/epicentre/gitdrift/internal/gittest"
	"github.com/epicentre/gitdrift/internal/workspace"
)

// noNotice keeps waitForGit silent in tests that assert an empty stderr: a
// loaded race-detector run may take longer than closeNotice to close.
const noNotice = time.Hour

// devNull is a character device that is no terminal: it passes the terminal
// check, so start goes on to git and discovery.
func devNull(t *testing.T) *os.File {
	t.Helper()

	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = f.Close() })

	return f
}

func TestStartRequiresTerminal(t *testing.T) {
	t.Parallel()

	f, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = f.Close() })

	err = start(t.Context(), t.TempDir(), f)
	if want := "an interactive terminal is required"; err == nil || err.Error() != want {
		t.Fatalf("start err = %v, want %q", err, want)
	}
}

func TestStartNoRepos(t *testing.T) {
	t.Parallel()

	dir := resolved(t, t.TempDir())

	err := start(t.Context(), dir, devNull(t))
	if want := "no git repositories found in " + dir; err == nil || err.Error() != want {
		t.Fatalf("start err = %v, want %q", err, want)
	}
}

// TestStartOldGit changes PATH, so it cannot run in parallel.
func TestStartOldGit(t *testing.T) {
	bin := t.TempDir()
	script := filepath.Join(bin, "git")

	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'git version 2.29.2'\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(script, 0o700); err != nil { //nolint:gosec // the fake git must be executable
		t.Fatal(err)
	}

	t.Setenv("PATH", bin)

	err := start(t.Context(), t.TempDir(), devNull(t))
	if want := "git 2.30 or newer is required (found 2.29.2)"; err == nil || err.Error() != want {
		t.Fatalf("start err = %v, want %q", err, want)
	}
}

func TestRunDashboard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   io.Reader
		cancel  bool
		wantErr error
	}{
		{name: "quit key", input: strings.NewReader("q")},
		{name: "interrupted", cancel: true, wantErr: context.Canceled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := gittest.New(t)
			clone := env.Clone(env.Remote("api.git"), "api")
			clone.Commit("README.md", "hello\n", "initial")
			clone.Git("push", "--quiet", "-u", "origin", "main")

			groups, err := workspace.Discover(t.Context(), env.Runner(), env.WorkDir())
			if err != nil {
				t.Fatal(err)
			}

			ws := workspace.New(env.Runner(), groups, workspace.DefaultOptions())

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			if tt.cancel {
				cancel()
			}

			var stderr bytes.Buffer

			err = runDashboard(ctx, ws, env.WorkDir(), &stderr, noNotice,
				tea.WithInput(tt.input), tea.WithOutput(io.Discard))
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("runDashboard err = %v, want %v", err, tt.wantErr)
			}

			// The workspace is closed: no git runs once gdrift exits.
			if snaps := ws.Refresh(t.Context(), groups[0].CommonDir, false); snaps != nil {
				t.Fatalf("Refresh after runDashboard = %d snapshots, want none", len(snaps))
			}

			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

// TestRunDashboardStopsFetchOnQuit quits while the first fetch hangs. The
// program context is cancelled before the workspace closes, so the fetch is
// killed instead of awaited.
func TestRunDashboardStopsFetchOnQuit(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	clone := env.Clone(env.Remote("api.git"), "api")
	clone.Commit("README.md", "hello\n", "initial")
	clone.Git("push", "--quiet", "-u", "origin", "main")

	bin := t.TempDir()
	script := filepath.Join(bin, "slow-upload-pack")
	started := filepath.Join(bin, "started")
	body := "#!/bin/sh\ntouch \"$(dirname \"$0\")/started\"\nsleep 30\nexec git upload-pack \"$@\"\n"

	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(script, 0o700); err != nil { //nolint:gosec // the test script must be executable
		t.Fatal(err)
	}

	clone.Git("config", "--local", "remote.origin.uploadpack", script)

	groups, err := workspace.Discover(t.Context(), env.Runner(), env.WorkDir())
	if err != nil {
		t.Fatal(err)
	}

	ws := workspace.New(env.Runner(), groups, workspace.DefaultOptions())
	keys, typing := io.Pipe()
	stop := make(chan struct{})
	typed := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)

		typed <- typeQuitOnce(started, typing, stop)
	}()

	// Whatever happens to the test, the typing goroutine ends with it: stop
	// ends its wait and closing the reader ends a write nobody reads.
	t.Cleanup(func() {
		close(stop)

		_ = keys.Close()

		<-done
	})

	var stderr bytes.Buffer

	begin := time.Now()

	err = runDashboard(t.Context(), ws, env.WorkDir(), &stderr, noNotice, tea.WithInput(keys), tea.WithOutput(io.Discard))
	if err != nil {
		t.Fatalf("runDashboard err = %v, want nil", err)
	}

	if d := time.Since(begin); d > 10*time.Second {
		t.Fatalf("runDashboard took %s, want the hanging fetch killed on quit", d)
	}

	if err := <-typed; err != nil {
		t.Fatal(err)
	}
}

// typeQuitOnce types q into w once the file started exists, then closes w.
// It gives up waiting when stop closes; after 10 seconds it types q anyway,
// so that the program ends, and reports that the file never appeared.
func typeQuitOnce(started string, w *io.PipeWriter, stop <-chan struct{}) error {
	defer func() { _ = w.Close() }()

	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()

	var waitErr error

wait:
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}

		select {
		case <-stop:
			return errors.New("the test ended before the fetch started")
		case <-timeout.C:
			waitErr = errors.New("the fetch never started")

			break wait
		case <-tick.C:
		}
	}

	if _, err := w.Write([]byte("q")); err != nil {
		return errors.Join(waitErr, fmt.Errorf("type q: %w", err))
	}

	return waitErr
}

func TestWaitForGit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		busy       time.Duration
		wantStderr string
	}{
		{name: "quick close is silent", busy: closeNotice - time.Millisecond},
		{name: "slow close says so once", busy: 5 * closeNotice, wantStderr: "waiting for git to finish…\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				var stderr bytes.Buffer

				begin := time.Now()

				waitForGit(func() { time.Sleep(tt.busy) }, &stderr, closeNotice)

				if got := time.Since(begin); got != tt.busy {
					t.Errorf("waitForGit returned after %s, want %s", got, tt.busy)
				}

				if stderr.String() != tt.wantStderr {
					t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantStderr)
				}
			})
		})
	}
}
