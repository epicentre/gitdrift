package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

const (
	defaultTimeout = 30 * time.Second
	waitDelay      = 3 * time.Second
)

// Runner starts allowlisted git processes.
type Runner struct {
	// Binary is the path of the git executable.
	Binary string
	// Env holds extra KEY=VALUE entries appended to os.Environ().
	Env []string
	// DefaultTimeout applies when the context has no deadline; zero means none.
	DefaultTimeout time.Duration

	// baseEnv supplies the inherited environment; nil means os.Environ.
	baseEnv func() []string
}

// repoEnvVars select a repository and are never inherited, so every command
// runs against the directory it is given.
var repoEnvVars = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_NAMESPACE", "GIT_PREFIX",
}

// NewRunner locates git on PATH.
func NewRunner() (*Runner, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git not found: %w", err)
	}

	return &Runner{Binary: bin, DefaultTimeout: defaultTimeout}, nil
}

// CommandError describes a failed git invocation.
type CommandError struct {
	Args     []string
	ExitCode int
	Stderr   string
	// Stdout is the output captured before the failure. Some commands report
	// a result through a non-zero exit code (for example `diff --no-index`
	// exits 1 when the files differ), so callers may need it.
	Stdout []byte
	Err    error
}

// Error returns "git <subcommand>: <first stderr line or cause>".
func (e *CommandError) Error() string {
	name := ""
	if len(e.Args) > 0 {
		name = e.Args[0]
	}

	for line := range strings.SplitSeq(e.Stderr, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return "git " + name + ": " + line
		}
	}

	if e.Err != nil {
		return "git " + name + ": " + e.Err.Error()
	}

	return "git " + name + ": failed"
}

// Unwrap returns the underlying cause.
func (e *CommandError) Unwrap() error { return e.Err }

// ExitCode returns the exit code carried by err, or -1 when err is not a
// *CommandError.
func ExitCode(err error) int {
	if ce, ok := errors.AsType[*CommandError](err); ok {
		return ce.ExitCode
	}

	return -1
}

// Run executes git with args in dir and returns stdout. The argv is checked
// by Classify first; a refused argv never starts a process.
func (r *Runner) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	kind, err := Classify(args)
	if err != nil {
		return nil, err
	}

	if _, ok := ctx.Deadline(); !ok && r.DefaultTimeout > 0 {
		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeout(ctx, r.DefaultTimeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, r.Binary, args...) //nolint:gosec // argv is validated by Classify and never passed to a shell
	cmd.Dir = dir
	cmd.Env = r.environ(kind)
	cmd.WaitDelay = waitDelay
	release := isolate(cmd)

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	release()

	if runErr == nil {
		return stdout.Bytes(), nil
	}

	// A descendant (for example an ssh control master) may keep the pipes open
	// after git itself succeeded; the output captured so far is complete.
	if errors.Is(runErr, exec.ErrWaitDelay) && cmd.ProcessState != nil && cmd.ProcessState.Success() {
		return stdout.Bytes(), nil
	}

	ce := &CommandError{Args: args, ExitCode: -1, Stderr: stderr.String(), Stdout: stdout.Bytes(), Err: runErr}
	if cmd.ProcessState != nil {
		ce.ExitCode = cmd.ProcessState.ExitCode()
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		ce.Err = ctxErr
	}

	return nil, ce
}

func (r *Runner) environ(kind Kind) []string {
	base := os.Environ
	if r.baseEnv != nil {
		base = r.baseEnv
	}

	var env []string

	for _, kv := range base() {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(repoEnvVars, name) {
			env = append(env, kv)
		}
	}

	env = append(env, r.Env...)
	env = append(env, "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")

	if kind == KindRead {
		env = append(env, "GIT_OPTIONAL_LOCKS=0")
	}

	return env
}
