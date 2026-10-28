package ui

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/ops"
	"github.com/epicentre/gitdrift/internal/repo"
)

// Status line and row copy of the actions.
const (
	nothingSafeToPull = "nothing safe to pull"
	cannotSelect      = "cannot select: "
	pullInProgress    = "pull in progress"
	stillFetching     = "still fetching"
	pulling           = "pulling…"
	baseNotChanged    = "base not changed: "
	openBranchesTab   = "open the Branches tab to delete gone branches"

	refusedCurrent    = "current branch cannot be deleted"
	refusedCheckedOut = "branch is checked out in "
	refusedFetch      = fetchFailed
	refusedNotFetched = "remote not fetched yet: "

	unselected = " unselected: "
)

// result is the outcome of the last pull of a repository as its row and the
// status line show it, until r or R refreshes the repository.
type result struct {
	text string
	// err reports that nothing was pulled: the pull was skipped or failed.
	err bool
}

// pullResult describes a pull outcome. A successful pull lists the badges of
// the snapshot it was confirmed from: after the pull nothing is incoming any
// more, so the badges would vanish right when they matter.
func pullResult(o ops.PullOutcome, badges []string) result {
	switch {
	case o.Err != nil:
		return result{text: "pull failed: " + gitMessage(o.Err), err: true}
	case o.Skipped != "":
		return result{text: "skipped: " + sanitize(o.Skipped), err: true}
	}

	text := "pulled " + strconv.Itoa(o.Pulled)

	if len(badges) > 0 {
		text += separator + strings.Join(sanitizeAll(badges), " ")
	}

	return result{text: text}
}

// firstLine is the sanitized first line of s.
func firstLine(s string) string {
	first, _, _ := strings.Cut(s, "\n")

	return sanitize(first)
}

// gitMessage is git's own message for a failed git command, sanitized (see
// git.Message). A message that sanitizing empties falls back to the first
// line of the error.
func gitMessage(err error) string {
	if msg := strings.TrimSpace(sanitize(git.Message(err))); msg != "" {
		return msg
	}

	return firstLine(err.Error())
}

// goneKey names a gone branch of the repository whose common directory is
// commonDir.
type goneKey struct {
	commonDir, branch string
}

// writable returns a copy of mp to change: the previous model keeps mp.
func writable[K comparable, V any](mp map[K]V) map[K]V {
	out := make(map[K]V, len(mp)+1)
	maps.Copy(out, mp)

	return out
}

// handleActionKey runs the actions that work from any pane. It reports
// whether the key was one of them. On a screen too small for the dashboard
// they do nothing: what they open or change could not be seen.
func (m *Model) handleActionKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.layout().TooSmall {
		return nil, key.Matches(msg, m.keys.actions()...)
	}

	switch {
	case key.Matches(msg, m.keys.Help):
		m.openDialog(helpOverlay{})
	case key.Matches(msg, m.keys.Select):
		m.toggleSelection()
	case key.Matches(msg, m.keys.Pull):
		m.askPull(false)
	case key.Matches(msg, m.keys.PullAll):
		m.askPull(true)
	case key.Matches(msg, m.keys.Delete):
		return m.askDelete(), true
	case key.Matches(msg, m.keys.Base):
		return m.openPicker(), true
	case key.Matches(msg, m.keys.Refresh):
		return m.refreshCursor(), true
	case key.Matches(msg, m.keys.RefreshAll):
		return m.refreshAll(), true
	default:
		return nil, false
	}

	return nil, true
}

// applyAction handles the answers of the action commands. It reports
// whether msg was one of them.
func (m *Model) applyAction(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case pullDoneMsg:
		return m.pullDone(msg), true
	case deleteLoadedMsg:
		m.deleteLoaded(msg)
	case deleteDoneMsg:
		m.deleteDone(msg)
	case remoteBranchesMsg:
		if p, ok := m.dialog.(basePicker); ok && p.loc.ID == msg.id {
			m.dialog = p.loaded(msg.refs, msg.err)
		}
	case baseChangedMsg:
		if msg.err != nil {
			m.notice = baseNotChanged + firstLine(msg.err.Error())
		}

		return m.requestRefresh(msg.commonDir, false), true
	default:
		return nil, false
	}

	return nil, true
}

