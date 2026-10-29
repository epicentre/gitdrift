package ui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/repo"
)

// Columns of a compact row, after the status marker and the selection mark.
// Each status token has its own column, so that a kind of token always
// starts at the same column.
const (
	compactName = iota
	compactFolder
	compactBranch
	compactLocal
	compactUpstream
	compactBase
	compactIncoming
	compactHere
	compactGone
	compactColumns
)

// Visible minimums of the compact name, folder and branch columns: too
// narrow a row cuts the folder, then the branch, then the name, down to these
// widths before it drops columns.
const (
	compactNameMin   = 13
	compactFolderMin = 10
	compactBranchMin = 10
)

// Columns of the zoom table, as in the zoom mockup. REPO holds the name and
// the selection mark after it.
const (
	tableRepo = iota
	tableFolder
	tableBranch
	tableLocal
	tableUpstream
	tableBase
	tableIncoming
	tableHere
	tableGone
	tableUpdated
	tableColumns
)

const (
	// tableNameMinWidth and tableBranchMinWidth are the visible minimums of
	// the zoom table's name and branch, the mockup widths.
	tableNameMinWidth   = 11
	tableBranchMinWidth = 14
	// tableSelectWidth is the space and the selection mark after the name.
	tableSelectWidth = 2
)

const (
	shortSHALength = 7
	// none fills a zoom table cell that has nothing to show.
	none = "–"
	// noRemote stands for both the upstream state and the only-here count
	// of a repository without any remote, all of whose work is only here.
	noRemote = "no remote"
	// noUpstream is the upstream state of a branch that tracks nothing.
	noUpstream = "no upstream"
	// rowPrefixWidth is the marker and the selection mark (or the space
	// after the marker) ahead of the name in both row kinds.
	rowPrefixWidth = 2
)

// newCompactLayout fits the compact rows of rows, every row of the list in
// the folder tab named tab, in rows width cells wide. A column with values is
// at least as wide as its title, so that the header is never cut; a column
// without values takes no room, so a flat workspace, or a tab whose
// repositories sit directly in it, has no FOLDER column. Too narrow a row
// cuts the folder from the left (see cutFolder), then the branch, then the
// name, down to their minimums, then drops columns and their titles: GONE,
// HERE, INCOMING, BASE, FOLDER, then LOCAL, and UPSTREAM, the main drift
// signal, last.
func newCompactLayout(rows []row, tab string, width int, st Styles) columnLayout {
	cols := []column{
		compactName:     {min: compactNameMin, shrink: 3},
		compactFolder:   {min: compactFolderMin, shrink: 1, drop: 5, cut: cutFolder},
		compactBranch:   {min: compactBranchMin, shrink: 2},
		compactLocal:    {drop: 6},
		compactUpstream: {drop: 7},
		compactBase:     {drop: 4},
		compactIncoming: {drop: 3, style: st.Badge},
		compactHere:     {drop: 2},
		compactGone:     {drop: 1},
	}

	for _, r := range rows {
		include(cols, compactCells(r.snap, r.loaded, tab))
	}

	for i, title := range compactHeader() {
		if cols[i].width > 0 {
			cols[i].width = max(cols[i].width, ansi.StringWidth(title))
		}
	}

	return layoutColumns(cols, width-rowPrefixWidth)
}

// compactHeader returns the column titles of the compact rows.
func compactHeader() []string {
	return []string{"REPO", "FOLDER", "BRANCH", "LOCAL", "UPSTREAM", "BASE", "INCOMING", "HERE", "GONE"}
}

// repoHeader renders the titles of the columns l shows, muted and aligned
// with repoRow; the marker and selection columns have none.
func repoHeader(l columnLayout, st Styles) string {
	return strings.Repeat(" ", rowPrefixWidth) + l.withStyle(st.Heading).render(compactHeader())
}

