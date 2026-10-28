package ui

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	resetLabel   = "Reset to default"
	footerPicker = "↑↓ move · enter choose · esc close"
	noMatches    = "no matching branches"
	// pickerHeadRows are the current base, the filter and a blank line
	// above the entries.
	pickerHeadRows = 3
)

// basePicker chooses the base branch of one repository among its
// remote-tracking branches, narrowed by a typed filter.
type basePicker struct {
	loc     repo.Location
	current repo.Base
	// refs are the remote-tracking branches as git lists them; nil while
	// loading is set.
	refs    []string
	loading bool
	err     error
	filter  string
	// cursor indexes entries(); top is the first entry shown.
	cursor, top int
}

// pickerEntry is one choice of the picker: the reset entry, or a branch.
type pickerEntry struct {
	// label is the sanitized text shown; ref is the branch as git lists it.
	label, ref string
	reset      bool
	current    bool
}

// entries lists the choices: Reset to default, then every branch whose name
// contains the filter, ignoring case.
func (p basePicker) entries() []pickerEntry {
	out := []pickerEntry{{label: resetLabel, reset: true}}
	needle := strings.ToLower(sanitize(p.filter))

	for _, ref := range p.refs {
		label := sanitize(ref)
		if !strings.Contains(strings.ToLower(label), needle) {
			continue
		}

		out = append(out, pickerEntry{label: label, ref: ref, current: p.current.Ref != "" && ref == p.current.Ref})
	}

	return out
}

// loaded fills in the answer of RemoteBranches.
func (p basePicker) loaded(refs []string, err error) basePicker {
	p.refs, p.err, p.loading = refs, err, false

	return p.refiltered()
}

// refiltered puts the cursor back after the entries changed: on the first
// matching branch while a filter is typed, else on the current base when it
// is listed. Without a match it rests on the first branch, or on the reset
// entry when there is none.
func (p basePicker) refiltered() basePicker {
	entries := p.entries()
	p.cursor, p.top = min(1, len(entries)-1), 0

	if p.filter != "" {
		return p
	}

	if i := slices.IndexFunc(entries, func(e pickerEntry) bool { return e.current }); i >= 0 {
		p.cursor = i
	}

	return p
}

// pickerRows is the number of entries the picker shows in a body of height
// lines.
func pickerRows(height int) int { return max(height-pickerHeadRows, 1) }

func (p basePicker) view(env dialogEnv, _, height int) dialogView {
	current := "current: none"
	if p.current.Ref != "" {
		current = "current: " + sanitize(p.current.Ref) + " (" + string(p.current.Source) + ")"
	}

	body := []string{current, "filter: " + sanitize(p.filter), ""}
	entries := p.entries()
	start, end := scrollWindow(len(entries), p.cursor, p.top, pickerRows(height))

	for i := start; i < end; i++ {
		body = append(body, entries[i].line(i == p.cursor, env.st))
	}

	if more := len(entries) - end; more > 0 {
		body = append(body, env.st.Muted.Render("… "+strconv.Itoa(more)+" more"))
	}

	switch {
	case p.loading:
		body = append(body, env.st.Muted.Render(loadingText))
	case p.err != nil:
		body = append(body, env.st.Attention.Render(errorPrefix+firstLine(p.err.Error())))
	case len(entries) == 1 && len(p.refs) > 0:
		body = append(body, env.st.Muted.Render(noMatches))
	}

	return dialogView{title: "Base for " + sanitize(p.loc.Name), body: body, footer: footerPicker, width: p.width()}
}

// width fits every entry whatever the filter: the widest name after the
// cursor and current marks.
func (p basePicker) width() int {
	widest := ansi.StringWidth(resetLabel)
	for _, ref := range p.refs {
		widest = max(widest, ansi.StringWidth(sanitize(ref)))
	}

	return ansi.StringWidth(cursorMark+" ● ") + widest
}

// line draws the entry with the cursor mark and ● on the current base.
func (e pickerEntry) line(isCursor bool, st Styles) string {
	mark := " "
	if e.current {
		mark = st.Selected.Render("●")
	}

	return cursorGutter(isCursor, st) + " " + mark + " " + e.label
}

// handlePickerKey moves the cursor with the arrow and page keys, types into
// the filter, chooses with enter once the branches are listed and closes
// with esc.
func (m *Model) handlePickerKey(p basePicker, msg tea.KeyPressMsg) tea.Cmd {
	rows := pickerRows(m.height - chromeRows - dialogFixedRows)
	n := len(p.entries())

	switch {
	case key.Matches(msg, m.keys.Back):
		m.closeDialog()

		return nil
	case key.Matches(msg, m.keys.Enter):
		// While loading only the reset entry is listed; enter must not
		// remove the base before the branches could even be seen. The same
		// holds when a typed filter matches no branch.
		if p.loading || (p.filter != "" && n == 1) {
			return nil
		}

		return m.choose(p)
	case key.Matches(msg, m.keys.Erase):
		runes := []rune(p.filter)
		p.filter = string(runes[:max(len(runes)-1, 0)])
		p = p.refiltered()
	case msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt) == 0:
		p.filter += msg.Text
		p = p.refiltered()
	default:
		next, ok := m.keys.listKeys().navigate(msg, p.cursor, n-1, max(rows-1, 1))
		if !ok {
			return nil
		}

		p.cursor = next
	}

	p.top, _ = scrollWindow(len(p.entries()), p.cursor, p.top, rows)
	m.dialog = p

	return nil
}

// choose stores the branch under the cursor as the base, or removes the
// stored base on the reset entry.
func (m *Model) choose(p basePicker) tea.Cmd {
	entries := p.entries()
	if p.cursor < 0 || p.cursor >= len(entries) {
		return nil
	}

	m.closeDialog()

	if e := entries[p.cursor]; !e.reset {
		return setBaseCmd(m.ctx, m.backend, p.loc, e.ref)
	}

	return resetBaseCmd(m.ctx, m.backend, p.loc)
}
