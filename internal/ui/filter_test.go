package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	// filterPlatform matches the folder services/platform of the nested
	// fixture, and no repository name.
	filterPlatform = "plat"
	// filterStorefront matches storefront alone.
	filterStorefront = "stor"
	// filterNothing matches no repository.
	filterNothing = "zzz"
	nameSearch    = "search"
)

// typeFilter presses / and types each character of text.
func typeFilter(t *testing.T, m Model, text string) Model {
	t.Helper()

	m = press(t, m, "/")
	for _, r := range text {
		m = press(t, m, string(r))
	}

	return m
}

// platformRows are the rows of the nested fixture in services/platform, in
// the order of the Repos pane.
func platformRows() []string {
	var out []string

	for _, p := range nestedOrder() {
		if strings.HasPrefix(p, folderPlatform+"/") {
			out = append(out, p)
		}
	}

	return out
}

// storefrontRows is the row of storefront alone.
func storefrontRows() []string { return []string{folderWeb + "/" + nameStorefront} }

func TestMatchesFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, folder, repoName, needle string
		want                           bool
	}{
		{"part of the name", folderWeb, nameStorefront, "front", true},
		{"part of the folder", folderPlatform, nameSearch, filterPlatform, true},
		{"the folder across its segments", folderPlatform, nameSearch, "services/plat", true},
		{"not across the folder and the name", folderPlatform, nameSearch, "platform/search", false},
		{"the name ignores case", folderPlatform, nameNotifications, "NoTiF", true},
		{"the folder ignores case", "Services/Platform", nameSearch, "sERVICES/p", true},
		{"no match", folderWeb, "admin", "pay", false},
		{"a root-level repository by name", "", "handbook", "hand", true},
		{"an empty needle matches everything", "", "handbook", "", true},
		{"escape sequences in the name are not matched", "", "pay\x1b[31mments\a", "payments", true},
		{"the text of an escape sequence is not matched", "", "pay\x1b[31mments", "31m", false},
		{"escape sequences in the folder are not matched", "ser\x1b[1mvices", nameBilling, folderServices, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := row{snap: drift.Snapshot{Loc: fixtureLocIn(tt.folder, tt.repoName)}}
			if got := matchesFilter(r, tt.needle); got != tt.want {
				t.Errorf("matchesFilter(%q in %q, %q) = %t, want %t", tt.repoName, tt.folder, tt.needle, got, tt.want)
			}
		})
	}
}

// TestFilterTyping types a filter into the Repos pane of the nested
// fixture: the rows narrow to those whose name or folder contains it, q
// types instead of quitting, backspace widens, enter keeps the filter, esc
// clears it, and ctrl+c still quits.
func TestFilterTyping(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	// notificationsStatus is the status line of notifications.
	const notificationsStatus = "services/platform/notifications · base origin/main (auto)"

	m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)
	m = moveTo(t, m, nameSearch)

	steps := []struct {
		keys           []string
		filter, status string
		typing         bool
		shown          []string
		cursor         string
	}{
		{[]string{"/", "p", "l", "a", "t"}, filterPlatform, "filter: plat▏", true, platformRows(), nameSearch},
		{[]string{"q"}, "platq", "filter: platq▏", true, nil, ""},
		{[]string{keyBackspace}, filterPlatform, "filter: plat▏", true, platformRows(), nameNotifications},
		{[]string{keyEnter}, filterPlatform, "filter: plat · " + notificationsStatus, false, platformRows(), nameNotifications},
		{[]string{"/", keyEsc}, "", notificationsStatus, false, nestedOrder(), nameNotifications},
	}

	for _, s := range steps {
		for _, k := range s.keys {
			var cmd tea.Cmd
			if m, cmd = pressCmd(t, m, k); quits(cmd) {
				t.Fatalf("%q quit while typing a filter", k)
			}
		}

		_, _, shown := reposPane(t, m)
		got := fmt.Sprintf("filter %q, typing %t, cursor on %q, status %q", m.filter, m.filtering, cursorName(m), statusText(m))
		want := fmt.Sprintf("filter %q, typing %t, cursor on %q, status %q", s.filter, s.typing, s.cursor, s.status)

		if got != want {
			t.Errorf("after %q: %s; want %s", s.keys, got, want)
		}

		if diff := cmp.Diff(s.shown, shown, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("after %q: rows mismatch (-want +got):\n%s", s.keys, diff)
		}
	}

	if _, cmd := pressCmd(t, typeFilter(t, m, "pa"), keyCtrlC); !quits(cmd) {
		t.Error("ctrl+c while typing a filter did not quit")
	}
}

