package ui

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	binaryText    = "binary file"
	truncatedText = "diff truncated at 1 MiB"
	// tabWidth is the distance between the tab stops of diff text.
	tabWidth = 4
)

// lineKind is what one line of a diff is.
type lineKind int

// Kinds of diff lines.
const (
	lineContext lineKind = iota
	lineAdd
	lineDel
	lineHunk
	lineMeta
)

// diffLineKind classifies one diff line by its start alone: file headers
// (---, +++, diff, index, …) are meta, then hunk headers, added, removed and
// context lines.
func diffLineKind(line string) lineKind {
	if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- ") {
		return lineMeta
	}

	return hunkLineKind(line)
}

// hunkLineKind classifies a line by its first character, which is all that
// counts inside a hunk: a removed line "-- x" reads "--- x" and is no header.
// Lines without a diff marker ("diff --git", "index", "\ No newline at end
// of file") are meta.
func hunkLineKind(line string) lineKind {
	switch {
	case strings.HasPrefix(line, "@@"):
		return lineHunk
	case strings.HasPrefix(line, "+"):
		return lineAdd
	case strings.HasPrefix(line, "-"):
		return lineDel
	case line == "", strings.HasPrefix(line, " "):
		return lineContext
	default:
		return lineMeta
	}
}

// diffLine is one line of the Diff pane, ready to draw.
type diffLine struct {
	text string
	kind lineKind
}

// parseDiff splits diff text into lines, sanitizes them, expands tabs and
// classifies each line, telling file headers from hunk content.
func parseDiff(text string) []diffLine {
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}

	raw := strings.Split(text, "\n")
	lines := make([]diffLine, 0, len(raw))
	inHunk := false

	for _, l := range raw {
		kind := diffLineKind(l)

		switch {
		case strings.HasPrefix(l, "diff "):
			inHunk = false
		case kind == lineHunk:
			inHunk = true
		case inHunk:
			kind = hunkLineKind(l)
		}

		lines = append(lines, diffLine{text: cleanDiffLine(l), kind: kind})
	}

	return lines
}

// cleanDiffLine makes one diff line safe to draw: it sanitizes the line but
// for its tabs, then expands them, so that the tab stops are measured on the
// text that is drawn.
func cleanDiffLine(line string) string { return expandTabs(sanitizeKeepTabs(line)) }

// expandTabs replaces each tab with the spaces up to the next tab stop,
// counting columns in display width.
func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}

	var (
		b   strings.Builder
		col int
	)

	for _, r := range s {
		if r == '\t' {
			n := tabWidth - col%tabWidth
			b.WriteString(strings.Repeat(" ", n))
			col += n

			continue
		}

		b.WriteRune(r)

		if r < utf8.RuneSelf {
			col++
		} else {
			col += ansi.StringWidth(string(r))
		}
	}

	return b.String()
}

// diffState is the Diff pane.
type diffState struct {
	loading bool
	err     error
	// title names what the diff shows: a file path or a short commit id.
	title string
	// header starts a whole-commit diff with the commit it shows.
	header    string
	lines     []diffLine
	truncated bool
	binary    bool
	// scroll is the first line shown.
	scroll int
}

// loaded stores a diff that arrived, scrolled to its start.
func (d diffState) loaded(msg diffLoadedMsg) diffState {
	d.loading, d.err, d.truncated, d.scroll = false, msg.err, msg.truncated, 0
	d.lines = parseDiff(msg.text)

	if d.header != "" && msg.err == nil {
		d.lines = append([]diffLine{{text: d.header, kind: lineMeta}}, d.lines...)
	}

	return d
}

// lineCount is the number of lines the diff draws, its truncation notice
// included.
func (d diffState) lineCount() int {
	if d.truncated {
		return len(d.lines) + 1
	}

	return len(d.lines)
}

// maxScroll is the last scroll offset that still fills height lines.
func (d diffState) maxScroll(height int) int {
	return max(d.lineCount()-height, 0)
}

// render draws the inside of the Diff pane, width x height cells, from the
// scroll offset, after a one-cell gutter like the other panes. Lines are cut
// to the width, never wrapped.
func (d diffState) render(width, height int, st Styles) []string {
	switch {
	case d.err != nil:
		return []string{errorLine(d.err, st)}
	case d.loading:
		return []string{" " + st.Muted.Render(loadingText)}
	case d.binary:
		return []string{" " + st.Muted.Render(binaryText)}
	}

	text := max(width-compactGutter, 0)

	start := min(d.scroll, d.maxScroll(height))
	end := min(start+height, d.lineCount())
	out := make([]string, 0, max(end-start, 0))

	for i := start; i < end; i++ {
		if i == len(d.lines) {
			out = append(out, " "+st.Muted.Render(ansi.Truncate(truncatedText, text, "…")))

			continue
		}

		l := d.lines[i]
		out = append(out, " "+lineStyle(l.kind, st).Render(ansi.Truncate(l.text, text, "…")))
	}

	return out
}

func lineStyle(k lineKind, st Styles) lipgloss.Style {
	switch k {
	case lineAdd:
		return st.DiffAdd
	case lineDel:
		return st.DiffDel
	case lineHunk:
		return st.DiffHunk
	case lineMeta:
		return st.DiffMeta
	case lineContext:
		return lipgloss.NewStyle()
	default:
		return lipgloss.NewStyle()
	}
}

// diffTitle is the Diff pane title: the file or commit it shows.
func (m Model) diffTitle() string {
	if t := m.detail.diff.title; t != "" {
		return titleDiff + separator + t
	}

	return titleDiff
}

// commitHeader starts the whole-commit diff of c.
func commitHeader(c repo.Commit) string {
	return "commit " + commitShort(c) + " " + sanitize(c.Subject)
}

// scrollDiff scrolls the Diff pane with a navigation key.
func (m *Model) scrollDiff(msg tea.KeyPressMsg) {
	height := innerHeight(m.layout().Diff)
	d := &m.detail.diff

	if next, ok := m.keys.navigate(msg, d.scroll, d.maxScroll(height), max(height-1, 1)); ok {
		d.scroll = next
	}
}
