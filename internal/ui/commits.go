package ui

import (
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	loadingText = "loading…"
	errorPrefix = "error: "

	// listIndent starts the entries of a detail list after the cursor
	// gutter.
	listIndent = " "
	// moreIndent puts "… N more" under the first column of a detail list.
	moreIndent = "  "
)

// Columns of a commit row: short sha, subject, author and age.
const (
	commitSHA = iota
	commitSubject
	commitAuthor
	commitAge
)

const (
	// authorGap separates the author from the subject, both free text.
	authorGap = "  "
	// commitAuthorMin is the narrowest an author is cut before the subject
	// gives way; commitSubjectMin is the narrowest a subject is cut before
	// the author column is left out.
	commitAuthorMin  = 10
	commitSubjectMin = 20
)

// commitsTitle is the Commits pane title in a pane w cells wide: the pane
// key and name, then the tabs. When the tabs do not fit after the name, the
// name gives way so that the active tab stays whole.
func (m Model) commitsTitle(w int) string {
	s, _ := m.detailSnapshot()
	room := w - boxTitleChrome

	if title := titleCommits + tabTitle(s, m.detail.tab, room-ansi.StringWidth(titleCommits)); ansi.StringWidth(title) <= room {
		return title
	}

	return titleCommitsKey + tabTitle(s, m.detail.tab, room-ansi.StringWidth(titleCommitsKey))
}

// commitsLines renders the inside of the Commits pane.
func (m Model) commitsLines(r Rect) []string {
	s, ok := m.detailSnapshot()
	if !ok {
		return nil
	}

	c := m.detail.commits

	switch {
	case c.err != nil:
		return []string{errorLine(c.err, m.styles)}
	case c.loading:
		return []string{" " + m.styles.Muted.Render(loadingText)}
	}

	n, hidden := m.commitEntries()
	if n == 0 {
		return []string{" " + m.styles.Muted.Render(emptyTabText(s, m.detail.tab))}
	}

	entry := m.commitsEntry(s, r.W-2-compactGutter)
	start, end, more := listWindow(n, hidden, c.cursor, c.top, innerHeight(r))
	lines := make([]string, 0, end-start+1)

	for i := start; i < end; i++ {
		lines = append(lines, cursorGutter(i == c.cursor, m.styles)+entry(i))
	}

	return appendMore(lines, more, m.styles)
}

// commitsEntry returns what draws entry i of the current tab in width cells.
// The commits and the Branches rows are laid out from every entry of the
// tab, so that scrolling does not move their columns.
func (m Model) commitsEntry(s drift.Snapshot, width int) func(i int) string {
	c := m.detail.commits
	now := m.now()

	switch m.detail.tab {
	case TabIncoming, TabBase:
		l := newCommitLayout(c.commits, width, now, m.styles)

		return func(i int) string {
			if i == 0 {
				return listIndent + allLabel(m.detail.tab, c.total)
			}

			return commitRow(c.commits[i-1], l, now, m.styles)
		}
	case TabBranches:
		l := newBranchLayout(c.items, width)

		return func(i int) string {
			it := c.items[i]
			selected := m.goneSel[goneKey{commonDir: s.Loc.CommonDir, branch: it.ref}]

			return it.marked(selected && it.kind == branchGone).row(l, m.styles)
		}
	case TabLocal:
		return func(int) string {
			return listIndent + "Uncommitted changes (" + countNoun(localChanges(s.Status), "file") + ")"
		}
	default:
		return func(int) string { return "" }
	}
}

// allLabel is the first entry of the Incoming and Base tabs, which stands
// for every commit of the range.
func allLabel(t Tab, total int) string {
	label := "All incoming"
	if t == TabBase {
		label = "All behind base"
	}

	return label + " (" + strconv.Itoa(total) + ")"
}

// emptyTabText explains a tab that has nothing to list.
func emptyTabText(s drift.Snapshot, t Tab) string {
	switch t {
	case TabIncoming:
		return "no incoming commits"
	case TabBase:
		switch {
		case s.Base.Ref == "":
			return "no base"
		case s.Base.OnBase:
			return "on the base branch"
		default:
			return "not behind base"
		}
	case TabLocal:
		return "no local changes"
	case TabBranches:
		return "nothing only here"
	default:
		return ""
	}
}

