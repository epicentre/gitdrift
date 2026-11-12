package ui

import (
	"cmp"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// token is a piece of styled text, such as the note a row lays across
// several of its columns.
type token struct {
	text  string
	style lipgloss.Style
}

// align is where a value sits in a column wider than it.
type align int

// Alignments of a column.
const (
	alignLeft align = iota
	alignRight
)

// column describes one column of a list for layoutColumns. All widths are
// display widths in cells.
type column struct {
	// width is the content width: the widest value of the column over every
	// row of the list, not only the rows on screen, so that scrolling never
	// moves a column. A column empty in every row takes no room.
	width int
	// min is the narrowest a shrinkable column gets before columns are
	// dropped.
	min int
	// shrink orders the columns that may be cut with "…" when the row is too
	// narrow: 1 shrinks first; 0 never shrinks.
	shrink int
	// drop orders the columns left out when shrinking is not enough: 1 goes
	// first; 0 always stays.
	drop int
	// gap is the least number of spaces ahead of the column; 0 means one.
	gap int
	// fill makes the column take the width left over, which otherwise
	// widens the gaps between the columns.
	fill  bool
	align align
	// keepEnd cuts a value too wide for the column from the left, so that
	// its end stays visible.
	keepEnd bool
	// cut, when set, shortens a value too wide for the column to at most
	// width cells, in place of the cut with "…" at the end or, with keepEnd,
	// at the start.
	cut func(s string, width int) string
	// style draws the column's values.
	style lipgloss.Style
}

func (c column) minGap() int { return max(c.gap, 1) }

// isEmpty reports whether the column takes no room: nothing to show and
// nothing to fill.
func (c column) isEmpty() bool { return c.width == 0 && !c.fill }

// include widens each column to hold the cell of a row in it.
func include(cols []column, cells []string) {
	for i, c := range cells {
		if i < len(cols) {
			cols[i].width = max(cols[i].width, ansi.StringWidth(c))
		}
	}
}

// columnsWidth is the width of a row with every column at its content width
// and the least gaps.
func columnsWidth(cols []column) int {
	var total int

	first := true

	for _, c := range cols {
		if c.isEmpty() {
			continue
		}

		if !first {
			total += c.minGap()
		}

		total += c.width
		first = false
	}

	return total
}

// columnLayout is a list's columns fitted to a row: which columns are shown,
// the spaces ahead of each, where each starts and how wide it is.
type columnLayout struct {
	cols   []column
	width  int
	shown  []bool
	gaps   []int
	x      []int
	widths []int
	// cellStyle, when restyled is set, draws every cell in place of the
	// styles of the columns (see in).
	cellStyle lipgloss.Style
	restyled  bool
}

// in returns the layout drawing every cell in style instead of its column's
// style; values laid across columns keep their own style.
func (l columnLayout) in(style lipgloss.Style) columnLayout {
	l.cellStyle, l.restyled = style, true

	return l
}

// layoutColumns fits cols in a row of width cells.
//
// When the content widths fit, the width left over goes to the fill column,
// or else is spread over the gaps between the columns, so that the last
// column ends at the row's right edge; with a single column it stays at the
// end of the row.
//
// When they do not fit, the shrinkable columns are cut down to their
// minimums in shrink order, and if that is not enough whole columns are
// dropped in drop order. The columns left get back as much of their content
// width as the row allows. Should even they not fit, the shrinkable ones go
// below their minimums, down to one cell.
func layoutColumns(cols []column, width int) columnLayout {
	n := len(cols)
	l := columnLayout{
		cols:   cols,
		width:  max(width, 0),
		shown:  make([]bool, n),
		gaps:   make([]int, n),
		x:      make([]int, n),
		widths: make([]int, n),
	}

	for i, c := range cols {
		l.shown[i] = !c.isEmpty()
		l.widths[i] = c.width
	}

	for l.used(l.minWidth) > l.width {
		i := l.nextDrop()
		if i < 0 {
			break
		}

		l.shown[i] = false
	}

	over := l.used(l.widthOf) - l.width
	over = l.shrink(over, func(c column) int { return min(c.min, c.width) })
	l.shrink(over, func(column) int { return 1 })
	l.spread(l.width - l.used(l.widthOf))

	var x int

	for i := range cols {
		if l.shown[i] {
			x += l.gaps[i]
			l.x[i] = x
			x += l.widths[i]
		}
	}

	return l
}

// withStyle returns the layout with every column drawn in st, such as for a
// header row.
func (l columnLayout) withStyle(st lipgloss.Style) columnLayout {
	l.cols = slices.Clone(l.cols)
	for i := range l.cols {
		l.cols[i].style = st
	}

	return l
}

func (l columnLayout) widthOf(i int) int { return l.widths[i] }

// minWidth is the width column i keeps before columns are dropped: its
// minimum when it may shrink, else its content width.
func (l columnLayout) minWidth(i int) int {
	if c := l.cols[i]; c.shrink > 0 {
		return min(c.min, c.width)
	}

	return l.cols[i].width
}

// used is the width of the shown columns at the widths w gives, with the
// least gaps between them.
func (l columnLayout) used(w func(i int) int) int {
	var total int

	first := true

	for i, c := range l.cols {
		if !l.shown[i] {
			continue
		}

		if !first {
			total += c.minGap()
		}

		total += w(i)
		first = false
	}

	return total
}

// nextDrop is the shown column to leave out next, or -1 when every shown
// column stays. Of two with the same priority the rightmost goes first.
func (l columnLayout) nextDrop() int {
	next := -1

	for i, c := range l.cols {
		if l.shown[i] && c.drop > 0 && (next < 0 || c.drop <= l.cols[next].drop) {
			next = i
		}
	}

	return next
}

// shrink takes up to over cells from the shown shrinkable columns in shrink
// order, none below the width floor gives it, and returns what it could not
// take.
func (l *columnLayout) shrink(over int, floor func(column) int) int {
	order := make([]int, 0, len(l.cols))

	for i, c := range l.cols {
		if l.shown[i] && c.shrink > 0 {
			order = append(order, i)
		}
	}

	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(l.cols[a].shrink, l.cols[b].shrink) })

	for _, i := range order {
		if over <= 0 {
			break
		}

		cut := min(over, max(l.widths[i]-floor(l.cols[i]), 0))
		l.widths[i] -= cut
		over -= cut
	}

	return over
}

