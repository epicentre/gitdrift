package ui

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/repo"
)

// Update applies one message to the model. Whenever the repository under the
// Repos cursor, its snapshot or the Commits tab changes, the detail panes
// start over.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.detailSubject()
	cmd, refreshed := m.apply(msg)

	if m.diffAutoZoomed() {
		m.zoom = false // no zoom state may hide behind the narrow Diff
	}

	if refreshed || m.detailSubject() != before {
		cmd = tea.Batch(cmd, m.resetDetail())
	}

	if spin := m.startSpinner(); spin != nil {
		cmd = tea.Batch(cmd, spin)
	}

	m.syncScroll()

	return m, cmd
}

// apply handles one message and returns the command it starts. refreshed
// reports a new snapshot of the repository under the Repos cursor.
func (m *Model) apply(msg tea.Msg) (cmd tea.Cmd, refreshed bool) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		m.styles = newStyles(msg.IsDark())
	case groupRefreshedMsg:
		cmd := m.refreshAnswered(msg.commonDir)
		allRead := m.readAnswered(msg.commonDir)

		// The rows hold still while other refreshes run, so that answers
		// arriving one by one do not reorder them; they sort once nothing
		// runs, and once every group was read before the fetches start.
		m.applySnapshots(msg.snaps, allRead || len(m.refreshing) == 0)

		if allRead {
			cmd = tea.Batch(cmd, m.fetchAll())
		}

		id, ok := m.cursorID()

		return cmd, ok && slices.ContainsFunc(msg.snaps, func(s drift.Snapshot) bool { return s.Loc.ID == id })
	case spinTickMsg:
		return m.turnSpinner(), false
	case tea.KeyPressMsg:
		return m.handleKey(msg), false
	case tea.PasteMsg:
		m.handlePaste(msg)
	default:
		if cmd, ok := m.applyAction(msg); ok {
			return cmd, false
		}

		return m.updateDetail(msg), false
	}

	return nil, false
}

// handleKey applies a key press and returns the command it starts, if any.
// ctrl+c always quits. Otherwise an open dialog takes every key, then a
// filter being typed does; the actions work from any pane, and navigation
// keys move the cursor of the focused pane, or scroll the Diff. Each key
// press clears the notice of the previous one.
func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	m.notice = ""

	if key.Matches(msg, m.keys.Interrupt) {
		return tea.Quit
	}

	if m.dialog != nil {
		return m.handleDialogKey(msg)
	}

	if m.filtering {
		m.handleFilterKey(msg)

		return nil
	}

	if key.Matches(msg, m.keys.Quit) {
		return tea.Quit
	}

	if cmd, ok := m.handleActionKey(msg); ok {
		return cmd
	}

	if m.handlePaneKey(msg) || m.handleTabKey(msg) {
		return nil
	}

	switch m.focus {
	case PaneRepos:
		m.handleCursorKey(msg)
	case PaneCommits:
		return m.moveCommitsCursor(msg)
	case PaneFiles:
		return m.moveFilesCursor(msg)
	case PaneDiff:
		m.scrollDiff(msg)
	}

	return nil
}

// handlePaneKey moves the focus, zooms, filters and starts typing a filter
// in the Repos pane. It reports whether the key was one of these.
func (m *Model) handlePaneKey(msg tea.KeyPressMsg) bool {
	switch {
	case key.Matches(msg, m.keys.NextPane):
		m.setFocus(m.focus.next())
	case key.Matches(msg, m.keys.PrevPane):
		m.setFocus(m.focus.prev())
	case key.Matches(msg, m.keys.FocusPane):
		m.setFocus(Pane(msg.String()[0] - '0'))
	case key.Matches(msg, m.keys.Enter):
		if m.focus != PaneDiff {
			m.setFocus(m.focus.next())
		}
	case key.Matches(msg, m.keys.Back):
		m.back()
	case key.Matches(msg, m.keys.Zoom):
		if !m.diffAutoZoomed() {
			m.zoom = !m.zoom
		}
	case key.Matches(msg, m.keys.OnlyHere):
		m.toggleOnlyHere()
	case key.Matches(msg, m.keys.Filter):
		// A filter is typed in the Repos pane, and only where it shows.
		if m.focus == PaneRepos && !m.layout().TooSmall {
			m.filtering = true
		}
	default:
		return false
	}

	return true
}