// TestFilterTakesKeys checks that every printable key typed into the filter
// goes to the filter, whatever it does otherwise: it quits nothing, focuses
// no pane, switches no tab, opens no window and starts no refresh.
func TestFilterTakesKeys(t *testing.T) {
	t.Parallel()

	keys := map[string]string{keySpace: " "}
	for _, k := range []string{"q", "1", "2", "4", "[", "]", "f", "z", "?", "p", "P", "r", "R", "b", "d", "j", "G", "/"} {
		keys[k] = k
	}

	for k, text := range keys {
		t.Run(k, func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)

			m, cmd := pressCmd(t, press(t, m, "/"), k)
			msgs := runCmd(cmd)

			if len(messagesOf[tea.QuitMsg](msgs)) > 0 || len(messagesOf[groupRefreshedMsg](msgs)) > 0 {
				t.Errorf("%q typed into the filter quit or refreshed: %v", k, msgs)
			}

			// The state each of those keys would change, and the filter.
			type state struct {
				Filter                 string
				Typing, Zoom, OnlyHere bool
				Focus                  Pane
				FolderTab, Dialog      string
				CommitsTab             Tab
				Selected               int
			}

			got := state{
				m.filter, m.filtering, m.zoom, m.onlyHere, m.focus, m.folderTab, fmt.Sprintf("%T", m.dialog),
				m.detail.tab, len(m.selected),
			}
			want := state{Filter: text, Typing: true, Focus: PaneRepos, Dialog: "<nil>", CommitsTab: TabIncoming}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("%q typed into the filter: state mismatch (-want +got):\n%s", k, diff)
			}
		})
	}
}

// TestFilterIgnoresOtherKeys checks that the keys that type nothing do
// nothing while a filter is typed.
func TestFilterIgnoresOtherKeys(t *testing.T) {
	t.Parallel()

	for _, k := range []string{keyUp, keyDown, keyPgDown, keyHome, keyEnd, keyTab, keyShiftTab} {
		t.Run(k, func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)
			m = typeFilter(t, moveTo(t, m, nameStorefront), "s")
			cursor := m.cursor

			if m = press(t, m, k); !m.filtering || m.filter != "s" || m.cursor != cursor || m.focus != PaneRepos {
				t.Errorf("%s while typing: typing %t, filter %q, cursor %d (was %d), focus %d; want nothing changed",
					k, m.filtering, m.filter, m.cursor, cursor, m.focus)
			}
		})
	}
}

// TestFilterOnTooSmallScreen checks that no filter is typed while the
// screen is too small to show it: / starts nothing there, and a filter
// being typed when the screen shrinks takes no text until it grows again,
// while enter and esc still end the typing.
func TestFilterOnTooSmallScreen(t *testing.T) {
	t.Parallel()

	m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)

	if small := press(t, resize(t, m, 59, 15), "/"); small.filtering {
		t.Error("/ started typing a filter on a too small screen")
	}

	m = resize(t, typeFilter(t, m, "s"), 59, 15)
	for _, k := range []string{"x", keyBackspace, "q"} {
		if m = press(t, m, k); m.filter != "s" || !m.filtering {
			t.Errorf("%s on a too small screen: filter %q, typing %t; want s, typing", k, m.filter, m.filtering)
		}
	}

	if m = press(t, resize(t, m, 150, 40), "x"); m.filter != "sx" {
		t.Errorf("typing after the screen grew: filter %q, want sx", m.filter)
	}

	if m = press(t, resize(t, m, 59, 15), keyEsc); m.filter != "" || m.filtering {
		t.Errorf("esc on a too small screen: filter %q, typing %t; want none", m.filter, m.filtering)
	}
}

