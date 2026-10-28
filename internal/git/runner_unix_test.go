//go:build unix

package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

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

// waitForPid polls until the fake git has written its child's pid to path.
// The shell creates the file before it writes to it, so the file existing is
// not enough: a cancel in between would leave it empty.
func waitForPid(t *testing.T, path string) int {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)

	for {
		raw, err := os.ReadFile(path) //nolint:gosec // path is a test temp file
		if err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
				return pid
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("%s never held a pid", path)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// waitGone polls until pid no longer exists.
func waitGone(t *testing.T, pid int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
		if time.Now().After(deadline) {
			t.Fatalf("child %d still alive", pid)
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// runAsync runs status with r under ctx and delivers the error of Run.
func runAsync(ctx context.Context, t *testing.T, r *Runner) <-chan error {
	t.Helper()

	done := make(chan error, 1)
	dir := t.TempDir()

	go func() {
		_, err := r.Run(ctx, dir, "status")
		done <- err
	}()

	return done
}

func TestRunnerTimeoutKillsProcessGroup(t *testing.T) {
	t.Parallel()

	pidFile := filepath.Join(t.TempDir(), "pid")
	r := fakeRunner(t, "sleep", "FAKEGIT_PIDFILE="+pidFile)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := runAsync(ctx, t, r)

	// Cancel only once the child exists, so that it is the group that dies.
	pid := waitForPid(t, pidFile)

	start := time.Now()

	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}

	if time.Since(start) > 5*time.Second {
		t.Fatal("run did not return promptly")
	}

	waitGone(t, pid)
}

func TestRunnerCancelLetsGitRemoveLocks(t *testing.T) {
	t.Parallel()

	lock := filepath.Join(t.TempDir(), "main.lock")
	r := fakeRunner(t, "lock", "FAKEGIT_LOCK="+lock)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := runAsync(ctx, t, r)

	waitForFile(t, lock)
	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}

	if _, err := os.Stat(lock); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock file left behind: stat err = %v", err)
	}
}

func TestRunnerCancelKillsGroupThatIgnoresTerm(t *testing.T) {
	t.Parallel()

	pidFile := filepath.Join(t.TempDir(), "pid")
	r := fakeRunner(t, "ignoreterm", "FAKEGIT_PIDFILE="+pidFile)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := runAsync(ctx, t, r)

	pid := waitForPid(t, pidFile)

	start := time.Now()

	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}

	if elapsed := time.Since(start); elapsed < killGrace {
		t.Fatalf("run returned after %v, before the %v grace", elapsed, killGrace)
	} else if elapsed > 10*time.Second {
		t.Fatalf("run returned after %v", elapsed)
	}

	// exec only kills git itself; the child dies with the group.
	waitGone(t, pid)
}

func TestCancelAfterExitIsProcessDone(t *testing.T) {
	t.Parallel()

	cmd := exec.CommandContext(t.Context(), "true")
	defer isolate(cmd)()

	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	if err := cmd.Cancel(); !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("Cancel after exit = %v, want os.ErrProcessDone", err)
	}
}

func TestRunnerSucceedsWhenDescendantHoldsPipe(t *testing.T) {
	t.Parallel()

	pidFile := filepath.Join(t.TempDir(), "pid")
	r := fakeRunner(t, "bgout", "FAKEGIT_PIDFILE="+pidFile)

	t.Cleanup(func() {
		raw, err := os.ReadFile(pidFile) //nolint:gosec // path is a test temp file
		if err != nil {
			return
		}

		if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})

	start := time.Now()

	out, err := r.Run(t.Context(), t.TempDir(), "status")
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	if strings.TrimSpace(string(out)) != "out" {
		t.Fatalf("out = %q", out)
	}

	if time.Since(start) > 5*time.Second {
		t.Fatal("run took too long")
	}
}
