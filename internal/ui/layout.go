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
	// leftPercent is the left column's share of a wide screen.
	leftPercent  = 44
	minLeftWidth = 40
	// collapsedHeight shows a pane's borders and one line.
	collapsedHeight = 3
	// reposShrunkHeight is the Repos pane while Commits or Files has focus.
	reposShrunkHeight = 5
	// focusedPercent is the focused left pane's share of the rows left over
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
// takes the first row and the status and footer lines the last two. The left
// column stacks Repos, Commits and Files; the Diff pane fills the right
// unless the screen is narrow. The focused left pane gets most of the
// column; with focus on Diff the column keeps the sizes of lastLeft. With
// zoom the focused pane takes the whole body.
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

	leftW := width
	if width < narrowWidth {
		lay.DiffHidden = true
	} else {
		leftW = max(width*leftPercent/100, minLeftWidth)
		lay.Diff = Rect{X: leftW, Y: body.Y, W: width - leftW, H: body.H}
	}

	sizing := focus
	if sizing != PaneCommits && sizing != PaneFiles && sizing != PaneRepos {
		sizing = lastLeft
	}

	repos, commits, files := leftHeights(body.H, sizing)
	lay.Repos = Rect{X: 0, Y: body.Y, W: leftW, H: repos}
	lay.Commits = Rect{X: 0, Y: body.Y + repos, W: leftW, H: commits}
	lay.Files = Rect{X: 0, Y: body.Y + repos + commits, W: leftW, H: files}

	return lay
}

// leftHeights splits the body rows between the left panes for the given
// focused left pane; anything other than Commits or Files sizes for Repos.
func leftHeights(body int, focus Pane) (repos, commits, files int) {
	if focus != PaneCommits && focus != PaneFiles {
		return body - 2*collapsedHeight, collapsedHeight, collapsedHeight
	}

	rest := body - reposShrunkHeight
	focused := rest * focusedPercent / 100

	if focus == PaneCommits {
		return reposShrunkHeight, focused, rest - focused
	}

	return reposShrunkHeight, rest - focused, focused
}
