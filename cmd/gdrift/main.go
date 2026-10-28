// Command gdrift is a terminal dashboard for drift across the git
// repositories in one folder.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

// Exit codes.
const (
	exitOK          = 0
	exitError       = 1
	exitUsage       = 2
	exitInterrupted = 130
)

const usageText = `Usage: gdrift [flags] [dir]

gdrift shows drift across the git repositories found in dir (default: the
current directory).

Flags:
  --version  print version information and exit

Git config keys (set with git config --local or --global):
  gitdrift.base   base branch used to measure how far a branch fell behind
  gitdrift.badge  what to show as the badge after a pull
`

// StartFunc runs the interactive program for the workspace in dir.
type StartFunc func(ctx context.Context, dir string) error

// quitSignals end gdrift through the cancellation of its context, so that it
// quits as it does on q: it waits for running git instead of being killed.
// SIGHUP arrives when the terminal closes.
var quitSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), quitSignals...)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr, startTUI)

	stop()
	os.Exit(code)
}

// run parses args, resolves the workspace directory and calls start. It
// returns the process exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer, start StartFunc) int {
	usage := func() { _, _ = io.WriteString(stderr, usageText) }

	fs := flag.NewFlagSet("gdrift", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	// Parse would call Usage itself on -h and on bad flags; run prints the
	// usage once, after any error message.
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version information and exit")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			usage()

			return exitOK
		}

		printf(stderr, "gdrift: %v\n", err)
		usage()

		return exitUsage
	}

	if *showVersion {
		writeLine(stdout, versionString())

		return exitOK
	}

	if fs.NArg() > 1 {
		writeLine(stderr, "gdrift: at most one directory argument")
		usage()

		return exitUsage
	}

	dir, err := resolveDir(fs.Arg(0))
	if err != nil {
		printf(stderr, "gdrift: %v\n", err)

		return exitError
	}

	if err := start(ctx, dir); err != nil {
		if errors.Is(err, context.Canceled) {
			return exitInterrupted
		}

		printf(stderr, "gdrift: %v\n", err)

		return exitError
	}

	return exitOK
}

// resolveDir returns the absolute, symlink-resolved directory for arg, or the
// current directory when arg is empty.
func resolveDir(arg string) (string, error) {
	if arg == "" {
		arg = "."
	}

	abs, err := filepath.Abs(arg)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", arg, err)
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve directory: %w", err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat directory: %w", err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", resolved)
	}

	return resolved, nil
}

// printf writes to a console stream; a failed write has nowhere to be
// reported, so the error is dropped.
func printf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// writeLine is printf for a single value followed by a newline.
func writeLine(w io.Writer, s string) {
	_, _ = fmt.Fprintln(w, s)
}
