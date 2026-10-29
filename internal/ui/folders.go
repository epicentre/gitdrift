package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	// allTabName names the folder tab that shows every repository; its key
	// is "".
	allTabName = "All"
	// hiddenTabsMark stands for the folder tabs a narrow tab bar leaves out
	// before the active one; hiddenTab is its place in a tab window.
	hiddenTabsMark = "…"
	hiddenTab      = -1
)

// topFolder is the top-level folder of folder, a repository's
// slash-separated folder below the workspace: its first name, "" at the
// root.
func topFolder(folder string) string {
	top, _, _ := strings.Cut(folder, "/")

	return top
}

// repoLabel names the repository at loc where it is named alone, in a
// dialog, a notice or the status line, sanitized: folder/name below the
// workspace, so that same-named repositories in different folders differ,
// and the name alone directly in the workspace.
func repoLabel(loc repo.Location) string {
	if loc.Folder == "" {
		return sanitize(loc.Name)
	}

	return sanitize(loc.Folder + "/" + loc.Name)
}

// folderTabs returns the folder tabs of rows that follow All: the distinct
// top-level folders, sorted by name; nil when every repository is at the
// root. A repository at the root shows only in All.
func folderTabs(rows []row) []string {
	var tabs []string

	for _, r := range rows {
		if top := topFolder(r.snap.Loc.Folder); top != "" {
			tabs = append(tabs, top)
		}
	}

	slices.Sort(tabs)

	return slices.Compact(tabs)
}

// folderTabBar renders the tab bar of a workspace with folders, the first
// line inside the Repos pane, in at most width cells: All and the tabs, each
// with its count, the active one padded with a space on each side and drawn
// as a filled box, the others muted. counts holds the visible and the total
// repositories of each tab by its key, "" for All; a tab with repositories
// hidden shows visible/total. When that is too wide the inactive tabs lose
// their counts; when it still is, the bar is cut from the right with "…", at
// least a space after the active tab, and the tabs before the active one
// give way from the left to a "…", so that the active tab stays whole.
func folderTabBar(tabs []string, active string, counts map[string][2]int, width int, st Styles) string {
	keys := append([]string{""}, tabs...)
	act := max(slices.Index(keys, active), 0)

	labels := folderTabLabels(keys, act, counts, false)
	if bar := joinFolderTabs(labels, tabRange(0, len(labels)), act, st); ansi.StringWidth(bar) <= width {
		return bar
	}

	labels = folderTabLabels(keys, act, counts, true)
	after := len(labels) - act - 1

	// A cut after the active tab needs a space and "…".
	cutRoom := 0
	if after > 0 {
		cutRoom = 2
	}

	for _, shown := range tabWindows(len(labels), act) {
		if head := shown[:len(shown)-after]; ansi.StringWidth(joinFolderTabs(labels, head, act, st))+cutRoom <= width {
			return ansi.Truncate(joinFolderTabs(labels, shown, act, st), width, "…")
		}
	}

	return ansi.Truncate(joinFolderTabs(labels, []int{act}, act, st), width, "…")
}

// folderTabLabels returns the label of each tab of keys: its name and count,
// the active tab act padded with a space on each side; with short, the
// inactive tabs go without their counts.
func folderTabLabels(keys []string, act int, counts map[string][2]int, short bool) []string {
	labels := make([]string, len(keys))

	for i, k := range keys {
		name := allTabName
		if k != "" {
			name = sanitize(k)
		}

		switch {
		case i == act:
			labels[i] = " " + name + " " + folderTabCount(counts[k]) + " "
		case short:
			labels[i] = name
		default:
			labels[i] = name + " " + folderTabCount(counts[k])
		}
	}

	return labels
}

// folderTabCount is the count of a tab with c, its visible and total
// repositories: the total, or visible/total when some are hidden.
func folderTabCount(c [2]int) string {
	if c[0] == c[1] {
		return strconv.Itoa(c[1])
	}

	return strconv.Itoa(c[0]) + "/" + strconv.Itoa(c[1])
}

// tabWindows returns the runs of tabs a narrow tab bar may show, as indexes
// into the n labels, each run ending with the tabs from act on, widest
// first: every tab, then the tabs before act left out one by one from the
// left behind hiddenTab, then the tabs from act on alone.
func tabWindows(n, act int) [][]int {
	windows := [][]int{tabRange(0, n)}

	for first := 1; first <= act; first++ {
		windows = append(windows, append([]int{hiddenTab}, tabRange(first, n)...))
	}

	if act > 0 {
		windows = append(windows, tabRange(act, n))
	}

	return windows
}

// tabRange is the indexes from to to, to excluded.
func tabRange(from, to int) []int {
	out := make([]int, 0, max(to-from, 0))
	for i := from; i < to; i++ {
		out = append(out, i)
	}

	return out
}

// joinFolderTabs draws the labels at shown, the active one act as a filled box
// and the others, with hiddenTab as hiddenTabsMark, muted.
func joinFolderTabs(labels []string, shown []int, act int, st Styles) string {
	parts := make([]string, len(shown))

	for i, at := range shown {
		switch at {
		case act:
			parts[i] = st.TabActive.Render(labels[at])
		case hiddenTab:
			parts[i] = st.Muted.Render(hiddenTabsMark)
		default:
			parts[i] = st.Muted.Render(labels[at])
		}
	}

	return strings.Join(parts, tabGap)
}

// reposTitle is the Repos pane title: its name, the number of repositories
// it shows, and whether it is zoomed.
func (m Model) reposTitle() string {
	title := titleRepos + " (" + strconv.Itoa(len(m.visible())) + ")"
	if m.tableMode() {
		title += separator + zoomedMark
	}

	return title
}

// folderTabCounts counts the repositories of each folder tab by its key, ""
// for All: how many the filters leave visible and how many there are.
func (m Model) folderTabCounts() map[string][2]int {
	counts := make(map[string][2]int)
	add := func(tab string, shown bool) {
		c := counts[tab]
		c[1]++

		if shown {
			c[0]++
		}

		counts[tab] = c
	}

	for _, r := range m.rows {
		shown := m.matchesFilters(r)
		add("", shown)

		if top := topFolder(r.snap.Loc.Folder); top != "" {
			add(top, shown)
		}
	}

	return counts
}

// inFolderTab reports whether the active folder tab shows r.
func (m Model) inFolderTab(r row) bool {
	return m.folderTab == "" || topFolder(r.snap.Loc.Folder) == m.folderTab
}

// switchFolderTab moves to the folder tab after the active one, or with back
// before it, wrapping around with All first. The cursor stays on its
// repository when the new tab shows it (see visibleChanged), and otherwise
// goes to the first row.
func (m *Model) switchFolderTab(tabs []string, back bool) {
	keys := append([]string{""}, tabs...)

	step := 1
	if back {
		step = len(keys) - 1
	}

	id, ok := m.cursorID()
	m.folderTab = keys[(max(slices.Index(keys, m.folderTab), 0)+step)%len(keys)]
	m.cursor = 0
	m.visibleChanged(id, ok)
}
