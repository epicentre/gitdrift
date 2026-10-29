package ui

// Pane is one of the four boxes of the screen.
type Pane int

// Panes, in focus order.
const (
	PaneRepos Pane = iota + 1
	PaneCommits
	PaneFiles
	PaneDiff
)

const (
	paneCount = 4

	// minWidth and minHeight are the smallest terminal the layout supports.
	minWidth  = 60
	minHeight = 15
	// narrowWidth is the width below which the Diff pane is hidden.
	narrowWidth = 90

	// chromeRows are the header, status and footer lines.
	chromeRows = 3
	// leftPercent is the share of a wide screen that Commits and Files take
	// below Repos; Diff takes the rest.
	leftPercent  = 44
	minLeftWidth = 40
	// collapsedHeight shows a pane's borders and one line.
	collapsedHeight = 3
	// bottomPercent is the bottom row's share of the body while Repos has
	// focus.
	bottomPercent = 40
	// reposShrunkHeight is the Repos pane while Commits, Files or Diff has
	// focus.
	reposShrunkHeight = 8
	// minBottomHeight keeps both bottom panes collapsed or larger, and the
	// focused one a line larger, when the screen is short.
	minBottomHeight = 2*collapsedHeight + 1
	// focusedPercent is the focused bottom pane's share of the rows left over
	// by the shrunk Repos pane.
	focusedPercent = 60
)

// next returns the pane after p in focus order, wrapping around.
func (p Pane) next() Pane { return p%paneCount + 1 }

// prev returns the pane before p in focus order, wrapping around.
func (p Pane) prev() Pane { return (p+paneCount-2)%paneCount + 1 }

// Rect is a screen area in cells.
type Rect struct {
	X, Y, W, H int
}

// Layout places the panes. A zero Rect is a pane that is not shown.
type Layout struct {
	Repos, Commits, Files, Diff Rect
	// DiffHidden is set when the screen is too narrow for the Diff pane.
	DiffHidden bool
	// TooSmall is set when the screen is below the minimum size; no pane is
	// placed.
	TooSmall bool
}

// rect returns the area of pane p, or nil for an unknown pane.
func (l *Layout) rect(p Pane) *Rect {
	switch p {
	case PaneRepos:
		return &l.Repos
	case PaneCommits:
		return &l.Commits
	case PaneFiles:
		return &l.Files
	case PaneDiff:
		return &l.Diff
	default:
		return nil
	}
}

// ComputeLayout places the panes on a width x height screen. The header
// takes the first row and the status and footer lines the last two. Repos
// spans the top; below it Commits is stacked on Files at the left and the
// Diff pane fills the right unless the screen is narrow. Repos with focus
// leaves the bottom row bottomPercent of the body; Commits or Files with
// focus shrinks Repos and gets most of the bottom row. With focus on Diff
// the panes keep the sizes of lastLeft, sized as for Commits when that is
// Repos. With zoom the focused pane takes the whole body.
func ComputeLayout(width, height int, focus, lastLeft Pane, zoom bool) Layout {
	var lay Layout

	if width < minWidth || height < minHeight {
		lay.TooSmall = true

		return lay
	}

	body := Rect{X: 0, Y: 1, W: width, H: height - chromeRows}

	if zoom {
		if r := lay.rect(focus); r != nil {
			*r = body
		}

		return lay
	}

	sizing := focus
	if sizing == PaneDiff {
		sizing = lastLeft
		if sizing == PaneRepos {
			sizing = PaneCommits
		}
	}

	repos, commits, files := paneHeights(body.H, sizing)
	lay.Repos = Rect{X: 0, Y: body.Y, W: width, H: repos}
	bottomY := body.Y + repos

	leftW := width
	if width < narrowWidth {
		lay.DiffHidden = true
	} else {
		leftW = max(width*leftPercent/100, minLeftWidth)
		lay.Diff = Rect{X: leftW, Y: bottomY, W: width - leftW, H: commits + files}
	}

	lay.Commits = Rect{X: 0, Y: bottomY, W: leftW, H: commits}
	lay.Files = Rect{X: 0, Y: bottomY + commits, W: leftW, H: files}

	return lay
}

// paneHeights splits the body rows between Repos and the bottom row, whose
// rows Commits and Files share, for the given focused pane; anything other
// than Commits or Files sizes for Repos.
func paneHeights(body int, focus Pane) (repos, commits, files int) {
	if focus != PaneCommits && focus != PaneFiles {
		bottom := max(body*bottomPercent/100, 2*collapsedHeight)

		return body - bottom, bottom / 2, bottom - bottom/2
	}

	rest := max(body-reposShrunkHeight, minBottomHeight)
	focused := rest * focusedPercent / 100

	if focus == PaneCommits {
		return body - rest, focused, rest - focused
	}

	return body - rest, rest - focused, focused
}