// toggleSelection selects or unselects the row under the cursor: a
// repository for pull in the Repos pane, a gone branch in the Branches tab.
func (m *Model) toggleSelection() {
	if m.focus == PaneRepos {
		m.togglePull()

		return
	}

	m.toggleGone()
}

// togglePull selects the repository under the cursor for pull when it is
// safe to pull, or says why not; a selected repository is unselected.
func (m *Model) togglePull() {
	r, ok := m.cursorRow()
	if !ok {
		return
	}

	id := r.snap.Loc.ID
	m.selected = writable(m.selected)

	switch reason := m.pullRefusal(r); {
	case m.selected[id]:
		delete(m.selected, id)
	case reason != "":
		m.notice = cannotSelect + reason
	default:
		m.selected[id] = true
	}
}

// pullRefusal is why row r cannot be pulled now, sanitized; empty when it
// can.
func (m Model) pullRefusal(r row) string {
	switch {
	case m.pulling[r.snap.Loc.ID]:
		return pullInProgress
	case !r.loaded:
		return stillFetching
	}

	if blockers := drift.PullBlockers(r.snap); len(blockers) > 0 {
		return sanitize(blockers[0])
	}

	return ""
}

// askPull opens the pull dialog for every repository that is safe to pull
// when all is set, else for the selection, else for the cursor's
// repository.
func (m *Model) askPull(all bool) {
	cursor, _ := m.cursorID()

	var snaps []drift.Snapshot

	for _, r := range m.rows {
		id := r.snap.Loc.ID

		wanted := all || m.selected[id] || (len(m.selected) == 0 && id == cursor)
		if wanted && m.pullRefusal(r) == "" {
			snaps = append(snaps, r.snap)
		}
	}

	if len(snaps) == 0 {
		m.notice = nothingSafeToPull

		return
	}

	m.openDialog(pullConfirm{snaps: snaps})
}

// confirmPull starts one pull per repository of the dialog, each with the
// snapshot the dialog showed. Their rows show "pulling…" until they answer.
func (m *Model) confirmPull(d pullConfirm) tea.Cmd {
	m.closeDialog()
	m.pulling, m.selected = writable(m.pulling), writable(m.selected)

	cmds := make([]tea.Cmd, 0, len(d.snaps))

	for _, s := range d.snaps {
		m.pulling[s.Loc.ID] = true
		delete(m.selected, s.Loc.ID)

		cmds = append(cmds, pullCmd(m.ctx, m.backend, s))
	}

	return tea.Batch(cmds...)
}

// pullDone keeps the result of a pull and re-reads its group.
func (m *Model) pullDone(msg pullDoneMsg) tea.Cmd {
	m.pulling, m.results = writable(m.pulling), writable(m.results)
	delete(m.pulling, msg.id)
	m.results[msg.id] = pullResult(msg.outcome, msg.badges)

	i := slices.IndexFunc(m.rows, func(r row) bool { return r.snap.Loc.ID == msg.id })
	if i < 0 {
		return nil
	}

	return m.requestRefresh(m.rows[i].snap.Loc.CommonDir, false)
}

// rowNote is what a read row shows in place of its status tokens: a pull or
// a fetch in progress, or the result of its last pull.
func (m Model) rowNote(r row) token {
	switch {
	case !r.loaded:
		return token{}
	case m.pulling[r.snap.Loc.ID]:
		return token{text: pulling, style: m.styles.Muted}
	case m.refreshing[r.snap.Loc.CommonDir]:
		return token{text: fetching, style: m.styles.Muted}
	}

	res, ok := m.results[r.snap.Loc.ID]
	if !ok {
		return token{}
	}

	if res.err {
		return token{text: res.text, style: m.styles.Attention}
	}

	return token{text: res.text, style: m.styles.UpToDate}
}