// spread sets the gaps ahead of the shown columns and hands out spare
// cells: all to the first fill column, or else across the gaps, the first
// gaps taking one more when they do not divide evenly.
func (l *columnLayout) spread(spare int) {
	var shown []int

	for i := range l.cols {
		if l.shown[i] {
			shown = append(shown, i)
		}
	}

	for k, i := range shown {
		if k > 0 {
			l.gaps[i] = l.cols[i].minGap()
		}
	}

	if spare <= 0 {
		return
	}

	if fill := slices.IndexFunc(shown, func(i int) bool { return l.cols[i].fill }); fill >= 0 {
		l.widths[shown[fill]] += spare

		return
	}

	if len(shown) < 2 {
		return
	}

	gaps := len(shown) - 1
	for k, i := range shown[1:] {
		l.gaps[i] += spare / gaps
		if k < spare%gaps {
			l.gaps[i]++
		}
	}
}

// render draws one row, exactly as wide as the layout, with cells[i] in
// column i: each value in the column's style, aligned, and cut with "…" when
// wider. Cells of the columns not shown are skipped; missing cells are blank.
func (l columnLayout) render(cells []string) string {
	return l.renderSpan(cells, len(l.cols), len(l.cols), token{}, alignLeft)
}

// renderSpan draws a row like render with value laid across the columns
// [from, to) in place of their cells, aligned and cut with "…" to their span
// (see span).
func (l columnLayout) renderSpan(cells []string, from, to int, value token, a align) string {
	var (
		b  strings.Builder
		at int
	)

	put := func(x, w int, s string) {
		b.WriteString(strings.Repeat(" ", max(x-at, 0)))
		b.WriteString(s)

		at = max(x, at) + w
	}

	for i := range l.cols {
		if i == from {
			start, end := l.span(from, to)
			put(start, end-start, place(value, end-start, a, false))
		}

		if !l.shown[i] || (i >= from && i < to) {
			continue
		}

		c := &l.cols[i]
		cell := token{style: c.style}

		if l.restyled {
			cell.style = l.cellStyle
		}

		if i < len(cells) {
			cell.text = cells[i]
		}

		if c.cut != nil && ansi.StringWidth(cell.text) > l.widths[i] {
			cell.text = c.cut(cell.text, l.widths[i])
		}

		put(l.x[i], l.widths[i], place(cell, l.widths[i], c.align, c.keepEnd))
	}

	// Every piece is exactly as wide as its place, so the row drawn so far is
	// at cells wide; it is cut only when fixed columns overflow it.
	if at > l.width {
		return fit(b.String(), l.width)
	}

	return b.String() + strings.Repeat(" ", l.width-at)
}

// span is the first cell and the end of the columns [from, to): from the
// first shown one to the last, and to the row's right edge when the range
// runs to the last column. Without a shown column the span starts one space
// after the shown columns before it.
func (l columnLayout) span(from, to int) (start, end int) {
	to = min(to, len(l.cols))

	first, last := -1, -1

	for i := from; i < to; i++ {
		if l.shown[i] {
			if first < 0 {
				first = i
			}

			last = i
		}
	}

	if first >= 0 {
		start, end = l.x[first], l.x[last]+l.widths[last]
	} else {
		for i := range from {
			if l.shown[i] {
				start = l.x[i] + l.widths[i] + 1
			}
		}

		end = start
	}

	if to == len(l.cols) {
		end = l.width
	}

	return min(start, end), end
}

// place draws t in exactly width cells: aligned, and cut with "…" when wider,
// from the left when keepEnd is set. A cut that still comes out too wide,
// as where East Asian widths make "…" two cells, is cut again at the end
// without a mark.
func place(t token, width int, a align, keepEnd bool) string {
	if width <= 0 {
		return ""
	}

	text := t.text

	w := ansi.StringWidth(text)
	if w > width {
		if keepEnd {
			text = cutLeft(text, width)
		} else {
			text = ansi.Truncate(text, width, "…")
		}

		if ansi.StringWidth(text) > width {
			text = ansi.Truncate(text, width, "")
		}

		w = ansi.StringWidth(text)
	}

	pad := strings.Repeat(" ", max(width-w, 0))

	if text != "" {
		text = t.style.Render(text)
	}

	if a == alignRight {
		return pad + text
	}

	return text + pad
}

// cutLeft cuts s to at most width cells by dropping its start, marked with
// "…". A wide character that straddles the cut goes too.
func cutLeft(s string, width int) string {
	w := ansi.StringWidth(s)
	if w <= width {
		return s
	}

	if width <= 1 {
		return ansi.Truncate("…", max(width, 0), "")
	}

	tail := ansi.TruncateLeft(s, w-width+1, "")
	if ansi.StringWidth(tail) > width-1 {
		tail = ansi.TruncateLeft(s, w-width+2, "")
	}

	return "…" + tail
}
