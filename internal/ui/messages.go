package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/ops"
	"github.com/epicentre/gitdrift/internal/repo"
)

// groupRefreshedMsg carries the snapshots of one refreshed group.
type groupRefreshedMsg struct {
	commonDir string
	snaps     []drift.Snapshot
}

// refreshCmd re-reads one group in the background, fetching first when fetch
// is set.
func refreshCmd(ctx context.Context, b Backend, commonDir string, fetch bool) tea.Cmd {
	return func() tea.Msg {
		return groupRefreshedMsg{commonDir: commonDir, snaps: b.Refresh(ctx, commonDir, fetch)}
	}
}

// detailTickMsg ends the debounce delay of the detail load generation
// token.
type detailTickMsg struct{ token int }

// commitsLoadedMsg carries the commits of the Incoming or Base tab: the
// newest ones and how many there are in all.
type commitsLoadedMsg struct {
	token   int
	commits []repo.Commit
	total   int
	err     error
}

// filesLoadedMsg carries the files of the entry under the Commits cursor and
// the comparison they were listed for, which their diffs use.
type filesLoadedMsg struct {
	token int
	files []repo.FileChange
	spec  repo.DiffSpec
	err   error
}

// diffLoadedMsg carries the diff text of the selected file or commit.
type diffLoadedMsg struct {
	token     int
	text      string
	truncated bool
	err       error
}

// onlyHereLoadedMsg carries the commits of the selected branch that no remote
// has.
type onlyHereLoadedMsg struct {
	token   int
	commits []repo.Commit
	err     error
}

// detailLoader starts the backend queries of one detail load generation for
// one repository. Every command it returns answers with the generation's
// token, so that Update can drop answers that arrive too late.
type detailLoader struct {
	backend Backend
	loc     repo.Location
	token   int
}

// commits lists the commits in exclude..include.
func (l detailLoader) commits(ctx context.Context, exclude, include string) tea.Cmd {
	return func() tea.Msg {
		commits, total, err := l.backend.RangeCommits(ctx, l.loc, exclude, include, repo.MaxCommits)

		return commitsLoadedMsg{token: l.token, commits: commits, total: total, err: err}
	}
}

// files lists the files that spec changes.
func (l detailLoader) files(ctx context.Context, spec repo.DiffSpec) tea.Cmd {
	return func() tea.Msg {
		files, err := l.backend.Files(ctx, l.loc, spec)

		return filesLoadedMsg{token: l.token, files: files, spec: spec, err: err}
	}
}

// commitFiles lists the files that one commit changes.
func (l detailLoader) commitFiles(ctx context.Context, sha string) tea.Cmd {
	return func() tea.Msg {
		spec, err := l.backend.CommitSpec(ctx, l.loc, sha)
		if err != nil {
			return filesLoadedMsg{token: l.token, err: err}
		}

		files, err := l.backend.Files(ctx, l.loc, spec)

		return filesLoadedMsg{token: l.token, files: files, spec: spec, err: err}
	}
}

// diff loads the diff of one file of spec, or of all of spec when path is
// empty; oldPath is the path of a renamed file before the rename.
func (l detailLoader) diff(ctx context.Context, spec repo.DiffSpec, path, oldPath string) tea.Cmd {
	return func() tea.Msg {
		text, truncated, err := l.backend.FileDiff(ctx, l.loc, spec, path, oldPath, repo.MaxDiffBytes)

		return diffLoadedMsg{token: l.token, text: text, truncated: truncated, err: err}
	}
}

// commitDiff loads the whole diff of one commit.
func (l detailLoader) commitDiff(ctx context.Context, sha string) tea.Cmd {
	return func() tea.Msg {
		spec, err := l.backend.CommitSpec(ctx, l.loc, sha)
		if err != nil {
			return diffLoadedMsg{token: l.token, err: err}
		}

		text, truncated, err := l.backend.FileDiff(ctx, l.loc, spec, "", "", repo.MaxDiffBytes)

		return diffLoadedMsg{token: l.token, text: text, truncated: truncated, err: err}
	}
}

