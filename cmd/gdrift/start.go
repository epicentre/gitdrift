package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/ui"
	"github.com/epicentre/gitdrift/internal/workspace"
)

// closeNotice is how long quitting waits for git in silence.
const closeNotice = time.Second

// startTUI runs the dashboard for dir in the terminal on stdout.
func startTUI(ctx context.Context, dir string) error {
	return start(ctx, dir, os.Stdout)
}

// start checks that stdout is a terminal and that git is recent enough,
// discovers the repositories in dir and runs the dashboard until the user
// quits or ctx ends, which it reports as context.Canceled.
func start(ctx context.Context, dir string, stdout *os.File) error {
	if !isTerminal(stdout) {
		return errors.New("an interactive terminal is required")
	}

	g, err := git.NewRunner()
	if err != nil {
		return err
	}

	if err := checkGitVersion(ctx, g); err != nil {
		return err
	}

	groups, err := workspace.Discover(ctx, g, dir)
	if errors.Is(err, workspace.ErrNoRepos) {
		return fmt.Errorf("no git repositories found in %s", dir)
	}

	if err != nil {
		return err
	}

	ws := workspace.New(g, groups, workspace.DefaultOptions())

	return runDashboard(ctx, ws, dir, os.Stderr, closeNotice, tea.WithOutput(stdout))
}

// isTerminal reports whether f is a character device, as a terminal is.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()

	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// checkGitVersion refuses a git older than git.MinVersion.
func checkGitVersion(ctx context.Context, g *git.Runner) error {
	v, err := g.Version(ctx)
	if err != nil {
		return fmt.Errorf("check git version: %w", err)
	}

	if !v.AtLeast(git.MinVersion) {
		return fmt.Errorf("git %d.%d or newer is required (found %s)", git.MinVersion.Major, git.MinVersion.Minor, v)
	}

	return nil
}

// runDashboard runs the dashboard over ws until it ends. Then it cancels the
// program context, so that refreshes stop, and waits for every git command
// still running: a pull's merge and a branch deletion are never cancelled,
// and exiting would close their output pipes, so that git could die from
// SIGPIPE halfway through. Waiting longer than notice is said on stderr. A
// program ended by ctx is reported as context.Canceled.
func runDashboard(
	ctx context.Context, ws *workspace.Workspace, root string, stderr io.Writer, notice time.Duration,
	opts ...tea.ProgramOption,
) error {
	progCtx, cancel := context.WithCancel(ctx)

	// main turns SIGINT and SIGTERM into the cancellation of ctx; a second
	// handler in the program would race it for the exit code.
	opts = append([]tea.ProgramOption{tea.WithContext(progCtx), tea.WithoutSignalHandler()}, opts...)
	_, err := tea.NewProgram(ui.New(progCtx, ws, root), opts...).Run()

	cancel()
	waitForGit(ws.Close, stderr, notice)

	switch {
	case errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil:
		return context.Canceled
	case err != nil:
		return fmt.Errorf("run dashboard: %w", err)
	default:
		return nil
	}
}

// waitForGit calls closeWorkspace and returns once it does. When that takes
// longer than notice it says once on stderr what gdrift waits for; the
// program has restored the terminal by then.
func waitForGit(closeWorkspace func(), stderr io.Writer, notice time.Duration) {
	done := make(chan struct{})

	go func() {
		defer close(done)

		closeWorkspace()
	}()

	timer := time.NewTimer(notice)
	defer timer.Stop()

	select {
	case <-done:
		return
	case <-timer.C:
		writeLine(stderr, "waiting for git to finish…")
	}

	<-done
}