// newCommitLayout fits a list of commits in rows width cells wide: the sha,
// the subject taking the width left and cut with "…" when there is none, the
// author muted, and the age right-aligned in a column as wide as the longest
// age, at the right edge. When the row is too narrow the author is cut
// first, down to commitAuthorMin, then the subject down to commitSubjectMin;
// if both minimums do not fit, the author column is left out.
func newCommitLayout(commits []repo.Commit, width int, now time.Time, st Styles) columnLayout {
	return layoutColumns(commitColumnsOf(commits, now, st), width-len(listIndent))
}

// commitColumnsOf measures the columns of a list of commits.
func commitColumnsOf(commits []repo.Commit, now time.Time, st Styles) []column {
	cols := []column{
		commitSHA:     {},
		commitSubject: {fill: true, shrink: 2, min: commitSubjectMin},
		commitAuthor:  {shrink: 1, min: commitAuthorMin, drop: 1, gap: len(authorGap), style: st.Muted},
		commitAge:     {align: alignRight, gap: len(ageGap)},
	}

	for _, c := range commits {
		include(cols, commitCells(c, now))
	}

	return cols
}

// commitCells returns the cells of a commit row.
func commitCells(c repo.Commit, now time.Time) []string {
	return []string{
		commitSHA:     commitShort(c),
		commitSubject: sanitize(c.Subject),
		commitAuthor:  sanitize(c.Author),
		commitAge:     relAge(c.When, now),
	}
}

// commitRow renders one commit of a list laid out by l: short sha, subject,
// author and age. Merge commits are muted whole.
func commitRow(c repo.Commit, l columnLayout, now time.Time, st Styles) string {
	if c.Merge {
		l = l.withStyle(st.Muted)
	}

	return listIndent + l.render(commitCells(c, now))
}

// commitShort is the abbreviated id of c.
func commitShort(c repo.Commit) string {
	if c.Short != "" {
		return sanitize(c.Short)
	}

	return shortSHA(c.SHA)
}

// cursorGutter marks the entry under the cursor of a detail list.
func cursorGutter(isCursor bool, st Styles) string {
	if isCursor {
		return st.Cursor.Render(cursorMark)
	}

	return " "
}

// appendMore ends a list with "… N more" when entries remain below it.
func appendMore(lines []string, more int, st Styles) []string {
	if more <= 0 {
		return lines
	}

	return append(lines, moreIndent+st.Muted.Render("… "+strconv.Itoa(more)+" more"))
}

// errorLine shows the first line of a failed query.
func errorLine(err error, st Styles) string {
	return " " + st.Attention.Render(errorPrefix+firstLine(err.Error()))
}

// countNoun renders n with noun, adding an s unless n is 1.
func countNoun(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}

	return strconv.Itoa(n) + " " + noun + "s"
}

// commitEntries returns how many entries the Commits pane lists and how
// many more commits the range has that were not loaded.
func (m Model) commitEntries() (n, hidden int) {
	c := m.detail.commits

	s, ok := m.detailSnapshot()
	if !ok || c.loading || c.err != nil {
		return 0, 0
	}

	switch m.detail.tab {
	case TabIncoming, TabBase:
		if _, ok := rangeTarget(s, m.detail.tab); !ok {
			return 0, 0
		}

		return 1 + len(c.commits), max(c.total-len(c.commits), 0)
	case TabLocal:
		if localChanges(s.Status) > 0 {
			return 1, 0
		}
	case TabBranches:
		return len(c.items), 0
	}

	return 0, 0
}

// cursorBranchItem returns the Branches row under the Commits cursor.
func (m Model) cursorBranchItem() (branchItem, bool) {
	c := m.detail.commits
	if m.detail.tab != TabBranches || c.cursor < 0 || c.cursor >= len(c.items) || !c.items[c.cursor].selectable() {
		return branchItem{}, false
	}

	return c.items[c.cursor], true
}

// moveCommitsCursor moves the Commits cursor with a navigation key; landing
// on another entry reloads the Files and Diff panes.
func (m *Model) moveCommitsCursor(msg tea.KeyPressMsg) tea.Cmd {
	c := &m.detail.commits
	n, _ := m.commitEntries()
	page := max(innerHeight(m.layout().Commits)-1, 1)

	next, ok := m.keys.navigate(msg, c.cursor, n-1, page)
	if !ok {
		return nil
	}

	if m.detail.tab == TabBranches {
		dir := 1
		if next < c.cursor {
			dir = -1
		}

		if next, ok = nearestSelectable(c.items, next, dir); !ok {
			return nil
		}
	}

	if next == c.cursor {
		return nil
	}

	c.cursor = next

	m.selectEntry()

	return m.reload()
}
