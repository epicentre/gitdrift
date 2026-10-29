package ui

import (
	"charm.land/lipgloss/v2"
)

// Styles holds every style the UI draws with. The zero value draws plain
// text.
type Styles struct {
	// Brand is the program name in the header.
	Brand lipgloss.Style
	// Muted is secondary text: the root, hints, "… N more".
	Muted lipgloss.Style
	// Strong is emphasized text, such as the repository in the status line.
	Strong lipgloss.Style

	// Attention, Behind and UpToDate color the row markers.
	Attention, Behind, UpToDate lipgloss.Style
	// Selected marks a row selected for pull.
	Selected lipgloss.Style
	// Cursor marks the row under the cursor.
	Cursor lipgloss.Style
	// Badge colors the "what to do after pull" badges.
	Badge lipgloss.Style
	// Heading draws column titles.
	Heading lipgloss.Style
	// TabActive draws the active folder tab as a filled box; reverse video
	// keeps it apart where colors are turned off.
	TabActive lipgloss.Style

	// Border and Title draw an unfocused pane; BorderFocused and
	// TitleFocused draw the focused one.
	Border, BorderFocused lipgloss.Style
	Title, TitleFocused   lipgloss.Style

	// DiffAdd, DiffDel, DiffHunk and DiffMeta color added and removed lines,
	// hunk headers and file headers of a diff; context lines are plain.
	DiffAdd, DiffDel, DiffHunk, DiffMeta lipgloss.Style
}

// newStyles builds the palette for a dark or a light terminal background.
func newStyles(isDark bool) Styles {
	ld := lipgloss.LightDark(isDark)

	accent := ld(lipgloss.Color("#005FD7"), lipgloss.Color("#5FAFFF"))
	muted := ld(lipgloss.Color("#767676"), lipgloss.Color("#8A8A8A"))
	border := ld(lipgloss.Color("#B2B2B2"), lipgloss.Color("#585858"))
	red := ld(lipgloss.Color("#D70000"), lipgloss.Color("#FF5F5F"))
	yellow := ld(lipgloss.Color("#AF8700"), lipgloss.Color("#FFD75F"))
	green := ld(lipgloss.Color("#008700"), lipgloss.Color("#87D787"))
	magenta := ld(lipgloss.Color("#AF00AF"), lipgloss.Color("#D787FF"))

	return Styles{
		Brand:         lipgloss.NewStyle().Foreground(accent).Bold(true),
		Muted:         lipgloss.NewStyle().Foreground(muted),
		Strong:        lipgloss.NewStyle().Bold(true),
		Attention:     lipgloss.NewStyle().Foreground(red).Bold(true),
		Behind:        lipgloss.NewStyle().Foreground(yellow).Bold(true),
		UpToDate:      lipgloss.NewStyle().Foreground(green),
		Selected:      lipgloss.NewStyle().Foreground(magenta).Bold(true),
		Cursor:        lipgloss.NewStyle().Foreground(accent).Bold(true),
		Badge:         lipgloss.NewStyle().Foreground(magenta),
		Heading:       lipgloss.NewStyle().Foreground(muted).Bold(true),
		TabActive:     lipgloss.NewStyle().Foreground(accent).Reverse(true).Bold(true),
		Border:        lipgloss.NewStyle().Foreground(border),
		BorderFocused: lipgloss.NewStyle().Foreground(accent),
		Title:         lipgloss.NewStyle().Foreground(muted),
		TitleFocused:  lipgloss.NewStyle().Foreground(accent).Bold(true),
		DiffAdd:       lipgloss.NewStyle().Foreground(green),
		DiffDel:       lipgloss.NewStyle().Foreground(red),
		DiffHunk:      lipgloss.NewStyle().Foreground(accent),
		DiffMeta:      lipgloss.NewStyle().Bold(true),
	}
}
