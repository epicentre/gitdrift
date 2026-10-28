// Package ui is the terminal dashboard: a Bubble Tea model with a Repos pane,
// commit, file and diff panes, and the dialogs that drive the few writing
// operations. Every string that comes from a repository is sanitized before
// it is rendered.
package ui

import (
	"context"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/ops"
	"github.com/epicentre/gitdrift/internal/repo"
	"github.com/epicentre/gitdrift/internal/workspace"
)

// Backend is everything the UI needs from the workspace. New calls Groups
// once; every other method may block on git and is called from a command,
// never from Update or View.
type Backend interface {
	// Groups returns the discovered repository groups.
	Groups() []workspace.Group
	// Refresh re-reads one group, fetching its remotes first when fetch is set.
	Refresh(ctx context.Context, commonDir string, fetch bool) []drift.Snapshot
	// RangeCommits lists the commits in exclude..include and their total count.
	RangeCommits(ctx context.Context, loc repo.Location, exclude, include string, limit int) ([]repo.Commit, int, error)
	// OnlyHereCommits lists the commits of ref that no remote has.
	OnlyHereCommits(ctx context.Context, loc repo.Location, ref string, limit int) ([]repo.Commit, error)
	// CommitSpec returns the diff specification of one commit.
	CommitSpec(ctx context.Context, loc repo.Location, sha string) (repo.DiffSpec, error)
	// Files lists the files changed by spec.
	Files(ctx context.Context, loc repo.Location, spec repo.DiffSpec) ([]repo.FileChange, error)
	// FileDiff returns the diff of one file, cut at maxBytes; oldPath is the
	// path of a renamed file before the rename.
	FileDiff(ctx context.Context, loc repo.Location, spec repo.DiffSpec, path, oldPath string, maxBytes int) (string, bool, error)
	// RemoteBranches lists the remote-tracking branches of the repository.
	RemoteBranches(ctx context.Context, loc repo.Location) ([]string, error)
	// Pull fast-forwards one repository.
	Pull(ctx context.Context, req ops.PullRequest) ops.PullOutcome
	// DeleteGone deletes one gone branch.
	DeleteGone(ctx context.Context, req ops.DeleteRequest) ops.DeleteOutcome
	// SetBase stores the base branch of the repository.
	SetBase(ctx context.Context, loc repo.Location, value string) error
	// ResetBase removes the stored base branch of the repository.
	ResetBase(ctx context.Context, loc repo.Location) error
}

// The workspace is the production Backend.
var _ Backend = (*workspace.Workspace)(nil)
