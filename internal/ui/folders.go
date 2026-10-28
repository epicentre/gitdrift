package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	// titleReposKey is the pane key ahead of the folder tabs in the Repos
	// pane title.
	titleReposKey = "1"
	// allTabName names the folder tab that shows every repository; its key
	// is "".
	allTabName = "All"
	// hiddenTabsMark stands for the folder tabs a narrow title leaves out
	// before the active one.
	hiddenTabsMark = "…"
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

// reposTabTitle renders the Repos pane title of a workspace with folders in
// at most width cells: the pane key, then All and tabs, each with its count,
// the active one in brackets. counts holds the visible and the total
// repositories of each tab by its key, "" for All; a tab with repositories
// hidden shows visible/total. When that is too wide the inactive tabs lose
// their counts; when it still is, the title is cut from the right with "…",
// at least a space after the active tab, and the tabs before the active one
// give way from the left to a "…", so that the active tab stays whole.
func reposTabTitle(tabs []string, active string, counts map[string][2]int, width int) string {
	keys := append([]string{""}, tabs...)
	act := max(slices.Index(keys, active), 0)

	if title := tabsTitle(folderTabLabels(keys, act, counts, false)); ansi.StringWidth(title) <= width {
		return title
	}

	labels := folderTabLabels(keys, act, counts, true)
	after := len(labels) - act - 1

	// A cut after the active tab needs a space and "…".
	cutRoom := 0
	if after > 0 {
		cutRoom = 2
	}

	for _, shown := range tabWindows(labels, act) {
		if head := shown[:len(shown)-after]; ansi.StringWidth(tabsTitle(head))+cutRoom <= width {
			return ansi.Truncate(tabsTitle(shown), width, "…")
		}
	}

	return ansi.Truncate(tabsTitle(labels[act:act+1]), width, "…")
}

// folderTabLabels returns the label of each tab of keys: its name and count,
// the active tab act in brackets; with short, the inactive tabs go without
// their counts.
func folderTabLabels(keys []string, act int, counts map[string][2]int, short bool) []string {
	labels := make([]string, len(keys))

	for i, k := range keys {
		name := allTabName
		if k != "" {
			name = sanitize(k)
		}

		switch {
		case i == act:
			labels[i] = "[" + name + " " + folderTabCount(counts[k]) + "]"
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

// tabWindows returns the runs of labels a narrow title may show, each ending
// with the tabs from act on, widest first: every tab, then the tabs before
// act left out one by one from the left behind hiddenTabsMark, then the tabs
// from act on alone.
func tabWindows(labels []string, act int) [][]string {
	windows := [][]string{labels}

	for first := 1; first <= act; first++ {
		windows = append(windows, append([]string{hiddenTabsMark}, labels[first:]...))
	}

	if act > 0 {
		windows = append(windows, labels[act:])
	}

	return windows
}

// tabsTitle joins the pane key and the tab labels.
func tabsTitle(labels []string) string {
	return titleReposKey + tabGap + strings.Join(labels, tabGap)
}

// reposTitle is the Repos pane title in a pane w cells wide: the folder tabs
// in a workspace with folders, compact or zoomed, else the plain title.
func (m Model) reposTitle(w int) string {
	tabs := folderTabs(m.rows)

	switch {
	case tabs != nil:
		return reposTabTitle(tabs, m.folderTab, m.folderTabCounts(), w-boxTitleChrome)
	case m.tableMode():
		return titleReposZoomed
	default:
		return titleRepos
	}
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
