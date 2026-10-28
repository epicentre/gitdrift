package ui

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/ops"
	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	// dialogMargin is the room a window leaves free on each side of the
	// screen.
	dialogMargin = 2
	// dialogChrome is the border and the one-cell padding on each side of a
	// window's lines.
	dialogChrome = 4
	// dialogFixedRows are the borders, the blank line and the footer.
	dialogFixedRows = 4
	// scrollHint ends the footer of a window whose body does not fit.
	scrollHint = " · ↑↓ scroll"

	footerPullDialog   = "y pull · n cancel"
	footerDeleteDialog = "y delete · n cancel"
	footerClose        = "press any key to close"

	// maxDeleteCommits is how many commits of a branch the delete dialog
	// lists before "+N more".
	maxDeleteCommits = 5
	// commitIndent starts the commit lines of the delete dialog.
	commitIndent = "  "
)

// dialog is a window drawn over the dashboard. While one is open it takes
// every key.
type dialog interface {
	// view lays the window out with body lines of at most width cells. A
	// window that fills its body itself, such as a list with its own
	// window, keeps to height lines.
	view(env dialogEnv, width, height int) dialogView
}

// dialogEnv is what a dialog draws with.
type dialogEnv struct {
	st   Styles
	keys keyMap
	now  time.Time
}

// dialogView is the content of a window.
type dialogView struct {
	title  string
	body   []string
	footer string
	// width is the least inner width, which keeps a window from changing
	// size while its body changes.
	width int
}

// innerWidth is the inner width the window asks for: its widest line, or the
// least width it keeps.
func (v dialogView) innerWidth() int {
	// The title sits in the top border, which needs one more cell.
	inner := max(ansi.StringWidth(v.title)+1, ansi.StringWidth(v.footer), v.width)
	for _, l := range v.body {
		inner = max(inner, ansi.StringWidth(l))
	}

	return inner
}

// dialogFrame is the open dialog laid out for the screen: its body wrapped at
// the window's inner width, and the rows of it that fit from top.
type dialogFrame struct {
	title, footer string
	lines         []string
	inner         int
	rows, top     int
}

// maxTop is the scroll offset that shows the last body line.
func (f dialogFrame) maxTop() int { return max(len(f.lines)-f.rows, 0) }

// frame lays the open dialog out for the screen. It is false when no dialog
// is open or the screen has no room for one.
func (m Model) frame() (dialogFrame, bool) {
	maxWidth := m.width - 2*dialogMargin - dialogChrome
	maxRows := m.height - chromeRows - dialogFixedRows

	if m.dialog == nil || maxWidth < 1 || maxRows < 1 {
		return dialogFrame{}, false
	}

	v := m.dialog.view(dialogEnv{st: m.styles, keys: m.keys, now: m.now()}, maxWidth, maxRows)
	inner := min(v.innerWidth(), maxWidth)

	f := dialogFrame{title: v.title, footer: v.footer, lines: wrapLines(v.body, inner), inner: inner}
	f.rows = min(len(f.lines), maxRows)
	f.top = min(max(m.dialogTop, 0), f.maxTop())

	if f.maxTop() > 0 {
		f.footer += scrollHint
	}

	return f, true
}

// wrapLines breaks every line wider than width, so that nothing a window
// shows is cut off.
func wrapLines(lines []string, width int) []string {
	out := make([]string, 0, len(lines))

	for _, l := range lines {
		if ansi.StringWidth(l) <= width {
			out = append(out, l)

			continue
		}

		out = append(out, strings.Split(ansi.Wrap(l, width, ""), "\n")...)
	}

	return out
}

// draw renders the window: the shown body lines, "… N more" in place of the
// last one when more follow, then a blank line and the footer.
func (f dialogFrame) draw(st Styles) string {
	shown := slices.Clone(f.lines[f.top : f.top+f.rows])
	if more := len(f.lines) - f.top - f.rows; more > 0 && len(shown) > 0 {
		shown[len(shown)-1] = st.Muted.Render("… " + strconv.Itoa(more+1) + " more")
	}

	inside := make([]string, 0, len(shown)+2)
	for _, l := range shown {
		inside = append(inside, " "+l)
	}

	inside = append(inside, "", " "+st.Muted.Render(f.footer))

	return box(f.title, inside, f.inner+dialogChrome, len(inside)+2, true, st)
}

// overlayDialog draws the open dialog centered over the body of screen.
func (m Model) overlayDialog(screen string) string {
	f, ok := m.frame()
	if !ok {
		return screen
	}

	win := f.draw(m.styles)
	w := f.inner + dialogChrome
	h := strings.Count(win, "\n") + 1

	return overlay(screen, win, max((m.width-w)/2, 0), 1+max((m.height-chromeRows-h)/2, 0))
}