// TestFilterWithTabs checks that the filter applies inside the folder tab,
// stays when the tab changes and narrows the tab counts.
func TestFilterWithTabs(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)
	m = press(t, typeFilter(t, m, filterStorefront), keyEnter)

	if _, _, shown := reposPane(t, m); !cmp.Equal(shown, storefrontRows()) {
		t.Errorf("All with /%s shows %q, want storefront", filterStorefront, shown)
	}

	assertViewContains(t, m, "╭─ 1  [All 1/7]  services 0/4  web 1/2 ─")

	m = press(t, m, "]")
	assertViewContains(t, m, "╭─ 1  All 1/7  [services 0/4]  web 1/2 ─", noMatchingRepos)

	m = press(t, m, "]")
	if _, _, shown := reposPane(t, m); m.folderTab != folderWeb || m.filter != filterStorefront ||
		!cmp.Equal(shown, storefrontRows()) {
		t.Errorf("] ] with /%s: tab %q, filter %q, rows %q; want web, the filter kept and storefront",
			filterStorefront, m.folderTab, m.filter, shown)
	}

	assertViewContains(t, m, "[web 1/2]", "filter: stor")
}

// TestFilterEscPrecedence checks that esc in the Repos pane clears a kept
// filter before it leaves zoom, that esc in the other panes does what it did
// before and keeps the filter, and that esc while typing clears the filter
// and nothing else.
func TestFilterEscPrecedence(t *testing.T) {
	t.Parallel()

	kept := []string{"/", "s", "t", "o", "r", keyEnter}

	tests := []struct {
		name   string
		keys   []string
		filter string
		zoom   bool
		focus  Pane
	}{
		{"zoomed Repos: esc clears the filter, keeps zoom", slices.Concat([]string{"z"}, kept, []string{keyEsc}), "", true, PaneRepos},
		{"zoomed Repos: the next esc leaves zoom", slices.Concat([]string{"z"}, kept, []string{keyEsc, keyEsc}), "", false, PaneRepos},
		{"Commits: esc goes back to Repos", slices.Concat(kept, []string{"2", keyEsc}), filterStorefront, false, PaneRepos},
		{"Commits: the next esc, in Repos, clears it", slices.Concat(kept, []string{"2", keyEsc, keyEsc}), "", false, PaneRepos},
		{"zoomed Commits: esc leaves zoom", slices.Concat(kept, []string{"2", "z", keyEsc}), filterStorefront, false, PaneCommits},
		{"Diff: esc goes back to Files", slices.Concat(kept, []string{"4", keyEsc}), filterStorefront, false, PaneFiles},
		{"typing in zoom: esc clears the filter, keeps zoom", []string{"z", "/", "s", keyEsc}, "", true, PaneRepos},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)
			m = press(t, m, tt.keys...)

			got := fmt.Sprintf("filter %q, typing %t, zoom %t, focus %d", m.filter, m.filtering, m.zoom, m.focus)
			if want := fmt.Sprintf("filter %q, typing false, zoom %t, focus %d", tt.filter, tt.zoom, tt.focus); got != want {
				t.Errorf("after %q: %s; want %s", tt.keys, got, want)
			}
		})
	}
}

// TestFilterStartsOnlyInRepos checks that / starts no filter with the
// Commits, Files or Diff pane focused, so that q still quits there.
func TestFilterStartsOnlyInRepos(t *testing.T) {
	t.Parallel()

	for _, focus := range []string{"2", "3", "4"} {
		t.Run(focus, func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)

			m = press(t, m, focus, "/")
			if m.filtering {
				t.Errorf("/ in pane %s started typing a filter", focus)
			}

			if _, cmd := pressCmd(t, m, "q"); !quits(cmd) {
				t.Errorf("q after / in pane %s did not quit", focus)
			}
		})
	}
}