// onlyHere lists the commits of branch ref that no remote has.
func (l detailLoader) onlyHere(ctx context.Context, ref string) tea.Cmd {
	return func() tea.Msg {
		commits, err := l.backend.OnlyHereCommits(ctx, l.loc, ref, repo.MaxCommits)

		return onlyHereLoadedMsg{token: l.token, commits: commits, err: err}
	}
}

// pullDoneMsg carries the outcome of one pull and the badges of the snapshot
// the user confirmed it from.
type pullDoneMsg struct {
	id      repo.ID
	outcome ops.PullOutcome
	badges  []string
}

// pullCmd fast-forwards the repository of s to the upstream commit the user
// reviewed, if it still matches the snapshot.
func pullCmd(ctx context.Context, b Backend, s drift.Snapshot) tea.Cmd {
	req := ops.PullRequest{
		Loc: s.Loc, Branch: s.Status.Branch, HeadSHA: s.Status.HeadSHA, UpstreamSHA: s.Status.UpstreamSHA, Fetch: s.Fetch,
	}

	return func() tea.Msg {
		return pullDoneMsg{id: s.Loc.ID, outcome: b.Pull(ctx, req), badges: s.Badges}
	}
}

// deleteLoadedMsg carries the gone branches to offer for deletion, each with
// the commits that only this machine has.
type deleteLoadedMsg struct {
	loc   repo.Location
	items []deleteItem
	err   error
}

// loadDeleteCmd lists, for each gone branch, the commits a deletion would
// leave only in the reflog.
func loadDeleteCmd(ctx context.Context, b Backend, loc repo.Location, branches []repo.GoneBranch) tea.Cmd {
	return func() tea.Msg {
		items := make([]deleteItem, 0, len(branches))

		for _, g := range branches {
			commits, err := b.OnlyHereCommits(ctx, loc, g.Name, repo.MaxCommits)
			if err != nil {
				return deleteLoadedMsg{loc: loc, err: err}
			}

			items = append(items, deleteItem{gone: g, commits: commits})
		}

		return deleteLoadedMsg{loc: loc, items: items}
	}
}

// deleteDoneMsg carries the outcome of each confirmed deletion.
type deleteDoneMsg struct {
	loc      repo.Location
	outcomes []ops.DeleteOutcome
}

// deleteCmd deletes the gone branches one after the other, each with the tip
// the user reviewed.
func deleteCmd(ctx context.Context, b Backend, loc repo.Location, branches []repo.GoneBranch) tea.Cmd {
	return func() tea.Msg {
		outcomes := make([]ops.DeleteOutcome, 0, len(branches))

		for _, g := range branches {
			outcomes = append(outcomes, b.DeleteGone(ctx, ops.DeleteRequest{
				Loc: loc, Branch: g.Name, SHA: g.SHA, Remote: g.Remote, RemoteRef: g.RemoteRef,
			}))
		}

		return deleteDoneMsg{loc: loc, outcomes: outcomes}
	}
}

// remoteBranchesMsg carries the remote-tracking branches the base picker
// offers.
type remoteBranchesMsg struct {
	id   repo.ID
	refs []string
	err  error
}

func remoteBranchesCmd(ctx context.Context, b Backend, loc repo.Location) tea.Cmd {
	return func() tea.Msg {
		refs, err := b.RemoteBranches(ctx, loc)

		return remoteBranchesMsg{id: loc.ID, refs: refs, err: err}
	}
}

// baseChangedMsg reports a stored or removed base of the group commonDir.
type baseChangedMsg struct {
	commonDir string
	err       error
}

// setBaseCmd stores ref as the base of the repository.
func setBaseCmd(ctx context.Context, b Backend, loc repo.Location, ref string) tea.Cmd {
	return func() tea.Msg {
		return baseChangedMsg{commonDir: loc.CommonDir, err: b.SetBase(ctx, loc, ref)}
	}
}

// resetBaseCmd removes the stored base of the repository.
func resetBaseCmd(ctx context.Context, b Backend, loc repo.Location) tea.Cmd {
	return func() tea.Msg {
		return baseChangedMsg{commonDir: loc.CommonDir, err: b.ResetBase(ctx, loc)}
	}
}
