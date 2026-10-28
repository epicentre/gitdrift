package ui

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/epicentre/gitdrift/internal/badge"
	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	untrackedStatus = "?"
	binaryLabel     = "binary"
	noOnlyHereText  = "no commits only on this machine"
	// fileGap separates the path, the line counts and the badges.
	fileGap = 2
	// filePathMin is the narrowest the path column gets before the badges
	// are dropped.
	filePathMin = 12
)

// Columns of a file row: status letter, path, added and deleted lines, and
// the labels of the badge rules the path matches.
const (
	fileStatus = iota
	filePath
	fileAdded
	fileDeleted
	fileBadges
	fileColumns
)

// filesTitle is the Files pane title: the file count once listed, or in the
// Branches tab the row whose commits exist only here.
func (m Model) filesTitle() string {
	if m.detail.tab == TabBranches {
		if it, ok := m.cursorBranchItem(); ok {
			return titleOnlyHere + separator + sanitize(it.ref)
		}

		return titleFiles
	}

	if f := m.detail.files; f.ready {
		return titleFiles + " (" + strconv.Itoa(len(f.files)) + ")"
	}

	return titleFiles
}

// filesLines renders the inside of the Files pane.
func (m Model) filesLines(r Rect) []string {
	f := m.detail.files

	switch {
	case f.err != nil:
		return []string{errorLine(f.err, m.styles)}
	case f.loading:
		return []string{" " + m.styles.Muted.Render(loadingText)}
	}

	n, hidden := m.fileEntries()
	if n == 0 && f.ready && m.detail.tab == TabBranches {
		return []string{" " + m.styles.Muted.Render(noOnlyHereText)}
	}

	// The rows are laid out from every entry, so that scrolling does not move
	// their columns.
	width := r.W - 2 - compactGutter
	now := m.now()
	start, end, more := listWindow(n, hidden, f.cursor, f.top, innerHeight(r))
	lines := make([]string, 0, end-start+1)

	var entry func(i int) string

	if m.detail.tab == TabBranches {
		l := newCommitLayout(f.commits, width, now)
		entry = func(i int) string { return commitRow(f.commits[i], l, now, m.styles) }
	} else {
		l := f.list.layout(width, m.styles)
		entry = func(i int) string { return fileRow(f.list.lines[i], l) }
	}

	for i := start; i < end; i++ {
		lines = append(lines, cursorGutter(i == f.cursor, m.styles)+entry(i))
	}

	return appendMore(lines, more, m.styles)
}

// fileList is a list of changed files as the Files pane draws it: the cells
// of each file and the columns measured over all of them. It is computed
// once, when the list arrives, so that a View neither matches badge rules
// nor sanitizes paths; a list never changes after it arrived.
type fileList struct {
	lines []fileLine
	cols  []column
}

// fileLine is one file of a fileList.
type fileLine struct {
	cells  []string
	binary bool
}

// newFileList prepares files for the Files pane, with the labels of the
// rules each path matches.
func newFileList(files []repo.FileChange, rules []badge.Rule) fileList {
	cols := []column{
		fileStatus:  {},
		filePath:    {fill: true, shrink: 1, min: filePathMin, keepEnd: true},
		fileAdded:   {align: alignRight, gap: fileGap},
		fileDeleted: {align: alignRight},
		fileBadges:  {gap: fileGap, drop: 1},
	}

	lines := make([]fileLine, len(files))

	var binary bool

	for i, f := range files {
		lines[i] = fileLine{cells: fileCells(f, rules), binary: f.Binary}
		include(cols, lines[i].cells)

		binary = binary || f.Binary
	}

	// A binary file shows its label across both count columns.
	if binary {
		need := ansi.StringWidth(binaryLabel)
		if d := cols[fileDeleted]; d.width > 0 {
			need -= d.minGap() + d.width
		}

		cols[fileAdded].width = max(cols[fileAdded].width, need)
	}

	return fileList{lines: lines, cols: cols}
}

