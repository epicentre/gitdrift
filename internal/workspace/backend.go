package workspace

import (
	"context"
	"errors"
	"fmt"

	"github.com/epicentre/gitdrift/internal/ops"
	"github.com/epicentre/gitdrift/internal/repo"
)

// ErrUnknownGroup means a write named a repository outside the workspace.
var ErrUnknownGroup = errors.New("workspace: repository is not part of the workspace")

// RangeCommits lists the commits in exclude..include and their total count.
func (w *Workspace) RangeCommits(
	ctx context.Context, loc repo.Location, exclude, include string, limit int,
) ([]repo.Commit, int, error) {
	if !w.enter() {
		return nil, 0, ErrClosed
	}
	defer w.leave()

	return repo.RangeCommits(ctx, w.g, loc, exclude, include, limit)
}

// OnlyHereCommits lists the commits of ref that no remote has.
func (w *Workspace) OnlyHereCommits(
	ctx context.Context, loc repo.Location, ref string, limit int,
) ([]repo.Commit, error) {
	if !w.enter() {
		return nil, ErrClosed
	}
	defer w.leave()

	return repo.OnlyHereCommits(ctx, w.g, loc, ref, limit)
}

// CommitSpec returns the diff specification of one commit.
func (w *Workspace) CommitSpec(ctx context.Context, loc repo.Location, sha string) (repo.DiffSpec, error) {
	if !w.enter() {
		return repo.DiffSpec{}, ErrClosed
	}
	defer w.leave()

	return repo.CommitSpec(ctx, w.g, loc, sha)
}

// Files lists the files changed by spec.
func (w *Workspace) Files(ctx context.Context, loc repo.Location, spec repo.DiffSpec) ([]repo.FileChange, error) {
	if !w.enter() {
		return nil, ErrClosed
	}
	defer w.leave()

	return repo.Files(ctx, w.g, loc, spec)
}

// FileDiff returns the diff of one file, cut at maxBytes; oldPath is the path
// of a renamed file before the rename.
func (w *Workspace) FileDiff(
	ctx context.Context, loc repo.Location, spec repo.DiffSpec, path, oldPath string, maxBytes int,
) (string, bool, error) {
	if !w.enter() {
		return "", false, ErrClosed
	}
	defer w.leave()

	return repo.FileDiff(ctx, w.g, loc, spec, path, oldPath, maxBytes)
}

// RemoteBranches lists the remote-tracking branches of the repository.
func (w *Workspace) RemoteBranches(ctx context.Context, loc repo.Location) ([]string, error) {
	if !w.enter() {
		return nil, ErrClosed
	}
	defer w.leave()

	return repo.RemoteBranches(ctx, w.g, loc)
}

// lockGroup takes the lock of the group that owns commonDir. Writes and
// refreshes of one group are serialized because its worktrees share refs.
func (w *Workspace) lockGroup(ctx context.Context, commonDir string) (func(), error) {
	st, ok := w.state[commonDir]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownGroup, commonDir)
	}

	if err := st.acquire(ctx); err != nil {
		return nil, err
	}

	return st.release, nil
}

// Pull fast-forwards the repository after waiting for the group's refresh.
func (w *Workspace) Pull(ctx context.Context, req ops.PullRequest) ops.PullOutcome {
	if !w.enter() {
		return ops.PullOutcome{Err: ErrClosed}
	}
	defer w.leave()

	unlock, err := w.lockGroup(ctx, req.Loc.CommonDir)
	if err != nil {
		return ops.PullOutcome{Err: err}
	}
	defer unlock()

	return ops.Pull(ctx, w.g, req)
}

// DeleteGone deletes a gone branch after waiting for the group's refresh.
func (w *Workspace) DeleteGone(ctx context.Context, req ops.DeleteRequest) ops.DeleteOutcome {
	if !w.enter() {
		return ops.DeleteOutcome{Branch: req.Branch, SHA: req.SHA, Err: ErrClosed}
	}
	defer w.leave()

	unlock, err := w.lockGroup(ctx, req.Loc.CommonDir)
	if err != nil {
		return ops.DeleteOutcome{Branch: req.Branch, SHA: req.SHA, Err: err}
	}
	defer unlock()

	return ops.DeleteGone(ctx, w.g, req)
}

// SetBase stores the base branch after waiting for the group's refresh.
func (w *Workspace) SetBase(ctx context.Context, loc repo.Location, value string) error {
	if !w.enter() {
		return ErrClosed
	}
	defer w.leave()

	unlock, err := w.lockGroup(ctx, loc.CommonDir)
	if err != nil {
		return err
	}
	defer unlock()

	return ops.SetBase(ctx, w.g, loc, value)
}

// ResetBase removes the stored base branch after waiting for the group's
// refresh.
func (w *Workspace) ResetBase(ctx context.Context, loc repo.Location) error {
	if !w.enter() {
		return ErrClosed
	}
	defer w.leave()

	unlock, err := w.lockGroup(ctx, loc.CommonDir)
	if err != nil {
		return err
	}
	defer unlock()

	return ops.ResetBase(ctx, w.g, loc)
}
