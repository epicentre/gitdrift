package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/epicentre/gitdrift/internal/repo"
)

// branchKind is the kind of one row of the Branches tab.
type branchKind int

// Kinds of Branches rows.
const (
	branchHeader branchKind = iota + 1
	branchGone
	branchLocalOnly
	branchStash
)

const (
	headerGone     = "GONE · deleted on remote"
	headerOnlyHere = "ONLY ON THIS MACHINE"

	// branchNameWidth is the name column of the labels, which do not depend
	// on the pane. A name keeps branchNameGap spaces after it and is cut
	// with "…" when longer.
	branchNameWidth = 23
	branchNameGap   = 2
	// branchNameMin is the narrowest a narrow pane cuts the names of the
	// Branches rows.
	branchNameMin = 2
	// ageGap separates the age from what precedes it.
	ageGap = "  "
	// selectBox prefixes a gone branch that can be selected for deletion,
	// selectedBox one that is selected.
	selectBox   = "[ ] "
	selectedBox = "[x] "
	// branchIndent replaces the selection box on the other rows, which
	// keeps all names in one column.
	branchIndent = "    "
)

// Columns of a Branches row after the selection box: the branch name or
// stash ref, the counts or the stash subject, and the age.
const (
	itemName = iota
	itemInfo
	itemAge
)

// branchItem is one row of the Branches tab: a section header, a gone
// branch, a branch with commits only here, or a stash entry.
type branchItem struct {
	kind  branchKind
	label string
	gone  *repo.GoneBranch
	local *repo.LocalOnlyBranch
	stash *repo.Stash
	// ref names the branch or stash entry of the row; empty for headers.
	ref string

	// box, name, info and age are the parts of label: the selection box,
	// the sanitized name, the counts or stash subject, and the age (empty
	// for local-only branches). row lays them out again for the pane.
	box, name, info, age string
}

// selectable reports whether the Commits cursor can rest on the row.
func (it branchItem) selectable() bool { return it.kind != branchHeader }

// onlyHereCount is the number of commits of the row that no remote has.
func (it branchItem) onlyHereCount() int {
	switch {
	case it.gone != nil:
		return it.gone.OnlyHere
	case it.local != nil:
		return it.local.Commits
	case it.stash != nil:
		return 1
	default:
		return 0
	}
}

// branchItems lists the Branches tab of a repository: its gone branches,
// then the branches and stash entries that exist only on this machine. The
// checked-out branch currentBranch gets no selection box: it cannot be
// deleted. Labels use the full name column.
func branchItems(w repo.Work, currentBranch string, now time.Time) []branchItem {
	var items []branchItem

	if len(w.Gone) > 0 {
		items = append(items, branchItem{kind: branchHeader, label: headerGone})
	}

	for _, g := range w.Gone {
		box := selectBox
		if g.Name == currentBranch {
			box = branchIndent
		}

		items = append(items, labeled(branchItem{
			kind: branchGone, gone: &g, ref: g.Name,
			box: box, name: sanitize(g.Name), info: strconv.Itoa(g.OnlyHere) + " only here", age: relAge(g.LastCommit, now),
		}))
	}

	if len(w.LocalOnly)+len(w.Stashes) > 0 {
		items = append(items, branchItem{kind: branchHeader, label: headerOnlyHere})
	}

	for _, b := range w.LocalOnly {
		info := strconv.Itoa(b.Commits) + " not pushed"
		if b.NoUpstream {
			info = countNoun(b.Commits, "commit") + ", no upstream"
		}

		items = append(items, labeled(branchItem{
			kind: branchLocalOnly, local: &b, ref: b.Name, name: sanitize(b.Name), info: info,
		}))
	}

	for _, s := range w.Stashes {
		items = append(items, labeled(branchItem{
			kind: branchStash, stash: &s, ref: s.Ref,
			name: sanitize(s.Ref), info: sanitize(s.Subject), age: relAge(s.When, now),
		}))
	}

	return items
}

