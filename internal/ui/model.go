package ui

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/repo"
	"github.com/epicentre/gitdrift/internal/workspace"
)

const (
	tooSmallNotice = "terminal too small (need 60×15)"

	// Pane titles; the number is the key that focuses the pane. The tabs
	// follow titleCommits, or titleCommitsKey where the name does not fit.
	titleRepos      = "1 Repos"
	zoomedMark      = "zoomed"
	titleCommitsKey = "2 "
	titleCommits    = titleCommitsKey + "Commits" + tabGap
	titleFiles      = "3 Files"
	titleOnlyHere   = "3 Only here"
	titleDiff       = "4 Diff"

	// compactHeaderRows is the least inner height at which the Repos pane
	// heads its compact rows with column titles; a collapsed pane gives
	// every line to repositories.
	compactHeaderRows = 4

	// Cursor gutters ahead of compact rows and zoom table rows.
	cursorMark        = "▸"
	compactGutter     = 1
	tableGutter       = 3
	tableCursorGutter = " " + cursorMark + " "
)

// Model is the Bubble Tea model of the dashboard.
type Model struct {
	ctx     context.Context //nolint:containedctx // Bubble Tea commands have no call chain; the program context is the only cancellation source
	backend Backend
	groups  []workspace.Group
	root    string
	home    string
	now     func() time.Time
	keys    keyMap
	styles  Styles

	width, height   int
	focus, lastLeft Pane
	zoom            bool
	onlyHere        bool
	// folderTab is the top-level folder whose repositories the Repos pane
	// shows, "" for All.
	folderTab string
	// filter narrows the Repos pane to the repositories whose name or
	// folder contains it, ignoring case; "" shows every one. filtering is
	// set while it is typed.
	filter    string
	filtering bool

	// rows holds every repository, sorted; cursor indexes the visible rows
	// and top is the first visible row the Repos pane shows.
	rows        []row
	cursor, top int
	// cursorMoved is set once the user moves the cursor; from then on the
	// cursor stays on its repository when the rows re-sort.
	cursorMoved bool

	// detail is the Commits, Files and Diff panes of the cursor's
	// repository.
	detail detail

	// selected holds the repositories selected for pull and pulling those
	// with a pull in flight. results keeps the outcome of each repository's
	// last pull until r or R refreshes it.
	selected map[repo.ID]bool
	pulling  map[repo.ID]bool
	results  map[repo.ID]result
	// goneSel holds the gone branches selected for deletion.
	goneSel map[goneKey]bool
	// refreshing maps the common directory of each group with a refresh in
	// flight to whether it fetches; queued holds the refresh requested
	// meanwhile, which starts once that one answers.
	refreshing, queued map[string]bool
	// unread holds the groups whose first read has not answered yet; once
	// the last one answers, every group fetches. fetchBatch holds the groups
	// asked to fetch since the last time no fetch was running, which the
	// header counts.
	unread, fetchBatch map[string]bool
	// spinning is set while spinner ticks come; spinFrame is the frame that
	// loading rows show.
	spinning  bool
	spinFrame int

	// notice replaces the status line until the next key press.
	notice string
	// dialog is the window open over the dashboard, nil when there is none;
	// dialogTop is the first body line it shows. waiting holds the delete
	// dialogs and results that arrived while another window was open.
	dialog    dialog
	dialogTop int
	waiting   []dialog
}

var _ tea.Model = Model{}

// New creates the model for the repositories of b found under root. Every
// repository starts as a loading row until its first read, which Init
// starts without a fetch, returns.
func New(ctx context.Context, b Backend, root string) Model {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	m := Model{
		ctx:      ctx,
		backend:  b,
		groups:   b.Groups(),
		root:     root,
		home:     home,
		now:      time.Now,
		keys:     defaultKeyMap(),
		styles:   newStyles(true),
		focus:    PaneRepos,
		lastLeft: PaneRepos,
		detail:   detail{tab: TabIncoming},
	}

	m.refreshing = make(map[string]bool, len(m.groups))
	m.unread = make(map[string]bool, len(m.groups))

	for _, g := range m.groups {
		m.refreshing[g.CommonDir] = false
		m.unread[g.CommonDir] = true

		for _, loc := range g.Members {
			m.rows = append(m.rows, row{snap: drift.Snapshot{Loc: loc}})
		}
	}

	sortRows(m.rows)
	m.spinning = len(m.groups) > 0

	return m
}

