package ui

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	refOriginDevelop = "origin/develop"
	nameBilling      = "billing"
	keyBackspace     = "backspace"
)

// openPicker presses b and delivers the remote branches it lists.
func openPicker(t *testing.T, m Model) Model {
	t.Helper()

	m, cmd := pressCmd(t, m, "b")
	m = feed(t, m, runCmd(cmd))

	if _, ok := m.dialog.(basePicker); !ok {
		t.Fatalf("b opened %T, want the base picker", m.dialog)
	}

	return m
}

// pickerLabels lists the entries the open picker offers.
func pickerLabels(t *testing.T, m Model) []string {
	t.Helper()

	p, ok := m.dialog.(basePicker)
	if !ok {
		t.Fatalf("dialog is %T, want the base picker", m.dialog)
	}

	var out []string
	for _, it := range p.entries() {
		out = append(out, it.label)
	}

	return out
}

func TestBasePicker(t *testing.T) {
	t.Parallel()

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 120, 40)
	fake.setRemoteBranches([]string{refOriginDevelop, refOriginMain, "origin/release-2026.10", "upstream/main"})
	m = loadAll(t, m, fake)

	m = openPicker(t, m)
	assertCalls(t, fake, "RemoteBranches", []string{"RemoteBranches billing"})
	assertViewContains(t, m, "Base for billing", "current: origin/main (auto)", "Reset to default", "● origin/main",
		"upstream/main")

	m = press(t, m, "d", "E", "v")
	if diff := cmp.Diff([]string{resetLabel, refOriginDevelop}, pickerLabels(t, m)); diff != "" {
		t.Errorf("entries for dEv mismatch (-want +got):\n%s", diff)
	}

	m, cmd := pressCmd(t, m, keyEnter)
	if m.dialog != nil {
		t.Fatalf("enter left %T open", m.dialog)
	}

	m, cmd = updateCmd(t, m, runCmd(cmd)[0])
	assertCalls(t, fake, "SetBase", []string{"SetBase billing origin/develop"})
	m = feed(t, m, runCmd(cmd))
	assertCalls(t, fake, "Refresh", []string{refreshCall(nameBilling, false)})

	m = openPicker(t, m)
	m, cmd = pressCmd(t, press(t, m, keyPgUp), keyEnter)
	m, cmd = updateCmd(t, m, runCmd(cmd)[0])
	assertCalls(t, fake, "ResetBase", []string{"ResetBase billing"})
	m = feed(t, m, runCmd(cmd))
	assertCalls(t, fake, "Refresh", []string{refreshCall(nameBilling, false), refreshCall(nameBilling, false)})

	m = press(t, openPicker(t, m), "q", keyEsc)
	if m.dialog != nil {
		t.Errorf("esc left %T open", m.dialog)
	}

	if n := len(fake.Calls("SetBase")) + len(fake.Calls("ResetBase")); n != 2 {
		t.Errorf("esc wrote the base: %d writes, want 2", n)
	}
}

func TestBasePickerFilter(t *testing.T) {
	t.Parallel()

	const long = "origin/release/2026-10-hotfix-for-very-old-clients"

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 120, 40)
	fake.setRemoteBranches([]string{refOriginDevelop, refOriginMain, "origin/fe\x1b[31mature\x07", long})
	m = openPicker(t, loadAll(t, m, fake))

	steps := []struct {
		keys []string
		want []string
	}{
		{nil, []string{resetLabel, refOriginDevelop, refOriginMain, "origin/feature", long}},
		{[]string{"x", "y", "z"}, []string{resetLabel}},
		{[]string{keyBackspace, keyBackspace, keyBackspace, "M", "A"}, []string{resetLabel, refOriginMain}},
		{[]string{keyBackspace, keyBackspace, "t", "u"}, []string{resetLabel, "origin/feature"}},
	}

	f, _ := m.frame()
	width := f.inner

	for i, s := range steps {
		m = press(t, m, s.keys...)
		if diff := cmp.Diff(s.want, pickerLabels(t, m)); diff != "" {
			t.Errorf("step %d %v: entries mismatch (-want +got):\n%s", i, s.keys, diff)
		}

		if f, _ := m.frame(); f.inner != width {
			t.Errorf("step %d %v: the window is %d cells wide, want it to keep %d", i, s.keys, f.inner, width)
		}
	}

	if !strings.Contains(viewText(m), "filter: tu") {
		t.Errorf("view does not show the filter:\n%s", viewText(m))
	}

	m, cmd := pressCmd(t, m, keyEnter)
	runCmd(cmd)

	want := []string{"SetBase billing origin/fe\x1b[31mature\x07"}
	if diff := cmp.Diff(want, fake.Calls("SetBase")); diff != "" {
		t.Errorf("SetBase gets the ref as listed (-want +got):\n%s", diff)
	}

	if m.dialog != nil {
		t.Errorf("enter left %T open", m.dialog)
	}
}