// TestFilterQueuesWindows checks that a window arriving while a filter is
// typed waits until the typing ends: the typed keys never reach it, so a
// typed y confirms no deletion and a typed key dismisses no delete result.
func TestFilterQueuesWindows(t *testing.T) {
	t.Parallel()

	loc := fixtureLocIn(folderServices, namePayments)
	confirm := deleteLoadedMsg{loc: loc, items: []deleteItem{{gone: repo.GoneBranch{Name: branchCashRounding, SHA: shaGone}}}}

	tests := []struct {
		name string
		msg  tea.Msg
		// end is the key that ends the typing.
		end  string
		want dialog
	}{
		{"the delete dialog", confirm, keyEnter, deleteConfirm{}},
		{"a delete result", cashRoundingDeleted(), keyEsc, deleteResult{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, fake := loadedModel(t, fixtureNested(fixedNow), 150, 40)

			if m = step(t, typeFilter(t, m, "pa"), tt.msg); m.dialog != nil {
				t.Fatalf("%T opened while a filter is typed", m.dialog)
			}

			m, cmd := pressCmd(t, m, "y")
			if msgs := runCmd(cmd); len(messagesOf[deleteDoneMsg](msgs)) > 0 || len(fake.Calls("DeleteGone")) > 0 {
				t.Fatal("y typed into the filter deleted a branch")
			}

			if m.dialog != nil || m.filter != "pay" {
				t.Errorf("after y: dialog %T, filter %q; want none, pay", m.dialog, m.filter)
			}

			if m = press(t, m, tt.end); fmt.Sprintf("%T", m.dialog) != fmt.Sprintf("%T", tt.want) || m.filtering {
				t.Errorf("after %s: dialog %T, typing %t; want %T", tt.end, m.dialog, m.filtering, tt.want)
			}
		})
	}
}

// TestFilterLine checks the status line of a filter: the caret while it is
// typed, and a filter too wide for the line cut from the left, so that its
// newest characters and the caret stay visible.
func TestFilterLine(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	st := newStyles(true)

	tests := []struct {
		name, filter string
		typing       bool
		width        int
		want         string
	}{
		{"typing", filterPlatform, true, 80, "filter: plat▏"},
		{"kept", filterPlatform, false, 80, "filter: plat"},
		{"escape sequences are not shown", "pl\x1b[31mat\a", false, 80, "filter: plat"},
		{"exactly as wide", "abcdefgh", true, 17, "filter: abcdefgh▏"},
		{"too wide keeps the newest characters and the caret", "abcdefghij", true, 17, "filter: …defghij▏"},
		{"too wide, kept", "abcdefghij", false, 16, "filter: …defghij"},
		{"wide characters", "漢字漢字漢字", true, 14, "filter: …漢字▏"},
		{"wide characters at an odd width", "漢字漢字漢字", true, 15, "filter: …漢字▏"},
		{"too narrow for the prefix", filterPlatform, true, 6, "filte…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ansi.Strip(filterLine(tt.filter, tt.typing, tt.width, st)); got != tt.want {
				t.Errorf("filterLine(%q, %t, %d) = %q, want %q", tt.filter, tt.typing, tt.width, got, tt.want)
			}
		})
	}

	// At every width the line fits, and the caret shows once the prefix, a
	// cut mark, one character and the caret fit.
	for width := range 30 {
		got := ansi.Strip(filterLine("abcdefghijklmnopqrstuvwxyz", true, width, st))
		if w := ansi.StringWidth(got); w > width {
			t.Errorf("filterLine at %d = %q is %d cells wide", width, got, w)
		}

		if width >= ansi.StringWidth(filterPrefix+"…z"+filterCaret) && !strings.HasSuffix(got, "z"+filterCaret) {
			t.Errorf("filterLine at %d = %q hides the newest character or the caret", width, got)
		}
	}
}