// Init asks for the terminal background, turns the spinner and reads every
// group without a fetch, so that the rows show and sort by what is known
// locally; the fetches start once every group was read (see readAnswered).
func (m Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.groups)+2)
	cmds = append(cmds, tea.RequestBackgroundColor)

	if m.spinning {
		cmds = append(cmds, spinTick())
	}

	for _, g := range m.groups {
		cmds = append(cmds, refreshCmd(m.ctx, m.backend, g.CommonDir, false))
	}

	return tea.Batch(cmds...)
}

// View renders the whole screen in the alternate screen buffer.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true

	return v
}

func (m Model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	lay := m.layout()
	if lay.TooSmall {
		return tooSmallNotice
	}

	screen := strings.Join([]string{
		headerLine(tildePath(m.root, m.home), m.loadedSnapshots(), len(m.rows), m.progress(), m.width, m.styles),
		m.body(lay),
		m.status(),
		footerLine(m.focus, m.zoom, folderTabs(m.rows) != nil, m.detail.tab, m.width, m.styles),
	}, "\n")

	return m.overlayDialog(screen)
}

// layout places the panes for the current size, focus and zoom.
func (m Model) layout() Layout {
	return ComputeLayout(m.width, m.height, m.focus, m.lastLeft, m.zoom || m.diffAutoZoomed())
}

// diffAutoZoomed reports whether the focused Diff pane takes the whole body
// because the screen is too narrow to show it beside the left column. This
// is plain focus, not a zoom: z does nothing and esc returns to the last
// left pane.
func (m Model) diffAutoZoomed() bool {
	return m.focus == PaneDiff && m.width < narrowWidth
}

// tableMode reports whether the Repos pane shows the zoom table.
func (m Model) tableMode() bool { return m.zoom && m.focus == PaneRepos }

// body draws the panes lay places: Repos on top, below it Commits stacked on
// Files with Diff beside them. Under zoom only one pane is placed.
func (m Model) body(lay Layout) string {
	var rows, left []string

	if r := lay.Repos; r.W > 0 && r.H > 0 {
		rows = append(rows, m.pane(PaneRepos, r))
	}

	for _, p := range []Pane{PaneCommits, PaneFiles} {
		if r := lay.rect(p); r.W > 0 && r.H > 0 {
			left = append(left, m.pane(p, *r))
		}
	}

	switch hasDiff := lay.Diff.W > 0 && lay.Diff.H > 0; {
	case hasDiff && len(left) == 0:
		rows = append(rows, m.pane(PaneDiff, lay.Diff))
	case hasDiff:
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.JoinVertical(lipgloss.Left, left...), m.pane(PaneDiff, lay.Diff)))
	case len(left) > 0:
		rows = append(rows, lipgloss.JoinVertical(lipgloss.Left, left...))
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// pane draws pane p in r.
func (m Model) pane(p Pane, r Rect) string {
	focused := p == m.focus

	switch p {
	case PaneRepos:
		return box(m.reposTitle(), m.reposLines(r), r.W, r.H, focused, m.styles)
	case PaneCommits:
		return box(m.commitsTitle(r.W), m.commitsLines(r), r.W, r.H, focused, m.styles)
	case PaneFiles:
		return box(m.filesTitle(), m.filesLines(r), r.W, r.H, focused, m.styles)
	case PaneDiff:
		return box(m.diffTitle(), m.detail.diff.render(r.W-2, innerHeight(r), m.styles), r.W, r.H, focused, m.styles)
	default:
		return ""
	}
}

// reposCapacity is the number of row lines the Repos pane has in r.
func (m Model) reposCapacity(r Rect) int {
	capacity := innerHeight(r) - m.reposTabBarRows(r)
	if m.reposHasHeader(r) {
		capacity-- // column titles
	}

	return max(capacity, 0)
}

// reposTabBarRows is 1 when the Repos pane in r starts with the folder tab
// bar: in a workspace with folders, when a line is left for the rows.
func (m Model) reposTabBarRows(r Rect) int {
	if folderTabs(m.rows) != nil && innerHeight(r) >= 2 {
		return 1
	}

	return 0
}

// reposHasHeader reports whether the Repos pane in r has column titles,
// after the tab bar: the zoom table always does, the compact rows unless the
// pane is collapsed.
func (m Model) reposHasHeader(r Rect) bool {
	return m.tableMode() || innerHeight(r)-m.reposTabBarRows(r) >= compactHeaderRows
}

