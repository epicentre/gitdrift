// Package git runs git processes for gitdrift. Every git invocation in the
// project goes through Runner, which refuses any argv that Classify does not
// recognise as a read command or as one of the few exact write forms.
package git

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Kind classifies a git argv.
type Kind int

// Kinds returned by Classify.
const (
	// KindUnknown is an argv that is not allowed.
	KindUnknown Kind = iota
	// KindRead is an argv that only reads repository state.
	KindRead
	// KindWrite is one of the exact allowlisted writing argv forms.
	KindWrite
)

// ErrNotAllowed is wrapped by every error for a refused argv.
var ErrNotAllowed = errors.New("git: command not allowed")

const (
	cmdFetch  = "fetch"
	cmdMerge  = "merge"
	cmdBranch = "branch"
	cmdConfig = "config"

	flagEndOfOptions = "--end-of-options"
	baseConfigKey    = "gitdrift.base"
)

// readSubcommands may take any flags (subject to the global deny list).
var readSubcommands = []string{
	"status", "rev-parse", "rev-list", "log", "show", "diff", "diff-tree",
	"for-each-ref", "merge-base", "cat-file", "ls-files", "version",
}

// lsRemoteFlags are the only flags ls-remote accepts; others can run programs.
var lsRemoteFlags = []string{
	"--exit-code", flagEndOfOptions, "-q", "--quiet", "--heads", "--branches", "-b",
	"--tags", "-t", "--refs", "--symref",
}

// symbolicRefFlags are the read-only flags of symbolic-ref.
var symbolicRefFlags = []string{"-q", "--quiet", "--short", "--recurse", "--no-recurse"}

// configReadFlags selects a read form of git config.
var configReadFlags = []string{"--get", "--get-all", "--get-regexp", "--list", "-l"}

// configOptionFlags may accompany a config read form.
var configOptionFlags = []string{
	"--show-scope", "--local", "--global", "--null", "-z", "--includes",
}

// Classify reports whether args (without the leading "git") is a read, one of
// the allowlisted writes, or refused. A refused argv yields KindUnknown and an
// error wrapping ErrNotAllowed that names the subcommand.
func Classify(args []string) (Kind, error) {
	if len(args) == 0 {
		return KindUnknown, fmt.Errorf("%w: empty argument list", ErrNotAllowed)
	}

	sub := args[0]

	if slices.ContainsFunc(args, isDeniedArg) {
		return KindUnknown, refused(sub)
	}

	if isWriteForm(args) {
		return KindWrite, nil
	}

	if isReadForm(args) {
		return KindRead, nil
	}

	return KindUnknown, refused(sub)
}

// isDeniedArg reports arguments refused in any argv: force-style flags, and
// flags that write files or run external programs from read commands.
func isDeniedArg(a string) bool {
	switch a {
	case "--force", "--hard", "--autostash", "--output", "--ext-diff":
		return true
	default:
		return strings.HasPrefix(a, "--force=") || strings.HasPrefix(a, "--output=")
	}
}

func refused(sub string) error {
	return fmt.Errorf("%w: %s", ErrNotAllowed, sub)
}

func isReadForm(args []string) bool {
	sub, rest := args[0], args[1:]

	if slices.Contains(readSubcommands, sub) {
		return true
	}

	switch sub {
	case "ls-remote":
		return isLsRemoteRead(rest)
	case "symbolic-ref":
		return isSymbolicRefRead(rest)
	case "hash-object":
		return !slices.ContainsFunc(rest, hasWriteFlag)
	case "remote":
		return len(rest) == 0
	case "worktree", "stash":
		return len(rest) > 0 && rest[0] == "list" && allFlags(rest[1:])
	case cmdConfig:
		return isConfigRead(rest)
	default:
		return false
	}
}

// hasWriteFlag reports whether a hash-object argument is -w, alone or inside
// a short-flag cluster.
func hasWriteFlag(a string) bool {
	return strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "w")
}

func isLsRemoteRead(rest []string) bool {
	for _, a := range rest {
		if a == flagEndOfOptions {
			return true // everything after is positional
		}

		if !strings.HasPrefix(a, "-") {
			continue
		}

		if !slices.Contains(lsRemoteFlags, a) && !strings.HasPrefix(a, "--sort=") {
			return false
		}
	}

	return true
}

func isSymbolicRefRead(rest []string) bool {
	positional := 0

	for _, a := range rest {
		switch {
		case slices.Contains(symbolicRefFlags, a):
		case strings.HasPrefix(a, "-"):
			return false
		default:
			positional++
		}
	}

	return positional == 1
}

func allFlags(args []string) bool {
	return !slices.ContainsFunc(args, func(a string) bool { return !strings.HasPrefix(a, "-") })
}

func isConfigRead(rest []string) bool {
	hasRead := false

	for _, a := range rest {
		switch {
		case slices.Contains(configReadFlags, a):
			hasRead = true
		case slices.Contains(configOptionFlags, a):
		case strings.HasPrefix(a, "-"):
			return false
		}
	}

	return hasRead
}

func isWriteForm(args []string) bool {
	switch args[0] {
	case cmdFetch:
		return len(args) == 6 && slices.Equal(args[1:5], []string{"--prune", "--no-prune-tags", "--quiet", flagEndOfOptions}) &&
			isOperand(args[5])
	case cmdMerge:
		return len(args) == 6 &&
			slices.Equal(args[1:5], []string{"--ff-only", "--no-autostash", "--no-overwrite-ignore", flagEndOfOptions}) &&
			isSHA(args[5])
	case cmdBranch:
		return len(args) == 4 && args[1] == "-D" && args[2] == flagEndOfOptions && isOperand(args[3])
	case cmdConfig:
		return isConfigWrite(args)
	default:
		return false
	}
}

func isConfigWrite(args []string) bool {
	if len(args) != 4 || args[1] != "--local" {
		return false
	}

	return (args[2] == baseConfigKey && isOperand(args[3])) ||
		(args[2] == "--unset" && args[3] == baseConfigKey)
}

func isOperand(s string) bool {
	return s != "" && !strings.HasPrefix(s, "-")
}

func isSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}

	for _, c := range []byte(s) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}

	return true
}
