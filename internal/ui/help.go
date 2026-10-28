package ui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const titleHelp = "Help"

// helpOverlay lists every key of the dashboard. Any key closes it.
type helpOverlay struct{}

// view renders the full help of the key map. Its columns wrap into rows of
// columns when the window is too narrow for all of them side by side.
func (helpOverlay) view(env dialogEnv, width, _ int) dialogView {
	h := help.New()
	h.Styles = helpStyles(env.st)

	var body []string

	for i, row := range helpRows(h, env.keys.FullHelp(), width) {
		if i > 0 {
			body = append(body, "")
		}

		body = append(body, strings.Split(h.FullHelpView(row), "\n")...)
	}

	return dialogView{title: titleHelp, body: body, footer: footerClose}
}

// helpRows groups the help columns into rows that fit in width cells,
// keeping their order. A column wider than width gets a row of its own.
func helpRows(h help.Model, columns [][]key.Binding, width int) [][][]key.Binding {
	var (
		rows [][][]key.Binding
		row  [][]key.Binding
		used int
	)

	gap := ansi.StringWidth(h.FullSeparator)

	for _, col := range columns {
		w := lipgloss.Width(h.FullHelpView([][]key.Binding{col}))
		if len(row) > 0 && used+gap+w > width {
			rows, row, used = append(rows, row), nil, 0
		}

		if len(row) > 0 {
			used += gap
		}

		row = append(row, col)
		used += w
	}

	if len(row) > 0 {
		rows = append(rows, row)
	}

	return rows
}

// helpStyles draws the help with the dashboard's palette.
func helpStyles(st Styles) help.Styles {
	return help.Styles{
		Ellipsis:       st.Muted,
		ShortKey:       st.Cursor,
		ShortDesc:      lipgloss.NewStyle(),
		ShortSeparator: st.Muted,
		FullKey:        st.Cursor,
		FullDesc:       lipgloss.NewStyle(),
		FullSeparator:  st.Muted,
	}
}