// compactCells returns the cells of the compact row of s in the folder tab
// named tab: its name and folder (see folderLabel) and, once it was read, its
// branch and status tokens; a cell is empty when its token does not apply.
func compactCells(s drift.Snapshot, loaded bool, tab string) []string {
	cells := make([]string, compactColumns)
	cells[compactName] = sanitize(s.Loc.Name)
	cells[compactFolder] = sanitize(folderLabel(s.Loc.Folder, tab))

	if !loaded {
		return cells
	}

	cells[compactBranch] = branchLabel(s.Status)
	cells[compactUpstream] = upstreamLabel(s, "", "ok")

	if n := localChanges(s.Status); n > 0 {
		cells[compactLocal] = "*" + strconv.Itoa(n)
	}

	switch behind := baseBehind(s.Base); {
	case s.Base.NotFound:
		cells[compactBase] = "base?"
	case behind > 0:
		cells[compactBase] = "base↓" + strconv.Itoa(behind)
	}

	cells[compactIncoming] = strings.Join(sanitizeAll(s.Badges), " ")

	// Without a remote the upstream token already says so in place of ⌂N.
	if n := s.Work.OnlyHereTotal(); n > 0 && !s.Work.NoRemote {
		cells[compactHere] = "⌂" + strconv.Itoa(n)
	}

	if n := len(s.Work.Gone); n > 0 {
		cells[compactGone] = "✂" + strconv.Itoa(n)
	}

	return cells
}

// repoRow renders the compact row of one repository in the folder tab named
// tab laid out by l: status marker, pull selection mark, then the columns. A
// row not read yet shows "fetching…", and a row with a note shows the note,
// across the status columns.
func repoRow(s drift.Snapshot, loaded, selected bool, note token, tab string, l columnLayout, st Styles) string {
	prefix := marker(s, loaded, st) + selectMark(selected, " ", st)
	cells := compactCells(s, loaded, tab)

	if !loaded {
		note = token{text: fetching, style: st.Muted}
	}

	if note.text != "" {
		return prefix + l.renderSpan(cells, compactLocal, compactColumns, note, alignLeft)
	}

	return prefix + l.render(cells)
}

// newTableLayout fits the zoom table of rows, every row of the list in the
// folder tab named tab, in rows width cells wide. Each column is as wide as
// its title or its longest value; FOLDER is shown only when a row has a
// folder below the tab, and then always in full. Too narrow a row cuts the
// branch, then the name, down to the mockup widths, then drops columns from
// the right.
func newTableLayout(rows []row, tab string, width int, now time.Time) columnLayout {
	cols := []column{
		tableRepo:     {min: tableNameMinWidth + tableSelectWidth, shrink: 2},
		tableFolder:   {},
		tableBranch:   {min: tableBranchMinWidth, shrink: 1},
		tableLocal:    {drop: 7},
		tableUpstream: {drop: 6},
		tableBase:     {drop: 5},
		tableIncoming: {drop: 4},
		tableHere:     {drop: 3},
		tableGone:     {drop: 2},
		tableUpdated:  {drop: 1},
	}

	for _, r := range rows {
		include(cols, tableCells(r.snap, r.loaded, tab, now))

		name := ansi.StringWidth(sanitize(r.snap.Loc.Name)) + tableSelectWidth
		cols[tableRepo].width = max(cols[tableRepo].width, name)
	}

	for i, title := range tableHeader() {
		if i != tableFolder || cols[i].width > 0 {
			cols[i].width = max(cols[i].width, ansi.StringWidth(title))
		}
	}

	return layoutColumns(cols, width-rowPrefixWidth)
}

// tableHeader returns the column titles of the zoom table.
func tableHeader() []string {
	return []string{"REPO", "FOLDER", "BRANCH", "LOCAL", "UPSTREAM", "VS BASE", "INCOMING", "HERE", "GONE", "UPDATED"}
}

// tableCells returns the cells of the zoom table row of s in the folder tab
// named tab after REPO, which depends on the column width (see
// tableRepoCell); a row not read yet has only its folder.
func tableCells(s drift.Snapshot, loaded bool, tab string, now time.Time) []string {
	cells := make([]string, tableColumns)
	cells[tableFolder] = sanitize(folderLabel(s.Loc.Folder, tab))

	if !loaded {
		return cells
	}

	cells[tableBranch] = branchLabel(s.Status)
	cells[tableLocal] = localLabel(s.Status)
	cells[tableUpstream] = upstreamLabel(s, " ", "up to date")
	cells[tableBase] = baseLabel(s.Base)
	cells[tableIncoming] = incomingLabel(s.Badges)
	cells[tableHere] = hereLabel(s.Work)
	cells[tableGone] = countLabel(len(s.Work.Gone))
	cells[tableUpdated] = updatedLabel(s.Status.LastUpdate, now)

	return cells
}

