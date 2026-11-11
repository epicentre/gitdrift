package ops

import (
	"context"
	"time"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/repo"
)

// Skip reasons for a pull whose preconditions no longer hold.
const (
	skipHeadChanged     = "repository changed since review, press r"
	skipUpstreamChanged = "upstream changed since review, press r"
)

// mergeTimeout bounds the fast-forward. The merge runs detached from the
// caller's cancellation and the Runner's default timeout, so a quit or a slow
// checkout never kills git halfway through updating the worktree.
const mergeTimeout = 5 * time.Minute

// PullRequest describes the state the user reviewed before asking for a pull.
type PullRequest struct {
	Loc repo.Location
	// Branch is the branch checked out at review time.
	Branch string
	// HeadSHA and UpstreamSHA are the commits seen at review time.
	HeadSHA, UpstreamSHA string
	// Fetch is the fetch outcome of the review.
	Fetch drift.FetchState
}

// PullOutcome is the result of Pull.
type PullOutcome struct {
	// Pulled is the number of commits fast-forwarded.
	Pulled int
	// Skipped is the reason nothing was attempted; empty when git ran.
	Skipped string
	// Err is the error that stopped the pull: a failed status read, or git's
	// own error when the merge was attempted and failed.
	Err error
}

// Pull fast-forwards the current branch to the reviewed upstream commit. It
// re-reads the status, and with local changes the files the pull would
// change, and skips when the repository is no longer safe or no longer
// matches the review (another branch, HEAD or upstream commit). Local
// changes to other files stay as they are. It never fetches, and git itself
// refuses to overwrite local files, ignored ones included.
func Pull(ctx context.Context, g *git.Runner, req PullRequest) PullOutcome {
	st, err := repo.ReadStatus(ctx, g, req.Loc)
	if err != nil {
		return PullOutcome{Err: err}
	}

	snap := drift.Snapshot{Loc: req.Loc, Status: st, Fetch: req.Fetch}

	if st.Tracked > 0 {
		if snap.Incoming, err = repo.IncomingFiles(ctx, g, req.Loc, st); err != nil {
			return PullOutcome{Err: err}
		}

		snap.IncomingRead = true
	}

	if blockers := drift.PullBlockers(snap); len(blockers) > 0 {
		return PullOutcome{Skipped: blockers[0]}
	}

	// A switch to another branch at the same commit, or to a detached HEAD
	// (whose Branch is empty), changes what the merge would move.
	if st.HeadSHA != req.HeadSHA || st.Branch != req.Branch {
		return PullOutcome{Skipped: skipHeadChanged}
	}

	if st.UpstreamSHA != req.UpstreamSHA {
		return PullOutcome{Skipped: skipUpstreamChanged}
	}

	// Never start a write after the caller has quit.
	if err := ctx.Err(); err != nil {
		return PullOutcome{Err: err}
	}

	mergeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mergeTimeout)
	defer cancel()

	args := []string{"merge", "--ff-only", "--no-autostash", "--no-overwrite-ignore", "--end-of-options", req.UpstreamSHA}
	if _, err := g.Run(mergeCtx, req.Loc.Path, args...); err != nil {
		return PullOutcome{Err: err}
	}

	return PullOutcome{Pulled: st.Behind}
}
