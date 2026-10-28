package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Rounded border runes.
const (
	borderTopLeft     = "╭"
	borderTopRight    = "╮"
	borderBottomLeft  = "╰"
	borderBottomRight = "╯"
	borderHorizontal  = "─"
	borderVertical    = "│"

	// boxTitleChrome is the top edge around the title: corner, rule and a
	// space before it, a space and the closing corner after it.
	boxTitleChrome = 5
)

// box draws a w x h pane with a rounded border, title embedded in the top
// edge ("╭─ 1 Repos ───╮"). lines fill the inside from the top; each is cut
// or padded to the inner width and lines beyond the inner height are
// dropped. The focused pane's border and title use the accent color.
func box(title string, lines []string, w, h int, focused bool, st Styles) string {
	if w <= 0 || h <= 0 {
		return ""
	}

	if w < 2 || h < 2 {
		return strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", w)+"\n", h), "\n")
	}

	borderStyle, titleStyle := st.Border, st.Title
	if focused {
		borderStyle, titleStyle = st.BorderFocused, st.TitleFocused
	}

	inner := w - 2
	side := borderStyle.Render(borderVertical)
	out := make([]string, 0, h)
	out = append(out, topEdge(title, w, borderStyle.Render, titleStyle.Render))

	for i := range h - 2 {
		var line string
		if i < len(lines) {
			line = lines[i]
		}

		out = append(out, side+fit(line, inner)+side)
	}

	out = append(out, borderStyle.Render(borderBottomLeft+strings.Repeat(borderHorizontal, inner)+borderBottomRight))

	return strings.Join(out, "\n")
}

// topEdge renders the top border with the title, dropping or cutting the
// title when the pane is too narrow for it.
func topEdge(title string, w int, border, text func(...string) string) string {
	room := w - boxTitleChrome
	if title == "" || room < 1 {
		return border(borderTopLeft + strings.Repeat(borderHorizontal, w-2) + borderTopRight)
	}

	title = ansi.Truncate(title, room, "…")
	rest := room - ansi.StringWidth(title)

	return border(borderTopLeft+borderHorizontal+" ") + text(title) +
		border(" "+strings.Repeat(borderHorizontal, rest)+borderTopRight)
}

// fit cuts s to width cells and pads it with spaces to exactly width: a wide
// character that does not fit at the edge leaves a blank cell. It keeps ANSI
// styling intact.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}

	w := ansi.StringWidth(s)
	if w > width {
		s = ansi.Truncate(s, width, "")
		w = ansi.StringWidth(s)
	}

	return s + strings.Repeat(" ", width-w)
}