// requestRefresh re-reads the group of commonDir. While a refresh of the
// group is in flight it only remembers the request, which starts once that
// one answers; a remembered request fetches when any request for it did.
func (m *Model) requestRefresh(commonDir string, fetch bool) tea.Cmd {
	if _, busy := m.refreshing[commonDir]; busy {
		m.queued = writable(m.queued)
		m.queued[commonDir] = m.queued[commonDir] || fetch

		return nil
	}

	m.refreshing = writable(m.refreshing)
	m.refreshing[commonDir] = fetch

	return refreshCmd(m.ctx, m.backend, commonDir, fetch)
}

// refreshAnswered ends the refresh of commonDir in flight and starts the one
// requested meanwhile, if any.
func (m *Model) refreshAnswered(commonDir string) tea.Cmd {
	m.refreshing = writable(m.refreshing)
	delete(m.refreshing, commonDir)

	fetch, ok := m.queued[commonDir]
	if !ok {
		return nil
	}

	m.queued = writable(m.queued)
	delete(m.queued, commonDir)

	return m.requestRefresh(commonDir, fetch)
}

// refreshCursor fetches and re-reads the group of the cursor's repository
// and clears the pull results of its rows.
func (m *Model) refreshCursor() tea.Cmd {
	r, ok := m.cursorRow()
	if !ok {
		return nil
	}

	dir := r.snap.Loc.CommonDir
	m.results = writable(m.results)

	for _, rw := range m.rows {
		if rw.snap.Loc.CommonDir == dir {
			delete(m.results, rw.snap.Loc.ID)
		}
	}

	return m.requestRefresh(dir, true)
}

// refreshAll fetches and re-reads every group and clears every pull result.
func (m *Model) refreshAll() tea.Cmd {
	m.results = nil

	cmds := make([]tea.Cmd, 0, len(m.groups))
	for _, g := range m.groups {
		cmds = append(cmds, m.requestRefresh(g.CommonDir, true))
	}

	return tea.Batch(cmds...)
}

// pruneSelections drops the selected repositories that are no longer safe
// to pull, saying so in the status line, and the selected gone branches that
// are gone no more.
func (m *Model) pruneSelections(snaps []drift.Snapshot) {
	m.selected, m.goneSel = writable(m.selected), writable(m.goneSel)

	var dropped []string

	for _, s := range snaps {
		if blockers := drift.PullBlockers(s); len(blockers) > 0 && m.selected[s.Loc.ID] {
			delete(m.selected, s.Loc.ID)

			dropped = append(dropped, sanitize(s.Loc.Name)+unselected+sanitize(blockers[0]))
		}

		maps.DeleteFunc(m.goneSel, func(k goneKey, _ bool) bool {
			return k.commonDir == s.Loc.CommonDir &&
				!slices.ContainsFunc(s.Work.Gone, func(g repo.GoneBranch) bool { return g.Name == k.branch })
		})
	}

	switch len(dropped) {
	case 0:
	case 1:
		m.notice = dropped[0]
	default:
		m.notice = strconv.Itoa(len(dropped)) + " repositories unselected"
	}
}

// toggleGone selects the gone branch under the Commits cursor for deletion
// when it can be deleted, or says why not; a selected branch is unselected.
func (m *Model) toggleGone() {
	s, ok := m.detailSnapshot()
	g, okGone := m.goneUnderCursor()

	if !ok || !okGone {
		return
	}

	k := goneKey{commonDir: s.Loc.CommonDir, branch: g.Name}
	m.goneSel = writable(m.goneSel)

	switch reason := m.goneRefusal(s, g); {
	case m.goneSel[k]:
		delete(m.goneSel, k)
	case reason != "":
		m.notice = reason
	default:
		m.goneSel[k] = true
	}
}

// goneRefusal is why gone branch g of s is not offered for deletion,
// sanitized; empty when it is. A branch checked out in this or another
// worktree of the repository cannot be deleted, and a branch is only known
// to be gone after a successful fetch of its remote.
func (m Model) goneRefusal(s drift.Snapshot, g repo.GoneBranch) string {
	if !s.Status.Detached && g.Name == s.Status.Branch {
		return refusedCurrent
	}

	if wt, ok := m.checkedOutElsewhere(s, g.Name); ok {
		return refusedCheckedOut + sanitize(wt)
	}

	if s.Fetch.OK(g.Remote) {
		return ""
	}

	if _, failed := s.Fetch.Errors[g.Remote]; failed {
		return refusedFetch + sanitize(g.Remote)
	}

	return refusedNotFetched + sanitize(g.Remote)
}