// layout fits the list in rows width cells wide: the status letter, the path
// taking the width left, the added and deleted lines right-aligned in their
// own columns, and the badges. A path that does not fit loses its start, so
// that the file name stays; too narrow a row drops the badges.
func (fl fileList) layout(width int, st Styles) columnLayout {
	if fl.cols == nil {
		fl = newFileList(nil, nil)
	}

	cols := slices.Clone(fl.cols)
	cols[fileAdded].style = st.DiffAdd
	cols[fileDeleted].style = st.DiffDel
	cols[fileBadges].style = st.Badge

	return layoutColumns(cols, width-len(listIndent))
}

// fileCells returns the cells of a file row: the counts of a text file that
// git tracks, and the labels of the rules its path matches.
func fileCells(f repo.FileChange, rules []badge.Rule) []string {
	cells := make([]string, fileColumns)
	cells[fileStatus] = sanitize(f.Status)
	cells[filePath] = sanitize(f.Path)

	if !f.Binary && f.Status != untrackedStatus {
		cells[fileAdded] = "+" + strconv.Itoa(f.Added)
		cells[fileDeleted] = "-" + strconv.Itoa(f.Deleted)
	}

	cells[fileBadges] = strings.Join(sanitizeAll(badge.Match(rules, []string{f.Path})), " ")

	return cells
}

// fileRow renders one file of a list laid out by l: status letter, path,
// added and deleted lines, or "binary" across both, and badges.
func fileRow(line fileLine, l columnLayout) string {
	if line.binary {
		return listIndent + l.renderSpan(line.cells, fileAdded, fileBadges, token{text: binaryLabel}, alignRight)
	}

	return listIndent + l.render(line.cells)
}

// stashCommit is a stash entry as the only commit of its Branches row.
func stashCommit(s repo.Stash) repo.Commit {
	return repo.Commit{SHA: s.SHA, Short: shortSHA(s.SHA), Subject: s.Subject, When: s.When}
}

// fileRules are the badge rules the Files pane matches. Badges say what a
// pull brings in, so only the Incoming tab's files get them.
func (m Model) fileRules() []badge.Rule {
	if s, ok := m.detailSnapshot(); ok && m.detail.tab == TabIncoming {
		return s.BadgeRules
	}

	return nil
}

// fileEntries returns how many entries the Files pane lists and, for the
// commits of a branch, how many more exist than the listing limit loaded.
func (m Model) fileEntries() (n, hidden int) {
	f := m.detail.files
	if !f.ready {
		return 0, 0
	}

	if m.detail.tab != TabBranches {
		return len(f.files), 0
	}

	n = len(f.commits)
	if it, ok := m.cursorBranchItem(); ok && n >= repo.MaxCommits {
		hidden = max(it.onlyHereCount()-n, 0)
	}

	return n, hidden
}

// fileChange returns the file under the Files cursor.
func (m Model) fileChange() (repo.FileChange, bool) {
	f := m.detail.files
	if m.detail.tab == TabBranches || f.cursor < 0 || f.cursor >= len(f.files) {
		return repo.FileChange{}, false
	}

	return f.files[f.cursor], true
}

// onlyHereCommit returns the commit under the Files cursor in the Branches
// tab.
func (m Model) onlyHereCommit() (repo.Commit, bool) {
	f := m.detail.files
	if m.detail.tab != TabBranches || f.cursor < 0 || f.cursor >= len(f.commits) {
		return repo.Commit{}, false
	}

	return f.commits[f.cursor], true
}

// moveFilesCursor moves the Files cursor with a navigation key; landing on
// another entry reloads the Diff pane.
func (m *Model) moveFilesCursor(msg tea.KeyPressMsg) tea.Cmd {
	f := &m.detail.files
	n, _ := m.fileEntries()
	page := max(innerHeight(m.layout().Files)-1, 1)

	next, ok := m.keys.navigate(msg, f.cursor, n-1, page)
	if !ok || next == f.cursor {
		return nil
	}

	f.cursor = next

	m.selectFile()

	return m.reload()
}