// handleTabKey switches the tabs of the focused pane with [ and ], wrapping
// around: the folder tabs when the Repos pane, compact or zoomed, has the
// focus in a workspace with folders, and otherwise the Commits tabs. It
// reports whether the key was one of these.
func (m *Model) handleTabKey(msg tea.KeyPressMsg) bool {
	next := key.Matches(msg, m.keys.NextTab)
	if !next && !key.Matches(msg, m.keys.PrevTab) {
		return false
	}

	if m.focus == PaneRepos {
		if tabs := folderTabs(m.rows); tabs != nil {
			m.switchFolderTab(tabs, !next)

			return true
		}
	}

	if next {
		m.detail.tab = m.detail.tab.next()
	} else {
		m.detail.tab = m.detail.tab.prev()
	}

	return true
}

// handleCursorKey moves the Repos cursor.
func (m *Model) handleCursorKey(msg tea.KeyPressMsg) {
	page := max(m.reposCapacity(m.layout().Repos)-1, 1)

	if next, ok := m.keys.navigate(msg, m.cursor, len(m.visible())-1, page); ok {
		m.cursor = next
		m.cursorMoved = true
	}
}

// navigate applies a navigation key to a cursor over the rows [0, last],
// moving page rows at a time for pgup and pgdown. It reports false when msg
// is no navigation key.
func (k keyMap) navigate(msg tea.KeyPressMsg, cursor, last, page int) (int, bool) {
	switch {
	case key.Matches(msg, k.Up):
		cursor--
	case key.Matches(msg, k.Down):
		cursor++
	case key.Matches(msg, k.PageUp):
		cursor -= page
	case key.Matches(msg, k.PageDown):
		cursor += page
	case key.Matches(msg, k.Home):
		cursor = 0
	case key.Matches(msg, k.End):
		cursor = last
	default:
		return cursor, false
	}

	return min(max(cursor, 0), max(last, 0)), true
}

func (m *Model) setFocus(p Pane) {
	m.focus = p
	if p != PaneDiff {
		m.lastLeft = p
	}
}

// back clears a kept filter first in the Repos pane; else it returns from
// the narrow Diff to the last left pane, leaves zoom, or moves the focus to
// the previous pane.
func (m *Model) back() {
	switch {
	case m.filter != "" && m.focus == PaneRepos:
		m.setFilter("")
	case m.diffAutoZoomed():
		m.setFocus(m.lastLeft)
	case m.zoom:
		m.zoom = false
	case m.focus != PaneRepos:
		m.setFocus(m.focus.prev())
	}
}

// toggleOnlyHere shows only repositories with work that exists only here, or
// every repository again, keeping the cursor on its repository when it stays
// visible.
func (m *Model) toggleOnlyHere() {
	id, ok := m.cursorID()
	m.onlyHere = !m.onlyHere
	m.visibleChanged(id, ok)
}

// visibleChanged follows every change of the rows the Repos pane may show:
// a folder tab switch, f, each change of the filter, and every refresh that
// stores snapshots (rows load, and f hides a repository whose work only here
// is gone). id is the repository the cursor is to stay on, ok false for
// none. The cursor goes back to id when it is still visible, and then counts
// as moved, so that a later refresh keeps it on the repository instead of on
// its row; otherwise it stays in range. The selected repositories that are
// hidden now, or that a refresh found not safe to pull, are unselected (see
// dropSelections).
func (m *Model) visibleChanged(id repo.ID, ok bool) {
	if m.restoreCursor(id, ok) {
		m.cursorMoved = true
	}

	m.dropSelections()
}