// tableRepoCell is the REPO cell, width cells wide: the name, cut with "…"
// to leave room for the selection mark, then the mark.
func tableRepoCell(s drift.Snapshot, selected bool, width int, st Styles) string {
	name := place(token{text: sanitize(s.Loc.Name)}, width-tableSelectWidth, alignLeft, false)

	return name + " " + selectMark(selected, " ", st)
}

// repoTableHeader renders the zoom table's column titles, aligned with
// repoTableRow.
func repoTableHeader(l columnLayout) string {
	return strings.Repeat(" ", rowPrefixWidth) + l.render(tableHeader())
}

// repoTableRow renders one repository in the folder tab named tab as a zoom
// table row laid out by l. A row not read yet shows "fetching…", and a row
// with a note shows the note, in place of the columns after BRANCH.
func repoTableRow(
	s drift.Snapshot, loaded, selected bool, note token, tab string, l columnLayout, st Styles, now time.Time,
) string {
	prefix := marker(s, loaded, st) + " "
	cells := tableCells(s, loaded, tab, now)
	cells[tableRepo] = tableRepoCell(s, selected, l.widths[tableRepo], st)

	if !loaded {
		note = token{text: fetching, style: st.Muted}
	}

	if note.text != "" {
		return prefix + l.renderSpan(cells, tableLocal, tableColumns, note, alignLeft)
	}

	return prefix + l.render(cells)
}

// marker is the status mark: ! needs attention, ↓ behind, ✓ up to date and
// blank while the repository has not been read yet.
func marker(s drift.Snapshot, loaded bool, st Styles) string {
	if !loaded {
		return " "
	}

	switch drift.CategoryOf(s) {
	case drift.CategoryNeedsAttention:
		return st.Attention.Render("!")
	case drift.CategoryBehind:
		return st.Behind.Render("↓")
	case drift.CategoryUpToDate:
		return st.UpToDate.Render("✓")
	case drift.CategoryUnknown:
		return " "
	default:
		return " "
	}
}

// selectMark is ● followed by padding when selected, or blank of the same
// width.
func selectMark(selected bool, blank string, st Styles) string {
	if !selected {
		return blank
	}

	return st.Selected.Render("●") + blank[1:]
}

// folderLabel is folder, a repository's slash-separated folder below the
// workspace, as the FOLDER column shows it in the folder tab named tab: the
// path below the tab, empty for a repository directly in it. In the All tab
// ("") it is the whole folder, empty at the root.
func folderLabel(folder, tab string) string {
	if tab == "" {
		return folder
	}

	if folder == tab {
		return ""
	}

	if below, ok := strings.CutPrefix(folder, tab+"/"); ok {
		return below
	}

	return folder
}

// folderCutMark starts a folder cut at a folder boundary.
const folderCutMark = "…/"

// cutFolder shortens folder to at most width cells for the FOLDER column:
// whole when it fits, else "…/" and as many of its last folder names, each
// whole, as fit; only when even "…/" and the last name are too wide is it
// cut from the left like any value (see cutLeft).
func cutFolder(folder string, width int) string {
	if ansi.StringWidth(folder) <= width {
		return folder
	}

	var kept string

	for i := strings.LastIndex(folder, "/"); i >= 0; i = strings.LastIndex(folder[:i], "/") {
		if ansi.StringWidth(folderCutMark+folder[i+1:]) > width {
			break
		}

		kept = folder[i+1:]
	}

	if kept == "" {
		return cutLeft(folder, width)
	}

	return folderCutMark + kept
}

