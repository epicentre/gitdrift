package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

// spinInterval is how long the spinner shows each frame.
const spinInterval = 100 * time.Millisecond

// Header progress while repositories load.
const (
	progressReading  = "reading %d/%d"
	progressFetching = "fetching %d/%d"
)

// spinnerFrames are the frames of the spinner a loading row shows in place
// of its marker.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinTickMsg turns the spinner one frame.
type spinTickMsg struct{}

// spinTick asks for the next frame of the spinner.
func spinTick() tea.Cmd {
	return tea.Tick(spinInterval, func(time.Time) tea.Msg { return spinTickMsg{} })
}

// startSpinner returns the first tick of the spinner when rows load and it
// is not turning yet; ticks keep coming until nothing loads (see turnSpinner).
func (m *Model) startSpinner() tea.Cmd {
	if m.spinning || !m.anyLoading() {
		return nil
	}

	m.spinning = true

	return spinTick()
}

// turnSpinner shows the next frame and asks for another while rows load; it
// stops the spinner otherwise.
func (m *Model) turnSpinner() tea.Cmd {
	if !m.anyLoading() {
		m.spinning = false

		return nil
	}

	m.spinFrame = (m.spinFrame + 1) % len(spinnerFrames)

	return spinTick()
}

// anyLoading reports whether a first read or a fetch is still running.
func (m Model) anyLoading() bool {
	return len(m.unread) > 0 || m.fetchesRunning()
}

// fetchesRunning reports whether a group of the fetch batch still fetches or
// waits to.
func (m Model) fetchesRunning() bool {
	for dir := range m.fetchBatch {
		if m.fetchPending(dir) {
			return true
		}
	}

	return false
}

// fetchPending reports whether group dir has a fetch running or waiting to
// run.
func (m Model) fetchPending(dir string) bool {
	return m.refreshing[dir] || m.queued[dir]
}

// loading reports whether row r is not read yet or its group fetches.
func (m Model) loading(r row) bool {
	return !r.loaded || m.fetchPending(r.snap.Loc.CommonDir)
}

// spinnerOf is the spinner frame row r shows in place of its marker while it
// loads, "" otherwise.
func (m Model) spinnerOf(r row) string {
	if !m.loading(r) {
		return ""
	}

	return spinnerFrames[m.spinFrame]
}

// progress is the header's account of the loading: until every repository
// was read once, how many are; then, while fetches run, how many of the
// repositories whose groups fetch are done; "" once nothing loads.
func (m Model) progress() string {
	if len(m.unread) > 0 {
		var read int

		for _, r := range m.rows {
			if r.loaded {
				read++
			}
		}

		return fmt.Sprintf(progressReading, read, len(m.rows))
	}

	var done, total int

	for _, r := range m.rows {
		dir := r.snap.Loc.CommonDir
		if !m.fetchBatch[dir] {
			continue
		}

		total++

		if !m.fetchPending(dir) {
			done++
		}
	}

	if done == total {
		return ""
	}

	return fmt.Sprintf(progressFetching, done, total)
}
