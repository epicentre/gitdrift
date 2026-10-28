package ui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	// filterPrefix starts the status line while a filter is typed or kept.
	filterPrefix = "filter: "
	// filterCaret ends the filter while it is typed.
	filterCaret = "▏"
	// noMatchingRepos stands in the Repos pane for the rows when none is
	// visible.
	noMatchingRepos = "no matching repositories"
	// noticeMinCells is the least room a notice or a status line is shown
	// in after the filter: a character and "…", never a bare "…".
	noticeMinCells = 2
)

// matchesFilter reports whether needle, ignoring case, is part of the
// sanitized name or of the sanitized folder of r's repository. The folder is
// the full folder below the workspace, not the text of its FOLDER cell,
// which is relative to the folder tab and may be cut; a needle across the
// folder and the name does not match.
func matchesFilter(r row, needle string) bool {
	needle = strings.ToLower(needle)

	return strings.Contains(strings.ToLower(sanitize(r.snap.Loc.Name)), needle) ||
		strings.Contains(strings.ToLower(sanitize(r.snap.Loc.Folder)), needle)
}

// handleFilterKey takes a key press while a filter is typed: a key with text
// types it, q and digits included, backspace erases the last character,
// enter keeps the filter and esc clears it, both ending the typing. Every
// other key does nothing, and so does every key but enter and esc while the
// screen is too small to show the filter.
func (m *Model) handleFilterKey(msg tea.KeyPressMsg) {
	switch {
	case key.Matches(msg, m.keys.Enter):
		m.stopTyping()
	case key.Matches(msg, m.keys.Back):
		m.setFilter("")
		m.stopTyping()
	case m.layout().TooSmall:
		// Nothing is typed that cannot be seen.
	case key.Matches(msg, m.keys.Erase):
		runes := []rune(m.filter)
		m.setFilter(string(runes[:max(len(runes)-1, 0)]))
	case msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0:
		m.setFilter(m.filter + msg.Text)
	}
}

// handlePaste takes a paste: while a filter is typed and can be seen, the
// first line of the paste is typed into it, sanitized by setFilter; every
// other paste is ignored. Typing it clears the notice, as a key press does.
func (m *Model) handlePaste(msg tea.PasteMsg) {
	if !m.filtering || m.layout().TooSmall {
		return
	}

	line := msg.Content
	if end := strings.IndexAny(line, "\r\n"); end >= 0 {
		line = line[:end]
	}

	m.notice = ""
	m.setFilter(m.filter + line)
}

// stopTyping ends the typing of the filter and opens the first window that
// arrived meanwhile.
func (m *Model) stopTyping() {
	m.filtering = false
	m.openWaiting()
}

// setFilter replaces the filter with text, sanitized, so that the filter
// holds only what the status line shows and backspace never erases a
// character that cannot be seen. The cursor stays on its repository when the
// filter keeps it visible, and else goes to the first row.
func (m *Model) setFilter(text string) {
	text = sanitize(text)
	if text == m.filter {
		return
	}

	id, ok := m.cursorID()
	m.filter = text
	m.cursor = 0
	m.visibleChanged(id, ok)
}

// filterLine is the status line of filter: the text, then a caret while it
// is typed. A filter too wide for width cells loses its oldest characters
// behind "…", so that the newest ones and the caret stay in sight.
func filterLine(filter string, typing bool, width int, st Styles) string {
	text, caret := sanitize(filter), ""
	if typing {
		caret = filterCaret
	}

	if room := width - ansi.StringWidth(filterPrefix+caret); room > 0 {
		text = cutLeft(text, room)
	}

	line := filterPrefix + text
	if typing {
		line += st.Muted.Render(caret)
	}

	return ansi.Truncate(line, width, "…")
}

// typingLine is the status line while filter is typed: its filterLine, then
// the notice of the last key, if any, in what width leaves, so that the typed
// text and the caret stay in sight. A notice with less than noticeMinCells
// of room is left out.
func typingLine(filter, notice string, width int, st Styles) string {
	line := filterLine(filter, true, width, st)

	room, fits := roomAfter(line, width)
	if notice == "" || !fits {
		return line
	}

	return line + separator + st.Attention.Render(ansi.Truncate(notice, room, "…"))
}

// roomAfter is the room a line width cells wide leaves after line and a
// separator, and whether it holds at least noticeMinCells, the least a part
// that follows the filter on the status line is shown in.
func roomAfter(line string, width int) (int, bool) {
	room := width - ansi.StringWidth(line) - ansi.StringWidth(separator)

	return room, room >= noticeMinCells
}