// checkedOutElsewhere names a worktree of the repository of s, other than
// s, that has branch checked out.
func (m Model) checkedOutElsewhere(s drift.Snapshot, branch string) (string, bool) {
	for _, r := range m.rows {
		o := r.snap
		if r.loaded && o.Loc.CommonDir == s.Loc.CommonDir && o.Loc.ID != s.Loc.ID &&
			!o.Status.Detached && o.Status.Branch == branch {
			return o.Loc.Name, true
		}
	}

	return "", false
}

// askDelete loads what the selected gone branches of the cursor's
// repository, or else the gone branch under the Commits cursor, would lose;
// the dialog opens with that list. Branches that cannot be deleted are not
// offered. Outside the Branches tab it says where gone branches are.
func (m *Model) askDelete() tea.Cmd {
	s, ok := m.detailSnapshot()
	if !ok {
		return nil
	}

	if m.focus == PaneRepos || m.detail.tab != TabBranches {
		m.notice = openBranchesTab

		return nil
	}

	var picked []repo.GoneBranch

	for _, g := range s.Work.Gone {
		if m.goneSel[goneKey{commonDir: s.Loc.CommonDir, branch: g.Name}] {
			picked = append(picked, g)
		}
	}

	if len(picked) == 0 {
		g, ok := m.goneUnderCursor()
		if !ok {
			return nil
		}

		picked = []repo.GoneBranch{g}
	}

	var (
		offered []repo.GoneBranch
		refusal string
	)

	for _, g := range picked {
		if reason := m.goneRefusal(s, g); reason != "" {
			refusal = cmp.Or(refusal, reason)

			continue
		}

		offered = append(offered, g)
	}

	if len(offered) == 0 {
		m.notice = refusal

		return nil
	}

	return loadDeleteCmd(m.ctx, m.backend, s.Loc, offered)
}

// deleteLoaded opens the delete dialog once its commits are known, after
// the open window when there is one. Without the commits there is nothing to
// confirm: a failed listing only reports the error.
func (m *Model) deleteLoaded(msg deleteLoadedMsg) {
	if msg.err != nil {
		m.notice = errorPrefix + firstLine(msg.err.Error())

		return
	}

	m.show(deleteConfirm{loc: msg.loc, items: msg.items})
}

// confirmDelete deletes the branches of the dialog with the tips it showed.
func (m *Model) confirmDelete(d deleteConfirm) tea.Cmd {
	m.closeDialog()
	m.goneSel = writable(m.goneSel)

	branches := make([]repo.GoneBranch, len(d.items))
	for i, it := range d.items {
		branches[i] = it.gone
		delete(m.goneSel, goneKey{commonDir: d.loc.CommonDir, branch: it.gone.Name})
	}

	return deleteCmd(m.ctx, m.backend, d.loc, branches)
}

// deleteDone shows the results of a deletion, after the open window when
// there is one: they carry the restore commands and must not be lost.
func (m *Model) deleteDone(msg deleteDoneMsg) { m.show(deleteResult(msg)) }

// show opens d, or queues it behind the open window.
func (m *Model) show(d dialog) {
	if m.dialog == nil {
		m.openDialog(d)

		return
	}

	m.waiting = append(slices.Clip(m.waiting), d)
}

// openPicker opens the base picker of the cursor's repository and lists its
// remote-tracking branches.
func (m *Model) openPicker() tea.Cmd {
	r, ok := m.cursorRow()
	if !ok {
		return nil
	}

	m.openDialog(basePicker{loc: r.snap.Loc, current: r.snap.Base, loading: true})

	return remoteBranchesCmd(m.ctx, m.backend, r.snap.Loc)
}
