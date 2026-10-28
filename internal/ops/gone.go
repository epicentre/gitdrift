package ops

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/repo"
)

// LsRemoteTimeout bounds the remote check made before a branch is deleted.
const LsRemoteTimeout = 20 * time.Second

// deleteTimeout bounds branch -D. The deletion runs detached from the
// caller's cancellation, so a quit never kills git while it holds a ref lock
// and leaves a .lock file behind.
const deleteTimeout = 30 * time.Second

// Skip reasons for a branch deletion whose preconditions do not hold.
const (
	skipBranchMissing    = "branch no longer exists"
	skipBranchChanged    = "branch changed since review"
	skipBranchCheckedOut = "branch is checked out"
	skipUpstreamBack     = "upstream exists again"
	skipRemoteHasBranch  = "branch still exists on remote"
	skipNotVerified      = "could not verify remote: "
	skipNoRemoteUpstream = "branch has no remote-tracking upstream"
)

const (
	headsPrefix        = "refs/heads/"
	remotesPrefix      = "refs/remotes/"
	lsRemoteNoMatch    = 2
	worktreeBranchLine = "branch " + headsPrefix
)

// DeleteRequest describes a gone branch as the user reviewed it.
type DeleteRequest struct {
	Loc repo.Location
	// Branch is the short branch name and SHA its tip at review time.
	Branch, SHA string
	// Remote and RemoteRef name the branch on the remote, for example
	// "origin" and "refs/heads/feature/x".
	Remote, RemoteRef string
}

// DeleteOutcome is the result of DeleteGone.
type DeleteOutcome struct {
	Branch, SHA string
	Deleted     bool
	// Skipped is the reason nothing was deleted; empty when git ran.
	Skipped string
	// Err is the error that stopped the deletion.
	Err error
}

// DeleteGone deletes a local branch whose remote branch is gone, after
// verifying that the branch is unchanged, not checked out, has no tracking
// ref again and is really absent on the remote. The tip and checked-out
// checks run again after the network call, so a commit or checkout made
// meanwhile is never deleted and the restore SHA stays exact. Once branch -D
// has started, cancelling ctx no longer stops it.
func DeleteGone(ctx context.Context, g *git.Runner, req DeleteRequest) DeleteOutcome {
	out := DeleteOutcome{Branch: req.Branch, SHA: req.SHA}

	if skip, err := checkLocal(ctx, g, req); skip != "" || err != nil {
		out.Skipped, out.Err = skip, err

		return out
	}

	if skip, err := checkRemote(ctx, g, req); skip != "" || err != nil {
		out.Skipped, out.Err = skip, err

		return out
	}

	// The network call took time: verify again right before deleting.
	if skip, err := checkTipAndWorktrees(ctx, g, req); skip != "" || err != nil {
		out.Skipped, out.Err = skip, err

		return out
	}

	// Never start a write after the caller has quit.
	if err := ctx.Err(); err != nil {
		out.Err = err

		return out
	}

	deleteCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deleteTimeout)
	defer cancel()

	if _, err := g.Run(deleteCtx, req.Loc.Path, "branch", "-D", "--end-of-options", req.Branch); err != nil {
		out.Err = err

		return out
	}

	out.Deleted = true

	return out
}

// checkLocal runs the local checks in order: existence, tip, checkout and
// the tracking ref.
func checkLocal(ctx context.Context, g *git.Runner, req DeleteRequest) (string, error) {
	if skip, err := checkTipAndWorktrees(ctx, g, req); skip != "" || err != nil {
		return skip, err
	}

	upstream, err := g.Run(ctx, req.Loc.Path, "for-each-ref", "--format=%(upstream)", headsPrefix+req.Branch)
	if err != nil {
		return "", fmt.Errorf("ops: read upstream of %s: %w", req.Branch, err)
	}

	ref := strings.TrimSpace(string(upstream))
	if !strings.HasPrefix(ref, remotesPrefix) {
		return skipNoRemoteUpstream, nil
	}

	if _, err := g.Run(ctx, req.Loc.Path, "rev-parse", "--verify", "--quiet", ref); err == nil {
		return skipUpstreamBack, nil
	} else if git.ExitCode(err) != 1 {
		return "", fmt.Errorf("ops: check upstream of %s: %w", req.Branch, err)
	}

	return "", nil
}

// checkTipAndWorktrees verifies that the branch exists at the reviewed tip
// and is not checked out in any worktree.
func checkTipAndWorktrees(ctx context.Context, g *git.Runner, req DeleteRequest) (string, error) {
	tip, err := g.Run(ctx, req.Loc.Path, "rev-parse", "--verify", "--quiet", headsPrefix+req.Branch)
	if err != nil {
		if git.ExitCode(err) == 1 {
			return skipBranchMissing, nil
		}

		return "", fmt.Errorf("ops: resolve %s: %w", req.Branch, err)
	}

	if strings.TrimSpace(string(tip)) != req.SHA {
		return skipBranchChanged, nil
	}

	wt, err := g.Run(ctx, req.Loc.Path, "worktree", "list", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("ops: list worktrees: %w", err)
	}

	for line := range strings.SplitSeq(string(wt), "\n") {
		if line == worktreeBranchLine+req.Branch {
			return skipBranchCheckedOut, nil
		}
	}

	return "", nil
}

// checkRemote asks the remote whether the branch is really gone.
func checkRemote(ctx context.Context, g *git.Runner, req DeleteRequest) (string, error) {
	if req.Remote == "" || req.RemoteRef == "" {
		return skipNotVerified + "branch has no remote or remote ref", nil
	}

	lsCtx, cancel := context.WithTimeout(ctx, LsRemoteTimeout)
	defer cancel()

	_, err := g.Run(lsCtx, req.Loc.Path, "ls-remote", "--exit-code", "--end-of-options", req.Remote, req.RemoteRef)

	// A cancelled caller is not a remote failure.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}

	switch {
	case err == nil:
		return skipRemoteHasBranch, nil
	case git.ExitCode(err) == lsRemoteNoMatch:
		return "", nil
	default:
		return skipNotVerified + git.Message(err), nil
	}
}

// RestoreCommand returns a shell command that recreates a deleted branch at
// its old tip. The command is exact and can be shown as it is: a name with a
// character that a terminal hides or acts on (a control or format character,
// a line or paragraph separator, invalid UTF-8) is written as
// "$(printf '<octal>')", every byte of it as an octal escape. Any other name
// is single-quoted unless it only holds characters that are safe in a POSIX
// shell.
func RestoreCommand(o DeleteOutcome) string {
	name := shellQuote(o.Branch)
	if hasHiddenRune(o.Branch) {
		name = printfWord(o.Branch)
	}

	return "git branch " + name + " " + o.SHA
}

// hasHiddenRune reports whether s holds a rune that a terminal does not show
// as itself.
func hasHiddenRune(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}

	return strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)
	})
}

// printfWord writes s as a POSIX printf command substitution that prints
// exactly its bytes.
func printfWord(s string) string {
	var b strings.Builder

	b.WriteString(`"$(printf '`)

	for i := range len(s) {
		fmt.Fprintf(&b, `\%03o`, s[i])
	}

	b.WriteString(`')"`)

	return b.String()
}

func shellQuote(s string) string {
	safe := s != ""

	for _, r := range s {
		if !isShellSafe(r) {
			safe = false

			break
		}
	}

	if safe {
		return s
	}

	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isShellSafe(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	default:
		return strings.ContainsRune("._/@+-", r)
	}
}