// TestTypingLine checks the status line while a filter is typed: the
// filter, then the notice of the last key in the room left, never at the
// cost of the typed text or the caret.
func TestTypingLine(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	const notice = "notifications unselected"

	st := newStyles(true)

	tests := []struct {
		name, filter, notice string
		width                int
		want                 string
	}{
		{"no notice", filterPlatform, "", 80, filterPrefix + filterPlatform + filterCaret},
		{"a notice", "st", notice, 80, "filter: st▏ · notifications unselected"},
		{"a notice cut to the room left", "st", notice, 30, "filter: st▏ · notifications u…"},
		{"room for the separator alone", "st", notice, 14, "filter: st▏"},
		{"one cell for the notice", "st", notice, 15, "filter: st▏"},
		{"two cells for the notice", "st", notice, 16, "filter: st▏ · n…"},
		{"a filter as wide as the line", "abcdefghij", notice, 17, "filter: …defghij▏"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ansi.Strip(typingLine(tt.filter, tt.notice, tt.width, st)); got != tt.want {
				t.Errorf("typingLine(%q, %q, %d) = %q, want %q", tt.filter, tt.notice, tt.width, got, tt.want)
			}
		})
	}

	// At every width the line fits, starts with the filter line and never
	// ends with a notice cut down to its "…".
	for width := range 50 {
		got := ansi.Strip(typingLine("abcdefghijklmnop", notice, width, st))
		if w := ansi.StringWidth(got); w > width {
			t.Errorf("typingLine at %d = %q is %d cells wide", width, got, w)
		}

		if strings.HasSuffix(got, separator+"…") {
			t.Errorf("typingLine at %d = %q shows a bare … for the notice", width, got)
		}

		if line := ansi.Strip(filterLine("abcdefghijklmnop", true, width, st)); !strings.HasPrefix(got, line) {
			t.Errorf("typingLine at %d = %q does not start with the filter line %q", width, got, line)
		}
	}
}

// TestFilterPaste checks that a paste while a filter is typed adds its first
// line to the filter, sanitized, and that a paste is ignored otherwise: with
// a kept filter, without one, and while the screen is too small to show it.
func TestFilterPaste(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		keys   []string
		small  bool
		paste  string
		filter string
		typing bool
	}{
		{"typing", []string{"/", "s"}, false, "tor\x1b[31mefr\aont\nweb\n", nameStorefront, true},
		{"a carriage return ends the line", []string{"/"}, false, "plat\r\nweb", filterPlatform, true},
		{"an empty first line", []string{"/", "s"}, false, "\nweb", "s", true},
		{"a kept filter", []string{"/", "s", keyEnter}, false, "tor", "s", false},
		{"no filter", nil, false, filterPlatform, "", false},
		{"too small", []string{"/", "s"}, true, "tor", "s", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)
			if m = press(t, m, tt.keys...); tt.small {
				m = step(t, m, tea.WindowSizeMsg{Width: minWidth - 1, Height: 40})
			}

			m = step(t, m, tea.PasteMsg{Content: tt.paste})

			if m.filter != tt.filter || m.filtering != tt.typing {
				t.Errorf("after the paste: filter %q, typing %t; want %q, %t", m.filter, m.filtering, tt.filter, tt.typing)
			}
		})
	}
}

