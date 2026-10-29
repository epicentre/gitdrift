package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// borderRunes are the runes of a pane border.
type borderRunes struct {
	topLeft, topRight, bottomLeft, bottomRight, horizontal, vertical string
}

var (
	// thinBorder draws an unfocused pane, thickBorder the focused one.
	thinBorder  = borderRunes{"╭", "╮", "╰", "╯", "─", "│"}
	thickBorder = borderRunes{"┏", "┓", "┗", "┛", "━", "┃"}
)

const (
	// boxTitleChrome is the top edge around the title: corner, rule and a
	// space before it, a space and the closing corner after it.
	boxTitleChrome = 5
)

// box draws a w x h pane with its title embedded in the top edge: a thin
// rounded border ("╭─ 2 Commits ───╮"), or a thick one in the accent color
// for the focused pane ("┏━ 1 Repos (7) ━━━┓"). lines fill the inside from
// the top; each is cut or padded to the inner width and lines beyond the
// inner height are dropped.
func box(title string, lines []string, w, h int, focused bool, st Styles) string {
	if w <= 0 || h <= 0 {
		return ""
	}

	if w < 2 || h < 2 {
		return strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", w)+"\n", h), "\n")
	}

	runes, borderStyle, titleStyle := thinBorder, st.Border, st.Title
	if focused {
		runes, borderStyle, titleStyle = thickBorder, st.BorderFocused, st.TitleFocused
	}

	inner := w - 2
	side := borderStyle.Render(runes.vertical)
	out := make([]string, 0, h)
	out = append(out, topEdge(title, w, runes, borderStyle.Render, titleStyle.Render))

	for i := range h - 2 {
		var line string
		if i < len(lines) {
			line = lines[i]
		}

		out = append(out, side+fit(line, inner)+side)
	}

	out = append(out, borderStyle.Render(runes.bottomLeft+strings.Repeat(runes.horizontal, inner)+runes.bottomRight))

	return strings.Join(out, "\n")
}

// topEdge renders the top border with the title, dropping or cutting the
// title when the pane is too narrow for it.
func topEdge(title string, w int, runes borderRunes, border, text func(...string) string) string {
	room := w - boxTitleChrome
	if title == "" || room < 1 {
		return border(runes.topLeft + strings.Repeat(runes.horizontal, w-2) + runes.topRight)
	}

	title = ansi.Truncate(title, room, "…")
	rest := room - ansi.StringWidth(title)

	return border(runes.topLeft+runes.horizontal+" ") + text(title) +
		border(" "+strings.Repeat(runes.horizontal, rest)+runes.topRight)
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
