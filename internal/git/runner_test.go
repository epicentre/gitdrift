package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRunner returns a Runner whose binary is a copy of testdata/fakegit.sh.
func fakeRunner(t *testing.T, mode string, env ...string) *Runner {
	t.Helper()

	src, err := os.ReadFile(filepath.Join("testdata", "fakegit.sh"))
	if err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(t.TempDir(), "fakegit.sh")
	if err := os.WriteFile(bin, src, 0o755); err != nil { //nolint:gosec // test script must be executable
		t.Fatal(err)
	}

	return &Runner{
		Binary: bin,
		Env:    append([]string{"FAKEGIT_MODE=" + mode}, env...),
		baseEnv: func() []string {
			return []string{"PATH=" + os.Getenv("PATH"), "KEEP_ME=1", "GIT_DIR=/parent/repo", "GIT_INDEX_FILE=/parent/index"}
		},
	}
}

func TestRunnerEnvRead(t *testing.T) {
	t.Parallel()

	r := fakeRunner(t, "env", "GIT_SSH_COMMAND=ssh -i keep")

	out, err := r.Run(t.Context(), t.TempDir(), "status")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_OPTIONAL_LOCKS=0", "GIT_SSH_COMMAND=ssh -i keep",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("env lacks %q", want)
		}
	}
}

func TestRunnerEnvForcedValuesWin(t *testing.T) {
	t.Parallel()

	r := fakeRunner(t, "env", "GIT_TERMINAL_PROMPT=1", "GIT_OPTIONAL_LOCKS=1")

	out, err := r.Run(t.Context(), t.TempDir(), "status")
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(string(out), "\n")
	last := func(key string) string {
		v := ""

		for _, l := range lines {
			if strings.HasPrefix(l, key+"=") {
				v = l
			}
		}

		return v
	}

	// Duplicate keys are resolved by the last entry, so the forced value must come last.
	if got := last("GIT_TERMINAL_PROMPT"); got != "GIT_TERMINAL_PROMPT=0" {
		t.Errorf("GIT_TERMINAL_PROMPT = %q", got)
	}

	if got := last("GIT_OPTIONAL_LOCKS"); got != "GIT_OPTIONAL_LOCKS=0" {
		t.Errorf("GIT_OPTIONAL_LOCKS = %q", got)
	}
}

func TestRunnerEnvInheritsAndStripsRepoVars(t *testing.T) {
	t.Parallel()

	r := fakeRunner(t, "env")

	out, err := r.Run(t.Context(), t.TempDir(), "status")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), "KEEP_ME=1") {
		t.Error("inherited variable was dropped")
	}

	for _, banned := range []string{"GIT_DIR=", "GIT_INDEX_FILE="} {
		if strings.Contains(string(out), banned) {
			t.Errorf("env contains %s", banned)
		}
	}
}

func TestRunnerEnvWrite(t *testing.T) {
	t.Parallel()

	r := fakeRunner(t, "env")

	out, err := r.Run(t.Context(), t.TempDir(), "fetch", "--prune", "--no-prune-tags", "--quiet", "--end-of-options", "origin")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out), "GIT_TERMINAL_PROMPT=0") {
		t.Error("env lacks GIT_TERMINAL_PROMPT=0")
	}

	if strings.Contains(string(out), "GIT_OPTIONAL_LOCKS=0") {
		t.Error("write command must not set GIT_OPTIONAL_LOCKS")
	}
}

func TestRunnerRefusesBeforeExec(t *testing.T) {
	t.Parallel()

	marker := filepath.Join(t.TempDir(), "marker")
	r := fakeRunner(t, "marker", "FAKEGIT_MARKER="+marker)

	_, err := r.Run(t.Context(), t.TempDir(), "push", "origin", "main")
	if !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("err = %v, want ErrNotAllowed", err)
	}

	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("process ran: stat err = %v", statErr)
	}
}

func TestRunnerCommandError(t *testing.T) {
	t.Parallel()

	r := fakeRunner(t, "fail")

	_, err := r.Run(t.Context(), t.TempDir(), "status")
	if got := ExitCode(err); got != 128 {
		t.Fatalf("ExitCode = %d, want 128", got)
	}

	if err.Error() != "git status: fatal: boom" {
		t.Fatalf("err = %q", err.Error())
	}

	if got := ExitCode(errors.New("x")); got != -1 {
		t.Fatalf("ExitCode(non-command error) = %d, want -1", got)
	}
}

func TestRunnerDefaultTimeout(t *testing.T) {
	t.Parallel()

	r := fakeRunner(t, "sleep", "FAKEGIT_PIDFILE="+filepath.Join(t.TempDir(), "pid"))
	r.DefaultTimeout = 200 * time.Millisecond

	start := time.Now()

	_, err := r.Run(context.Background(), t.TempDir(), "status")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}

	if time.Since(start) > 5*time.Second {
		t.Fatal("timeout took too long")
	}
}