// applySnapshots stores refreshed snapshots in their rows, re-sorts the rows
// when sort is set and drops the selections that no longer apply:
// repositories hidden now or not safe to pull any more, and gone branches
// that are gone no more. Once the user has moved the cursor it stays on its
// repository; until then it stays on the first row, which ends up being the
// most urgent one. When the pull of the cursor's repository lands and the
// rows sort, the cursor keeps its row instead (see pullLanded). The rows are
// copied first: the previous model shares their backing array.
func (m *Model) applySnapshots(snaps []drift.Snapshot, sort bool) {
	if len(snaps) == 0 {
		return
	}

	cur, ok := m.cursorRow()
	id := cur.snap.Loc.ID
	ok = ok && m.cursorMoved && !(sort && m.pullLanded(cur, snaps))
	m.rows = slices.Clone(m.rows)

	for _, s := range snaps {
		r := row{snap: s, loaded: true, category: drift.CategoryOf(s)}

		i := slices.IndexFunc(m.rows, func(r row) bool { return r.snap.Loc.ID == s.Loc.ID })
		if i < 0 {
			m.rows = append(m.rows, r)

			continue
		}

		m.rows[i] = r
	}

	if sort {
		sortRows(m.rows)
	}

	// ok holds only once the cursor has moved, so visibleChanged keeps
	// cursorMoved as it is. It also drops the selected repositories that
	// are hidden now or not safe to pull any more.
	m.visibleChanged(id, ok)
	m.pruneGoneSelections(snaps)
}

// readAnswered records an answer of group dir and reports whether it was the
// last of the first reads: every group was read once, and the fetches are
// to start.
func (m *Model) readAnswered(dir string) bool {
	if !m.unread[dir] {
		return false
	}

	m.unread = writable(m.unread)
	delete(m.unread, dir)

	return len(m.unread) == 0
}

// pullLanded reports whether snaps show r up to date for the first time
// after a successful pull of it. The row then drops to the up-to-date rows,
// and the cursor keeps its place so that the repository below moves up
// under it, ready for the next pull. A failed pull, or one that leaves the
// repository needing attention, does not count: the cursor stays on the
// repository to show what happened.
func (m Model) pullLanded(r row, snaps []drift.Snapshot) bool {
	res, pulled := m.results[r.snap.Loc.ID]
	if !pulled || res.err || r.rank() == drift.CategoryUpToDate {
		return false
	}

	i := slices.IndexFunc(snaps, func(s drift.Snapshot) bool { return s.Loc.ID == r.snap.Loc.ID })

	return i >= 0 && drift.CategoryOf(snaps[i]) == drift.CategoryUpToDate
}

// visible returns the indexes into rows of the rows the Repos pane shows:
// those of the active folder tab that pass the filters.
func (m Model) visible() []int {
	out := make([]int, 0, len(m.rows))

	for i, r := range m.rows {
		if m.inFolderTab(r) && m.matchesFilters(r) {
			out = append(out, i)
		}
	}

	return out
}

// matchesFilters reports whether r passes the filters, whatever its folder
// tab: with f on, only repositories with work only here pass, and with a
// filter only those whose name or folder contains it.
func (m Model) matchesFilters(r row) bool {
	if m.onlyHere && (!r.loaded || !hasOnlyHere(r.snap)) {
		return false
	}

	return m.filter == "" || matchesFilter(r, m.filter)
}

// cursorRow returns the row under the cursor.
func (m Model) cursorRow() (row, bool) {
	vis := m.visible()
	if m.cursor < 0 || m.cursor >= len(vis) {
		return row{}, false
	}

	return m.rows[vis[m.cursor]], true
}

func (m Model) cursorID() (repo.ID, bool) {
	r, ok := m.cursorRow()

	return r.snap.Loc.ID, ok
}

// restoreCursor puts the cursor back on repository id when it is visible,
// and otherwise keeps it in range. It reports whether the cursor is on id.
func (m *Model) restoreCursor(id repo.ID, ok bool) bool {
	vis := m.visible()

	if ok {
		if i := slices.IndexFunc(vis, func(i int) bool { return m.rows[i].snap.Loc.ID == id }); i >= 0 {
			m.cursor = i

			return true
		}
	}

	m.cursor = min(max(m.cursor, 0), max(len(vis)-1, 0))

	return false
}

// syncScroll moves the windows of the panes so that their cursors stay
// visible. A pane that is not shown (too small, or another pane is zoomed)
// keeps its window.
func (m *Model) syncScroll() {
	lay := m.layout()
	if lay.Repos.H > 0 {
		m.top, _ = scrollWindow(len(m.visible()), m.cursor, m.top, m.reposCapacity(lay.Repos))
	}

	m.syncDetailScroll(lay)
}