// branchLabel is the checked-out branch, or the short HEAD in parentheses
// when detached.
func branchLabel(st repo.Status) string {
	if st.Detached {
		return "(" + shortSHA(st.HeadSHA) + ")"
	}

	return sanitize(st.Branch)
}

func shortSHA(sha string) string {
	if len(sha) > shortSHALength {
		return sha[:shortSHALength]
	}

	return sha
}

// upstreamLabel describes the branch against its upstream. gap separates the
// ahead and behind counts; upToDate is shown when there is nothing to report.
func upstreamLabel(s drift.Snapshot, gap, upToDate string) string {
	st := s.Status

	switch {
	case s.Work.NoRemote:
		return noRemote
	case st.UpstreamGone:
		return "gone"
	case st.Upstream == "":
		return noUpstream
	case st.Ahead > 0 && st.Behind > 0:
		return "↑" + strconv.Itoa(st.Ahead) + gap + "↓" + strconv.Itoa(st.Behind)
	case st.Behind > 0:
		return "↓" + strconv.Itoa(st.Behind)
	case st.Ahead > 0:
		return "↑" + strconv.Itoa(st.Ahead)
	default:
		return upToDate
	}
}

// baseBehind counts the base commits the branch lacks; 0 on the base itself.
func baseBehind(b repo.Base) int {
	if b.OnBase {
		return 0
	}

	return b.Behind
}

func baseLabel(b repo.Base) string {
	if b.NotFound {
		return "base?"
	}

	if n := baseBehind(b); n > 0 {
		return "↓" + strconv.Itoa(n)
	}

	return none
}

func localLabel(st repo.Status) string {
	switch n := st.Tracked + st.Untracked; n {
	case 0:
		return "clean"
	case 1:
		return "1 file"
	default:
		return strconv.Itoa(n) + " files"
	}
}

func incomingLabel(badges []string) string {
	if len(badges) == 0 {
		return none
	}

	return strings.Join(sanitizeAll(badges), separator)
}

// hereLabel is the HERE cell: the commits and stashes only here, or "no
// remote" when the repository has no remote at all.
func hereLabel(w repo.Work) string {
	if w.NoRemote {
		return noRemote
	}

	return countLabel(w.OnlyHereTotal())
}

func countLabel(n int) string {
	if n == 0 {
		return none
	}

	return strconv.Itoa(n)
}

func updatedLabel(t, now time.Time) string {
	if t.IsZero() {
		return none
	}

	age := relAge(t, now)
	if age == "now" {
		return age
	}

	return age + " ago"
}

// scrollWindow returns the rows [start, end) of total rows to show in
// capacity lines so that cursor stays visible, moving as little as possible
// from the previous first row top. When rows remain below the window, the
// window leaves the last line free for "… N more".
func scrollWindow(total, cursor, top, capacity int) (start, end int) {
	if total <= 0 || capacity <= 0 {
		return 0, 0
	}

	if total <= capacity {
		return 0, total
	}

	cursor = min(max(cursor, 0), total-1)

	if capacity < 2 {
		return cursor, cursor + 1
	}

	// last is the first row of the bottom window, which needs no "more" line.
	last := total - capacity
	page := capacity - 1

	top = min(max(top, 0), last, cursor)

	if top < last && cursor >= top+page {
		top = min(cursor-page+1, last)
	}

	if top == last {
		return top, total
	}

	return top, top + page
}

// row is one repository of the Repos pane.
type row struct {
	snap drift.Snapshot
	// loaded is false until the first refresh of the repository returns.
	loaded   bool
	category drift.Category
}

// rank orders rows by category, with rows not read yet last.
func (r row) rank() drift.Category {
	if !r.loaded || r.category == drift.CategoryUnknown {
		return drift.CategoryUpToDate + 1
	}

	return r.category
}

// sortRows orders rows by category, rows not read yet last, then by folder,
// name and path (see drift.CompareLocations). In a flat workspace every
// folder is empty, so the order is by category, then name.
func sortRows(rows []row) {
	slices.SortStableFunc(rows, func(a, b row) int {
		return cmp.Or(cmp.Compare(a.rank(), b.rank()), drift.CompareLocations(a.snap.Loc, b.snap.Loc))
	})
}