func TestBasePickerCurrent(t *testing.T) {
	t.Parallel()

	repoBase := fixtureBilling(fixedNow)
	repoBase.Base = repo.Base{Configured: refOriginDevelop, Source: repo.SourceRepo, Ref: refOriginDevelop, SHA: shaBase}

	noBase := fixtureBilling(fixedNow)
	noBase.Base = repo.Base{}

	notFound := fixtureBilling(fixedNow)
	notFound.Base = repo.Base{Configured: "origin/gone", Source: repo.SourceGlobal, NotFound: true}

	tests := []struct {
		name string
		snap drift.Snapshot
		want string
	}{
		{"auto", fixtureBilling(fixedNow), "current: origin/main (auto)"},
		{"repo", repoBase, "current: origin/develop (repo)"},
		{"none", noBase, "current: none"},
		{"not found", notFound, "current: none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, fake := loadedModel(t, []drift.Snapshot{tt.snap}, 120, 40)
			fake.setRemoteBranches([]string{refOriginDevelop, refOriginMain})

			m = openPicker(t, m)
			assertViewContains(t, m, tt.want)

			marked := tt.snap.Base.Ref
			if marked == "" {
				if strings.Contains(viewText(m), "●") {
					t.Errorf("an entry is marked current with no base:\n%s", viewText(m))
				}

				return
			}

			assertViewContains(t, m, "● "+marked)
		})
	}
}

// pickerCursor is the label of the entry under the open picker's cursor.
func pickerCursor(t *testing.T, m Model) string {
	t.Helper()

	p, ok := m.dialog.(basePicker)
	if !ok {
		t.Fatalf("dialog is %T, want the base picker", m.dialog)
	}

	return p.entries()[p.cursor].label
}