// reposLines renders the inside of the Repos pane: the folder tab bar in a
// workspace with folders, then the visible window of rows around the cursor
// and "… N more" when rows remain below it, or noMatchingRepos when no row
// is visible.
func (m Model) reposLines(r Rect) []string {
	inner := r.W - 2
	gutter := compactGutter
	rows := m.rowsAt(m.visible())
	now := m.now()

	if m.tableMode() {
		gutter = tableGutter
	}

	var (
		lines []string
		l     columnLayout
	)

	if m.reposTabBarRows(r) > 0 {
		bar := folderTabBar(folderTabs(m.rows), m.folderTab, m.folderTabCounts(), inner-gutter, m.styles)
		lines = append(lines, fit(strings.Repeat(" ", gutter)+bar, inner))
	}

	if len(rows) == 0 {
		indent := gutter + rowPrefixWidth
		text := ansi.Truncate(noMatchingRepos, inner-indent, "…")

		return append(lines, strings.Repeat(" ", indent)+m.styles.Muted.Render(text))
	}

	if m.tableMode() {
		l = newTableLayout(rows, m.folderTab, inner-gutter, now)
		lines = append(lines, strings.Repeat(" ", gutter)+m.styles.Heading.Render(repoTableHeader(l)))
	} else {
		l = newCompactLayout(rows, m.folderTab, inner-gutter, m.styles)
		if m.reposHasHeader(r) {
			lines = append(lines, strings.Repeat(" ", gutter)+repoHeader(l, m.styles))
		}
	}

	start, end := scrollWindow(len(rows), m.cursor, m.top, m.reposCapacity(r))

	for i := start; i < end; i++ {
		rw := rows[i]
		selected, note := m.selected[rw.snap.Loc.ID], m.rowNote(rw)

		var line string
		if m.tableMode() {
			line = repoTableRow(rw.snap, rw.loaded, m.spinnerOf(rw), selected, note, m.folderTab, l, m.styles, now)
		} else {
			line = repoRow(rw.snap, rw.loaded, m.spinnerOf(rw), selected, note, m.folderTab, l, m.styles)
		}

		lines = append(lines, m.paneRow(PaneRepos, i == m.cursor, m.gutter(i == m.cursor)+line, inner))
	}

	if more := len(rows) - end; more > 0 {
		lines = append(lines, strings.Repeat(" ", gutter+rowPrefixWidth)+
			m.styles.Muted.Render("… "+strconv.Itoa(more)+" more"))
	}

	return lines
}

// gutter is the cursor column ahead of a row.
func (m Model) gutter(isCursor bool) string {
	switch {
	case m.tableMode() && isCursor:
		return m.styles.Cursor.Render(tableCursorGutter)
	case m.tableMode():
		return strings.Repeat(" ", tableGutter)
	case isCursor:
		return m.styles.Cursor.Render(cursorMark)
	default:
		return " "
	}
}

// paneRow returns line, a row of a list in pane p with its cursor gutter, as
// the pane draws it width cells wide: the row under the cursor of the
// focused pane as the cursor bar (see cursorBar), any other row as it is.
// While a dialog is open it takes the keys, and only the dialog may show a
// bar.
func (m Model) paneRow(p Pane, isCursor bool, line string, width int) string {
	if !isCursor || m.focus != p || m.dialog != nil {
		return line
	}

	return cursorBar(line, width, m.styles)
}

// status renders the status line: the filter while it is typed, followed by
// the notice of the last key; else that notice, the gone branch under the
// Commits cursor, or the repository under the Repos cursor, after the kept
// filter when there is one.
func (m Model) status() string {
	if m.filtering {
		return typingLine(m.filter, m.notice, m.width, m.styles)
	}

	if m.notice != "" {
		return m.styles.Attention.Render(ansi.Truncate(m.notice, m.width, "…"))
	}

	if g, ok := m.goneUnderCursor(); ok {
		return goneStatusLine(g, m.width, m.styles)
	}

	r, ok := m.cursorRow()

	if m.filter != "" {
		line := filterLine(m.filter, false, m.width, m.styles)

		room, fits := roomAfter(line, m.width)
		if !ok || !fits {
			return line
		}

		return line + separator + statusLine(r.snap, r.loaded, m.statusNote(r), room, m.styles)
	}

	if !ok {
		return ""
	}

	return statusLine(r.snap, r.loaded, m.statusNote(r), m.width, m.styles)
}

// rowsAt returns the rows at the given indexes. The Repos pane sizes its
// columns from every visible row, not only the shown ones, so that scrolling
// does not move them.
func (m Model) rowsAt(indexes []int) []row {
	rows := make([]row, len(indexes))
	for i, idx := range indexes {
		rows[i] = m.rows[idx]
	}

	return rows
}

// loadedSnapshots returns the snapshots of every repository read so far.
func (m Model) loadedSnapshots() []drift.Snapshot {
	snaps := make([]drift.Snapshot, 0, len(m.rows))

	for _, r := range m.rows {
		if r.loaded {
			snaps = append(snaps, r.snap)
		}
	}

	return snaps
}
