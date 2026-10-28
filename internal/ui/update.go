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
		m.applySnapshots(msg.snaps)
		cmd := m.refreshAnswered(msg.commonDir)

		id, ok := m.cursorID()

		return cmd, ok && slices.ContainsFunc(msg.snaps, func(s drift.Snapshot) bool { return s.Loc.ID == id })
	case tea.KeyPressMsg:
		return m.handleKey(msg), false
	default:
		if cmd, ok := m.applyAction(msg); ok {
			return cmd, false
		}

		return m.updateDetail(msg), false
	}

	return nil, false
}

// handleKey applies a key press and returns the command it starts, if any.
// ctrl+c always quits. Otherwise an open dialog takes every key, the actions
// work from any pane, and navigation keys move the cursor of the focused
// pane, or scroll the Diff. Each key press clears the notice of the previous
// one.
func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	m.notice = ""

	if key.Matches(msg, m.keys.Interrupt) {
		return tea.Quit
	}

	if m.dialog != nil {
		return m.handleDialogKey(msg)
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

// handlePaneKey moves the focus, zooms and filters. It reports whether the
// key was one of these.
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
	case key.Matches(msg, m.keys.Filter):
		m.toggleOnlyHere()
	default:
		return false
	}

	return true
}

// handleTabKey switches the Commits tab with [ and ], wrapping around. It
// reports whether the key was one of these.
func (m *Model) handleTabKey(msg tea.KeyPressMsg) bool {
	switch {
	case key.Matches(msg, m.keys.NextTab):
		m.detail.tab = m.detail.tab.next()
	case key.Matches(msg, m.keys.PrevTab):
		m.detail.tab = m.detail.tab.prev()
	default:
		return false
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

// back returns from the narrow Diff to the last left pane, leaves zoom, or
// else moves the focus to the previous pane.
func (m *Model) back() {
	switch {
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

	m.restoreCursor(id, ok)
}

// applySnapshots stores refreshed snapshots, re-sorts the rows and drops
// selections that no longer apply. Once the user has moved the cursor it
// stays on its repository; until then it stays on the first row, which ends
// up being the most urgent one. The rows are copied first: the previous
// model shares their backing array.
func (m *Model) applySnapshots(snaps []drift.Snapshot) {
	if len(snaps) == 0 {
		return
	}

	id, ok := m.cursorID()
	ok = ok && m.cursorMoved
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

	sortRows(m.rows)
	m.restoreCursor(id, ok)
	m.pruneSelections(snaps)
}

// visible returns the indexes into rows of the rows the Repos pane shows.
func (m Model) visible() []int {
	out := make([]int, 0, len(m.rows))

	for i, r := range m.rows {
		if !m.onlyHere || (r.loaded && hasOnlyHere(r.snap)) {
			out = append(out, i)
		}
	}

	return out
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
// and otherwise keeps it in range.
func (m *Model) restoreCursor(id repo.ID, ok bool) {
	vis := m.visible()

	if ok {
		if i := slices.IndexFunc(vis, func(i int) bool { return m.rows[i].snap.Loc.ID == id }); i >= 0 {
			m.cursor = i

			return
		}
	}

	m.cursor = min(max(m.cursor, 0), max(len(vis)-1, 0))
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