// overlay draws top over base with its top left corner at column x, row y.
// The cells of base left and right of top keep their styling.
func overlay(base, top string, x, y int) string {
	lines := strings.Split(base, "\n")

	for i, line := range strings.Split(top, "\n") {
		row := y + i
		if row >= len(lines) {
			break
		}

		bg := lines[row]
		w := ansi.StringWidth(line)
		joined := fit(bg, x) + ansi.ResetStyle + line + ansi.ResetStyle + ansi.TruncateLeft(bg, x+w, "")
		lines[row] = fit(joined, max(ansi.StringWidth(bg), x+w))
	}

	return strings.Join(lines, "\n")
}

// openDialog shows d from its first line.
func (m *Model) openDialog(d dialog) {
	m.dialog, m.dialogTop = d, 0
}

// closeDialog closes the open dialog and shows the next window that arrived
// meanwhile, if any.
func (m *Model) closeDialog() {
	m.dialog = nil
	m.openWaiting()
}

// openWaiting opens the first window that waits, when no window is open.
func (m *Model) openWaiting() {
	if m.dialog == nil && len(m.waiting) > 0 {
		m.openDialog(m.waiting[0])
		m.waiting = m.waiting[1:]
	}
}

// dialogShown reports whether the open dialog is drawn: the screen is large
// enough for the dashboard and for the window.
func (m Model) dialogShown() bool {
	if m.layout().TooSmall {
		return false
	}

	_, ok := m.frame()

	return ok
}

// handleDialogKey gives a key press to the open dialog. A window whose body
// does not fit scrolls with the navigation keys.
func (m *Model) handleDialogKey(msg tea.KeyPressMsg) tea.Cmd {
	if !m.dialogShown() {
		m.handleHiddenDialogKey(msg)

		return nil
	}

	if p, ok := m.dialog.(basePicker); ok {
		return m.handlePickerKey(p, msg)
	}

	if f, ok := m.frame(); ok && f.maxTop() > 0 {
		if top, ok := m.keys.navigate(msg, f.top, f.maxTop(), max(f.rows-1, 1)); ok {
			m.dialogTop = top

			return nil
		}
	}

	switch d := m.dialog.(type) {
	case pullConfirm:
		return m.answer(msg, func() tea.Cmd { return m.confirmPull(d) })
	case deleteConfirm:
		return m.answer(msg, func() tea.Cmd { return m.confirmDelete(d) })
	case deleteResult:
		m.closeDialog()

		return m.requestRefresh(d.loc.CommonDir, false)
	default:
		m.closeDialog()
	}

	return nil
}

// handleHiddenDialogKey takes a key press while the open dialog cannot be
// drawn: what the user cannot see is never confirmed. A dialog that writes
// may only be cancelled, and a delete result stays open until its restore
// commands can be read.
func (m *Model) handleHiddenDialogKey(msg tea.KeyPressMsg) {
	switch m.dialog.(type) {
	case deleteResult:
		// Kept until the screen is large enough again.
	case helpOverlay:
		m.closeDialog()
	case basePicker:
		if key.Matches(msg, m.keys.Back) {
			m.closeDialog()
		}
	default:
		if key.Matches(msg, m.keys.Cancel) {
			m.closeDialog()
		}
	}
}

// answer runs confirm on y and closes the dialog on n or esc; other keys do
// nothing.
func (m *Model) answer(msg tea.KeyPressMsg, confirm func() tea.Cmd) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.Confirm):
		return confirm()
	case key.Matches(msg, m.keys.Cancel):
		m.closeDialog()
	}

	return nil
}

// pullConfirm asks before pulling the repositories of snaps, as they were
// shown when the user asked.
type pullConfirm struct {
	snaps []drift.Snapshot
}

// Columns of a pull dialog line: repository path, branch, the commits it is
// behind, the arrow and the upstream tip the pull goes to.
const (
	pullName = iota
	pullBranch
	pullBehind
	pullArrow
	pullTip
)

const (
	// pullGap separates the name, the branch and the behind count.
	pullGap = 2
	// pullNameMin and pullBranchMin are the visible widths a narrow window
	// cuts names and branches down to before going below them.
	pullNameMin   = 13
	pullBranchMin = 11
)