// TestKeptFilterStatusLine checks the status line once a filter is kept:
// the filter, then the status line of the repository under the cursor, with
// its attention reasons and fetch errors, sanitized and cut to the room
// left; the filter alone when no row passes it. While the filter is typed
// the status line shows only the filter.
func TestKeptFilterStatusLine(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	const (
		filterNotifications = "notif"
		// keptNotifications is the status line of the kept filter on
		// notifications, with its fetch error sanitized.
		keptNotifications = "filter: notif · services/platform/notifications · base origin/main (auto) · " +
			"fetch failed: origin (fatal: could not read from remote)"
	)

	snaps := withSnap(fixtureNested(fixedNow), nameNotifications, func(s *drift.Snapshot) {
		s.Fetch.Errors = map[string]string{remoteOrigin: "fatal: could not\a read from remote"}
	})

	tests := []struct {
		name, filter string
		width        int
		keys         []string
		want         string
	}{
		{"kept", filterNotifications, 150, []string{keyEnter}, keptNotifications},
		{"cut to the width", filterNotifications, 60, []string{keyEnter}, "filter: notif · services/platform/notifications · base orig…"},
		{"typed", filterNotifications, 150, nil, "filter: notif▏"},
		{"no row passes", filterNothing, 150, []string{keyEnter}, "filter: zzz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, snaps, tt.width, 40)
			m = press(t, typeFilter(t, m, tt.filter), tt.keys...)

			if got := statusText(m); got != tt.want {
				t.Errorf("status = %q, want %q", got, tt.want)
			}
		})
	}

	// At every width the line fits and starts with the kept filter.
	m, _ := loadedModel(t, snaps, 150, 40)
	m = press(t, typeFilter(t, m, filterNotifications), keyEnter)

	for width := minWidth; width <= 150; width++ {
		got := statusText(step(t, m, tea.WindowSizeMsg{Width: width, Height: 40}))
		if w := ansi.StringWidth(got); w > width {
			t.Errorf("status at %d = %q is %d cells wide", width, got, w)
		}

		if !strings.HasPrefix(got, "filter: notif") {
			t.Errorf("status at %d = %q does not start with the filter", width, got)
		}
	}
}

// TestFilterNoMatches checks the Repos pane of a filter that matches no
// repository, compact and zoomed, with and without folder tabs.
func TestFilterNoMatches(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	tests := []struct {
		name  string
		snaps []drift.Snapshot
		keys  []string
		title string
	}{
		{"nested", fixtureNested(fixedNow), nil, "1  [All 0/7]  services 0/4  web 0/2"},
		{"nested zoomed", fixtureNested(fixedNow), []string{"z"}, "1  [All 0/7]  services 0/4  web 0/2"},
		{"flat", fixtureSnapshots(fixedNow), nil, titleRepos},
		{"flat zoomed", fixtureSnapshots(fixedNow), []string{"z"}, titleReposZoomed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, tt.snaps, 150, 40)
			m = typeFilter(t, press(t, m, tt.keys...), filterNothing)

			lines := paneInside(t, m, PaneRepos)
			if got := strings.TrimSpace(lines[0]); got != noMatchingRepos {
				t.Errorf("first line of the Repos pane = %q, want %q:\n%s", got, noMatchingRepos, strings.Join(lines, "\n"))
			}

			if _, ok := m.cursorRow(); ok {
				t.Errorf("cursor on %q with no visible row", cursorName(m))
			}

			if got := statusText(m); got != "filter: zzz▏" {
				t.Errorf("status = %q, want %q", got, "filter: zzz▏")
			}

			assertViewContains(t, m, "╭─ "+tt.title+" ─")
		})
	}
}

// TestFilterFlatWorkspace checks that the filter works in a workspace
// without folders, whose title and footer stay as they are, and that the
// help lists the filter.
func TestFilterFlatWorkspace(t *testing.T) {
	t.Parallel()

	m, _ := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)
	m = press(t, typeFilter(t, m, "PAY"), keyEnter)

	if got := rowNames(m); !cmp.Equal(got, []string{namePayments}) {
		t.Errorf("rows for PAY = %q, want payments", got)
	}

	assertViewContains(t, m, "╭─ "+titleRepos+" ─",
		"filter: PAY · payments · base origin/main (auto) · diverged · local changes · 27 behind origin/main\n"+footerRepos)
	assertViewContains(t, press(t, m, "?"), "/", "filter repositories", "back, leave zoom or clear filter")
}

// TestViewGoldenFilter renders the nested fixture on a 150x30 screen while
// a filter that matches a folder is typed.
func TestViewGoldenFilter(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 30)
	m = typeFilter(t, moveTo(t, m, namePayments), filterPlatform)

	assertGolden(t, "view_filter.golden", viewText(m)+"\n")
}
