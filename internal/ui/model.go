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
	// follow titleCommits.
	titleRepos       = "1 Repos"
	titleReposZoomed = "1 Repos (zoomed)"
	titleCommits     = "2 "
	titleFiles       = "3 Files"
	titleOnlyHere    = "3 Only here"
	titleDiff        = "4 Diff"

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
// repository starts as a "fetching…" row until its first refresh, which Init
// starts, returns.
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

	for _, g := range m.groups {
		m.refreshing[g.CommonDir] = true

		for _, loc := range g.Members {
			m.rows = append(m.rows, row{snap: drift.Snapshot{Loc: loc}})
		}
	}

	sortRows(m.rows)

	return m
}

// Init asks for the terminal background and refreshes every group with a
// fetch.
func (m Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.groups)+1)
	cmds = append(cmds, tea.RequestBackgroundColor)

	for _, g := range m.groups {
		cmds = append(cmds, refreshCmd(m.ctx, m.backend, g.CommonDir, true))
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
		headerLine(tildePath(m.root, m.home), m.loadedSnapshots(), len(m.rows), m.width, m.styles),
		m.body(lay),
		m.status(),
		footerLine(m.focus, m.zoom, m.detail.tab, m.width, m.styles),
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

func (m Model) body(lay Layout) string {
	var left []string

	for _, p := range []Pane{PaneRepos, PaneCommits, PaneFiles} {
		if r := lay.rect(p); r.W > 0 && r.H > 0 {
			left = append(left, m.pane(p, *r))
		}
	}

	if lay.Diff.W <= 0 || lay.Diff.H <= 0 {
		return lipgloss.JoinVertical(lipgloss.Left, left...)
	}

	diff := m.pane(PaneDiff, lay.Diff)
	if len(left) == 0 {
		return diff
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.JoinVertical(lipgloss.Left, left...), diff)
}

// pane draws pane p in r.
func (m Model) pane(p Pane, r Rect) string {
	focused := p == m.focus

	switch p {
	case PaneRepos:
		title := titleRepos
		if m.tableMode() {
			title = titleReposZoomed
		}

		return box(title, m.reposLines(r), r.W, r.H, focused, m.styles)
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
	capacity := innerHeight(r)
	if m.reposHasHeader(r) {
		capacity-- // column titles
	}

	return max(capacity, 0)
}

// reposHasHeader reports whether the Repos pane in r starts with column
// titles: the zoom table always does, the compact rows unless the pane is
// collapsed.
func (m Model) reposHasHeader(r Rect) bool {
	return m.tableMode() || innerHeight(r) >= compactHeaderRows
}

// reposLines renders the inside of the Repos pane: the visible window of
// rows around the cursor and "… N more" when rows remain below it.
func (m Model) reposLines(r Rect) []string {
	inner := r.W - 2
	gutter := compactGutter
	rows := m.rowsAt(m.visible())
	now := m.now()

	var (
		lines []string
		l     columnLayout
	)

	if m.tableMode() {
		gutter = tableGutter
		l = newTableLayout(rows, inner-gutter, now)
		lines = append(lines, strings.Repeat(" ", gutter)+m.styles.Muted.Render(repoTableHeader(l)))
	} else {
		l = newCompactLayout(rows, inner-gutter, m.styles)
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
			line = repoTableRow(rw.snap, rw.loaded, selected, note, l, m.styles, now)
		} else {
			line = repoRow(rw.snap, rw.loaded, selected, note, l, m.styles)
		}

		lines = append(lines, m.gutter(i == m.cursor)+line)
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

// status renders the status line: the notice of the last key, the gone
// branch under the Commits cursor, or else the repository under the Repos
// cursor.
func (m Model) status() string {
	if m.notice != "" {
		return m.styles.Attention.Render(ansi.Truncate(m.notice, m.width, "…"))
	}

	if g, ok := m.goneUnderCursor(); ok {
		return goneStatusLine(g, m.width, m.styles)
	}

	r, ok := m.cursorRow()
	if !ok {
		return ""
	}

	return statusLine(r.snap, r.loaded, m.rowNote(r), m.width, m.styles)
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