// view lists each repository as aligned columns: its folder and name (see
// repoLabel) and branch as wide as the longest, cut with "…" when the window
// is narrow, branch first; the name is cut from the left, so repositories in
// one folder stay apart. Then ↓ with the behind count right-aligned after
// it, the arrow and the upstream tip.
func (d pullConfirm) view(_ dialogEnv, width, _ int) dialogView {
	cols := []column{
		pullName:   {min: pullNameMin, shrink: 2, keepEnd: true},
		pullBranch: {min: pullBranchMin, shrink: 1, gap: pullGap},
		pullBehind: {gap: pullGap},
		pullArrow:  {},
		pullTip:    {},
	}

	var digits int
	for _, s := range d.snaps {
		digits = max(digits, len(strconv.Itoa(s.Status.Behind)))
	}

	lines := make([][]string, len(d.snaps))

	for i, s := range d.snaps {
		behind := strconv.Itoa(s.Status.Behind)

		lines[i] = []string{
			pullName:   repoLabel(s.Loc),
			pullBranch: branchLabel(s.Status),
			pullBehind: "↓" + strings.Repeat(" ", digits-len(behind)) + behind,
			pullArrow:  "→",
			pullTip:    shortSHA(s.Status.UpstreamSHA),
		}

		include(cols, lines[i])
	}

	title := "Pull " + strconv.Itoa(len(d.snaps)) + " repositories?"
	if len(d.snaps) == 1 {
		title = "Pull 1 repository?"
	}

	v := dialogView{title: title, footer: footerPullDialog}
	l := layoutColumns(cols, min(max(v.innerWidth(), columnsWidth(cols)), width))

	v.body = make([]string, len(lines))
	for i, cells := range lines {
		v.body[i] = l.render(cells)
	}

	return v
}

// deleteItem is one gone branch offered for deletion with the commits that
// only this machine has.
type deleteItem struct {
	gone    repo.GoneBranch
	commits []repo.Commit
}

// lost is the number of commits only this machine has. It is never below
// the count of the snapshot, so a listing cut at repo.MaxCommits does not
// understate it.
func (it deleteItem) lost() int { return max(len(it.commits), it.gone.OnlyHere) }

// deleteConfirm asks before deleting gone branches of one repository.
type deleteConfirm struct {
	loc   repo.Location
	items []deleteItem
}

// view lists each branch with up to maxDeleteCommits of its commits. The
// commit lines are aligned columns that span the window: as wide as its
// other lines, or as their columns need, up to width.
func (d deleteConfirm) view(env dialogEnv, width, _ int) dialogView {
	var (
		body    []string
		commits []repo.Commit
		// at holds the body line of each commit.
		at []int
	)

	for i, it := range d.items {
		if i > 0 {
			body = append(body, "")
		}

		body = append(body, deleteHeading(it))

		shown := it.commits[:min(len(it.commits), maxDeleteCommits)]
		for _, c := range shown {
			at = append(at, len(body))
			commits = append(commits, c)
			body = append(body, "")
		}

		if more := it.lost() - len(shown); more > 0 {
			body = append(body, commitIndent+"+"+strconv.Itoa(more)+" more")
		}
	}

	title := "Delete " + strconv.Itoa(len(d.items)) + " gone branches in " + repoLabel(d.loc) + "?"
	if len(d.items) == 1 {
		title = "Delete 1 gone branch in " + repoLabel(d.loc) + "?"
	}

	v := dialogView{title: title, body: body, footer: footerDeleteDialog}

	cols := commitColumnsOf(commits, env.now)
	inner := min(max(v.innerWidth(), len(commitIndent)+columnsWidth(cols)), width)
	l := layoutColumns(cols, inner-len(commitIndent))

	for k, i := range at {
		v.body[i] = commitIndent + l.render(commitCells(commits[k], env.now))
	}

	return v
}

// deleteHeading names a branch, its tip and how many commits it alone has;
// a colon introduces the commits when there are any.
func deleteHeading(it deleteItem) string {
	line := sanitize(it.gone.Name) + " (" + shortSHA(it.gone.SHA) + "), " +
		countNoun(it.lost(), "commit") + " only on this machine"
	if it.lost() > 0 {
		line += ":"
	}

	return line
}

// deleteResult lists what happened to each branch of a confirmed deletion.
// It stays open until a key is pressed.
type deleteResult struct {
	loc      repo.Location
	outcomes []ops.DeleteOutcome
}

func (d deleteResult) view(env dialogEnv, _, _ int) dialogView {
	body := make([]string, len(d.outcomes))

	for i, o := range d.outcomes {
		line := deleteResultLine(o)

		switch {
		case o.Deleted:
			body[i] = line
		case o.Err != nil:
			body[i] = env.st.Attention.Render(line)
		default:
			body[i] = env.st.Behind.Render(line)
		}
	}

	return dialogView{title: "Gone branches in " + repoLabel(d.loc), body: body, footer: footerClose}
}

// deleteResultLine describes the outcome of one deletion; a deleted branch
// comes with the command that restores it. The command is shown as it is:
// ops.RestoreCommand writes any name a terminal would hide or act on as
// octal escapes, and sanitizing it would restore a different name.
func deleteResultLine(o ops.DeleteOutcome) string {
	name := sanitize(o.Branch)

	switch {
	case o.Deleted:
		return "deleted " + name + " (was " + shortSHA(o.SHA) + ")" + separator + "restore: " + ops.RestoreCommand(o)
	case o.Err != nil:
		return "failed " + name + ": " + gitMessage(o.Err)
	default:
		return "skipped " + name + ": " + sanitize(o.Skipped)
	}
}