func TestBasePickerCursorStart(t *testing.T) {
	t.Parallel()

	notListed := fixtureBilling(fixedNow)
	notListed.Base = repo.Base{Configured: "origin/gone", Source: repo.SourceRepo, NotFound: true}

	tests := []struct {
		name string
		snap drift.Snapshot
		refs []string
		want string
	}{
		{"on the current base", fixtureBilling(fixedNow), []string{refOriginDevelop, refOriginMain, "upstream/main"}, refOriginMain},
		{"on the first branch without a listed base", notListed, []string{refOriginDevelop, refOriginMain}, refOriginDevelop},
		{"on the reset entry without branches", fixtureBilling(fixedNow), nil, resetLabel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, fake := loadedModel(t, []drift.Snapshot{tt.snap}, 120, 40)
			fake.setRemoteBranches(tt.refs)

			m = openPicker(t, m)
			if got := pickerCursor(t, m); got != tt.want {
				t.Errorf("cursor on %q, want %q", got, tt.want)
			}

			// A filter typed and erased puts the cursor back where it started.
			if got := pickerCursor(t, press(t, m, "x", keyBackspace)); got != tt.want {
				t.Errorf("after an erased filter the cursor is on %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBasePickerIgnoresEnterWhileLoading(t *testing.T) {
	t.Parallel()

	m, fake := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)

	m, _ = pressCmd(t, m, "b")

	m, cmd := pressCmd(t, m, keyEnter)
	if _, ok := m.dialog.(basePicker); !ok {
		t.Fatalf("enter while loading closed the picker: dialog is %T", m.dialog)
	}

	runCmd(cmd)

	if n := len(fake.Calls("SetBase")) + len(fake.Calls("ResetBase")); n != 0 {
		t.Errorf("enter while loading wrote the base: SetBase %v, ResetBase %v", fake.Calls("SetBase"), fake.Calls("ResetBase"))
	}
}

func TestBasePickerIgnoresEnterWithoutMatches(t *testing.T) {
	t.Parallel()

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 120, 40)
	fake.setRemoteBranches([]string{refOriginDevelop, refOriginMain})
	m = openPicker(t, loadAll(t, m, fake))

	// A filter that matches no branch leaves only the reset entry; enter
	// must not reset the base the user was searching for a branch to set.
	m = press(t, m, "x", "y", "z")

	m, cmd := pressCmd(t, m, keyEnter)
	if _, ok := m.dialog.(basePicker); !ok {
		t.Fatalf("enter without matches closed the picker: dialog is %T", m.dialog)
	}

	runCmd(cmd)

	if n := len(fake.Calls("SetBase")) + len(fake.Calls("ResetBase")); n != 0 {
		t.Errorf("enter without matches wrote the base: SetBase %v, ResetBase %v", fake.Calls("SetBase"), fake.Calls("ResetBase"))
	}
}

func TestBasePickerLoadingAndError(t *testing.T) {
	t.Parallel()

	m, _ := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)

	m, _ = pressCmd(t, m, "b")
	assertViewContains(t, m, "Base for billing", resetLabel, loadingText)

	id := fixtureLoc(nameBilling).ID
	other := step(t, m, remoteBranchesMsg{id: fixtureLoc(namePayments).ID, refs: []string{refOriginDevelop}})

	if got := pickerLabels(t, other); len(got) != 1 {
		t.Errorf("an answer for another repository filled the picker: %v", got)
	}

	failed := step(t, m, remoteBranchesMsg{id: id, err: errors.New("git for-each-ref: fatal: \x1b[2Jbad\nmore")})
	assertViewContains(t, failed, "error: git for-each-ref: fatal: bad")

	if strings.Contains(viewText(failed), "more") {
		t.Errorf("the error shows more than its first line:\n%s", viewText(failed))
	}
}

func TestBasePickerScrolls(t *testing.T) {
	t.Parallel()

	refs := make([]string, 40)
	for i := range refs {
		refs[i] = fmt.Sprintf("origin/topic-%02d", i)
	}

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 100, 24)
	fake.setRemoteBranches(refs)
	m = openPicker(t, loadAll(t, m, fake))

	assertViewContains(t, m, "origin/topic-00", "more")

	for range 35 {
		m = press(t, m, keyDown)
	}

	// The cursor starts on the first branch, so 35 steps down end on the 36th.
	assertViewContains(t, m, "▸   origin/topic-35")

	if got := viewText(m); strings.Contains(got, "origin/topic-00") {
		t.Errorf("the picker did not scroll:\n%s", got)
	}
}

// TestBasePickerCursorBar draws the entry under the picker's cursor as the
// cursor bar across the window, as the focused pane draws its cursor row.
func TestBasePickerCursorBar(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 120, 40)
	fake.setRemoteBranches([]string{refOriginDevelop, refOriginMain, "origin/release-2026.10"})
	m = openPicker(t, loadAll(t, m, fake))

	// The dialog is drawn in the thick border of the focused window, one
	// cell of padding inside it.
	entry := regexp.MustCompile("┃ (" + cursorMark + "[^┃]*) ┃")

	for _, keys := range [][]string{nil, {keyUp}, {keyUp, keyUp}} {
		m := press(t, m, keys...)

		f, ok := m.frame()
		if !ok {
			t.Fatalf("after %q no dialog is laid out", keys)
		}

		match := entry.FindStringSubmatch(viewText(m))
		if match == nil {
			t.Fatalf("after %q the picker shows no cursor entry:\n%s", keys, viewText(m))
		}

		row := match[1]
		if w := ansi.StringWidth(row); w != f.inner {
			t.Errorf("after %q the cursor entry %q is %d cells wide, want the window's %d", keys, row, w, f.inner)
		}

		if !strings.Contains(m.View().Content, m.styles.CursorLine.Render(row)) {
			t.Errorf("after %q the cursor entry %q is not drawn as the cursor bar", keys, row)
		}
	}

	// The picker takes the keys, so the focused pane under it shows only
	// the gutter mark: one bar on screen, where the arrows move.
	if row := cursorRowIn(t, m, PaneRepos); strings.Contains(m.View().Content, m.styles.CursorLine.Render(row)) {
		t.Errorf("the Repos cursor row %q is drawn as the cursor bar under the picker", row)
	}
}