// labeled sets the label of it from its parts.
func labeled(it branchItem) branchItem {
	it.label = it.box + nameCell(it.name, branchNameWidth) + it.info + it.ageSuffix()

	return it
}

func (it branchItem) ageSuffix() string {
	if it.age == "" {
		return ""
	}

	return ageGap + it.age
}

// marked returns the row with its selection box checked when selected; rows
// without a box stay as they are.
func (it branchItem) marked(selected bool) branchItem {
	if selected && it.box == selectBox {
		it.box = selectedBox
	}

	return it
}

// lead is what precedes the name when the row is drawn: the selection box of
// a gone branch, or the indent that aligns the other names with it.
func (it branchItem) lead() string {
	if it.kind == branchGone {
		return it.box
	}

	return branchIndent
}

// nameCell is name in a column of width cells, cut with "…" so that
// branchNameGap spaces always follow it.
func nameCell(name string, width int) string {
	if room := max(width-branchNameGap, 0); ansi.StringWidth(name) > room {
		name = ansi.Truncate(name, room, "…")
	}

	return fit(name, width)
}

// newBranchLayout fits the Branches rows of items in rows width cells wide:
// the names in a column as wide as the longest, the counts and stash
// subjects after them, and the ages right-aligned at the right edge. A
// narrow pane cuts the stash subjects first, never below the longest count,
// then the names, so that the counts and ages stay visible.
func newBranchLayout(items []branchItem, width int) columnLayout {
	cols := []column{
		itemName: {min: branchNameMin, shrink: 2},
		itemInfo: {fill: true, shrink: 1, gap: branchNameGap},
		itemAge:  {align: alignRight, gap: len(ageGap)},
	}

	for _, it := range items {
		if it.kind == branchHeader {
			continue
		}

		include(cols, it.cells())

		if it.kind != branchStash {
			cols[itemInfo].min = max(cols[itemInfo].min, ansi.StringWidth(it.info))
		}
	}

	return layoutColumns(cols, width-len(branchIndent))
}

// cells returns the name, info and age cells of the row.
func (it branchItem) cells() []string {
	return []string{itemName: it.name, itemInfo: it.info, itemAge: it.age}
}

// row draws the row laid out by l: the selection box or its indent, then the
// columns. A section header keeps its label.
func (it branchItem) row(l columnLayout, st Styles) string {
	if it.kind == branchHeader {
		return st.Muted.Render(ansi.Truncate(it.label, len(branchIndent)+l.width, "…"))
	}

	return it.lead() + l.render(it.cells())
}

// nearestSelectable returns the selectable row nearest to i, looking in
// direction dir (1 down, -1 up) first. It is false when no row is
// selectable.
func nearestSelectable(items []branchItem, i, dir int) (int, bool) {
	step := 1
	if dir < 0 {
		step = -1
	}

	for _, d := range []int{step, -step} {
		for j := min(max(i, 0), len(items)-1); j >= 0 && j < len(items); j += d {
			if items[j].selectable() {
				return j, true
			}
		}
	}

	return 0, false
}

// goneStatusLine describes the gone branch under the Commits cursor.
func goneStatusLine(g repo.GoneBranch, width int, st Styles) string {
	parts := []string{
		st.Strong.Render(sanitize(g.Name)),
		"deleted on remote",
		countNoun(g.OnlyHere, "commit") + " not on any remote",
	}

	return ansi.Truncate(strings.Join(parts, separator), width, "…")
}

// goneUnderCursor returns the gone branch under the Commits cursor while a
// detail pane has the focus.
func (m Model) goneUnderCursor() (repo.GoneBranch, bool) {
	it, ok := m.cursorBranchItem()
	if m.focus == PaneRepos || !ok || it.kind != branchGone || it.gone == nil {
		return repo.GoneBranch{}, false
	}

	return *it.gone, true
}
