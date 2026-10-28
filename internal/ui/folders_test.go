package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/gittest"
	"github.com/epicentre/gitdrift/internal/workspace"
)

// Pane titles of the nested fixture's folder tabs, in the All, services and
// web tabs.
const (
	titleTabAll      = "1  [All 7]  services 4  web 2"
	titleTabServices = "1  All 7  [services 4]  web 2"
	titleTabWeb      = "1  All 7  services 4  [web 2]"
)

func TestTopFolder(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, folder, want string }{
		{"the root", "", ""},
		{"a top-level folder", folderServices, folderServices},
		{"a subfolder", folderPlatform, folderServices},
		{"deeper", "backend/x/y", "backend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := topFolder(tt.folder); got != tt.want {
				t.Errorf("topFolder(%q) = %q, want %q", tt.folder, got, tt.want)
			}
		})
	}
}

func TestFolderTabs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		snaps []drift.Snapshot
		want  []string
	}{
		{"nested fixture", fixtureNested(fixedNow), []string{folderServices, folderWeb}},
		{"flat fixture", fixtureSnapshots(fixedNow), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// cmp tells a nil slice from an empty one.
			if diff := cmp.Diff(tt.want, folderTabs(loadedRows(tt.snaps...))); diff != "" {
				t.Errorf("folderTabs() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReposTabTitle(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	// rawTab is a folder name with escape sequences, which a tab key keeps.
	const rawTab = "ser\x1b[31mvices\a"

	tabs := []string{folderServices, folderWeb}
	whole := map[string][2]int{"": {7, 7}, folderServices: {4, 4}, folderWeb: {2, 2}}
	narrowed := map[string][2]int{"": {3, 7}, folderServices: {1, 4}, folderWeb: {2, 2}}

	tests := []struct {
		name   string
		tabs   []string
		active string
		counts map[string][2]int
		width  int
		want   string
	}{
		{"full width", tabs, "", whole, 80, titleTabAll},
		{"exactly as wide", tabs, "", whole, 29, titleTabAll},
		{"a folder tab active", tabs, folderServices, whole, 80, titleTabServices},
		{"narrow drops the counts of inactive tabs", tabs, "", whole, 28, "1  [All 7]  services  web"},
		{"narrow keeps the count of the active tab", tabs, folderWeb, whole, 28, "1  All  services  [web 2]"},
		{"very narrow cuts from the right", tabs, "", whole, 15, "1  [All 7]  se…"},
		{"very narrow keeps a middle active tab", tabs, folderServices, whole, 22, "1  All  [services 4] …"},
		{"a cut keeps a space after the active tab", tabs, folderServices, whole, 21, "1  …  [services 4]  …"},
		{"no cut without room for that space", tabs, "", whole, 11, "1  [All 7]"},
		{"very narrow hides the tabs before the active one", tabs, folderWeb, whole, 15, "1  …  [web 2]"},
		{"too narrow for the hidden tabs mark", tabs, folderWeb, whole, 11, "1  [web 2]"},
		{"too narrow for the active tab", tabs, folderWeb, whole, 8, "1  [web…"},
		{"f narrows the tabs", tabs, "", narrowed, 80, "1  [All 3/7]  services 1/4  web 2"},
		{"narrow drops the narrowed counts", tabs, "", narrowed, 32, "1  [All 3/7]  services  web"},
		{"tab names are sanitized for display only", []string{rawTab}, rawTab, map[string][2]int{
			"": {2, 2}, rawTab: {1, 1},
		}, 80, "1  All 2  [services 1]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := reposTabTitle(tt.tabs, tt.active, tt.counts, tt.width)
			if got != tt.want {
				t.Errorf("reposTabTitle(%q, %d) = %q, want %q", tt.active, tt.width, got, tt.want)
			}

			if w := ansi.StringWidth(got); w > tt.width {
				t.Errorf("reposTabTitle(%q, %d) is %d cells wide", tt.active, tt.width, w)
			}
		})
	}

	// At every width the title fits, and the active tab shows whole as
	// soon as the pane key and the tab alone fit.
	active := map[string]string{"": "[All 7]", folderServices: "[services 4]", folderWeb: "[web 2]"}
	for tab, label := range active {
		for width := range 40 {
			got := reposTabTitle(tabs, tab, whole, width)
			if w := ansi.StringWidth(got); w > width {
				t.Errorf("reposTabTitle(%q, %d) = %q is %d cells wide", tab, width, got, w)
			}

			if width >= ansi.StringWidth("1  "+label) && !strings.Contains(got, label) {
				t.Errorf("reposTabTitle(%q, %d) = %q hides the active tab %s", tab, width, got, label)
			}
		}
	}
}

// reposPane returns the column titles and the rows of the Repos pane, and
// the relative paths of the rows the Repos pane shows.
func reposPane(t *testing.T, m Model) (header string, lines, shown []string) {
	t.Helper()

	inside := paneInside(t, m, PaneRepos)

	return inside[0], inside[1:], relativePaths(m.rowsAt(m.visible()))
}

// TestFolderTabKeys walks the folder tabs of the nested fixture with →, l
// and ] forward and ←, h and [ back: each tab shows its repositories, FOLDER relative to the tab, and the
// cursor stays on its repository while the tab shows it, else goes to the
// first row; the detail panes follow the cursor and the header keeps
// counting the whole workspace.
func TestFolderTabKeys(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, fake := loadedModel(t, fixtureNested(fixedNow), 150, 40)
	m = moveTo(t, m, namePayments)

	all := nestedOrder()
	services := []string{
		"services/billing", "services/payments", "services/platform/notifications", "services/platform/search",
	}
	web := []string{"web/storefront", "web/admin"}

	steps := []struct {
		key, tab, title string
		shown           []string
		cursor          string
	}{
		{keyRight, folderServices, titleTabServices, services, namePayments},
		{"l", folderWeb, titleTabWeb, web, nameStorefront},
		{"]", "", titleTabAll, all, nameStorefront},
		{keyLeft, folderWeb, titleTabWeb, web, nameStorefront},
		{"h", folderServices, titleTabServices, services, "billing"},
		{"[", "", titleTabAll, all, "billing"},
		{keyLeft, folderWeb, titleTabWeb, web, nameStorefront},
	}

	for i, s := range steps {
		m = pressSettle(t, m, s.key)

		_, _, shown := reposPane(t, m)
		if m.folderTab != s.tab || cursorName(m) != s.cursor {
			t.Errorf("step %d %q: tab %q, cursor on %q; want %q, %q", i, s.key, m.folderTab, cursorName(m), s.tab, s.cursor)
		}

		if diff := cmp.Diff(s.shown, shown); diff != "" {
			t.Errorf("step %d %q: rows mismatch (-want +got):\n%s", i, s.key, diff)
		}

		assertViewContains(t, m, "╭─ "+s.title+" ─", "7 repos · ")
	}

	if calls := fake.Calls("RangeCommits " + nameStorefront + " "); len(calls) == 0 {
		t.Error("the detail panes did not load storefront, the cursor's repository in the web tab")
	}

	m = pressSettle(t, m, "[")

	header, lines, _ := reposPane(t, m)
	for name, want := range map[string]string{namePayments: "", nameNotifications: platformInServices, "search": platformInServices} {
		if got := cellUnder(t, header, rowOf(t, lines, name), titleFolder); got != want {
			t.Errorf("FOLDER of %s in the services tab = %q, want %q:\n%s\n%s", name, got, want, header, strings.Join(lines, "\n"))
		}
	}
}

// TestFolderTabKeysInZoom checks that ] and [ switch the folder tabs of the
// zoomed Repos pane, whose title then shows the tabs, and that the zoom
// table's FOLDER is relative to the tab.
func TestFolderTabKeysInZoom(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)
	m = pressSettle(t, m, "z", "]")

	if m.folderTab != folderServices || !m.tableMode() {
		t.Fatalf("tab %q, table mode %t; want services, zoomed", m.folderTab, m.tableMode())
	}

	assertViewContains(t, m, "╭─ "+titleTabServices+" ─")

	header, lines, _ := reposPane(t, m)
	if got := cellUnder(t, header, rowOf(t, lines, nameNotifications), titleFolder); got != platformInServices {
		t.Errorf("FOLDER of notifications in the zoomed services tab = %q, want %q:\n%s\n%s",
			got, platformInServices, header, strings.Join(lines, "\n"))
	}

	if m = pressSettle(t, m, "["); m.folderTab != "" {
		t.Errorf("[ in the zoomed services tab went to %q, want All", m.folderTab)
	}
}

// TestFolderTabCountsWithOnlyHere checks that f narrows each tab's count to
// visible/total and that a tab shows only the rows f leaves.
func TestFolderTabCountsWithOnlyHere(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)
	m = pressSettle(t, m, "f")

	assertViewContains(t, m, "╭─ 1  [All 2/7]  services 2/4  web 0/2 ─", "7 repos · ")

	m = pressSettle(t, m, "]")
	if _, _, shown := reposPane(t, m); !cmp.Equal(shown, []string{"services/payments", "services/platform/search"}) {
		t.Errorf("services tab with f shows %q, want payments and search", shown)
	}
}

// TestFolderTabLeavesOutSiblingPrefix checks that the web tab leaves out
// web-legacy, whose name starts with web, and that web-legacy has a tab of
// its own with FOLDER relative to it.
func TestFolderTabLeavesOutSiblingPrefix(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	const legacy = "web-legacy"

	snaps := append(fixtureNested(fixedNow),
		relocated(legacy, "shop", tracking("shop", branchMain, fixedNow)),
		relocated(legacy+"/old", "kiosk", tracking("kiosk", branchMain, fixedNow)),
	)
	m, _ := loadedModel(t, snaps, 150, 40)

	if diff := cmp.Diff([]string{folderServices, folderWeb, legacy}, folderTabs(m.rows)); diff != "" {
		t.Fatalf("folderTabs() mismatch (-want +got):\n%s", diff)
	}

	m = pressSettle(t, m, "]", "]")
	assertViewContains(t, m, "╭─ 1  All 9  services 4  [web 2]  web-legacy 2 ─")

	if _, _, shown := reposPane(t, m); !cmp.Equal(shown, []string{"web/storefront", "web/admin"}) {
		t.Errorf("web tab shows %q, want storefront and admin only", shown)
	}

	m = pressSettle(t, m, "]")

	header, lines, shown := reposPane(t, m)
	if !cmp.Equal(shown, []string{"web-legacy/shop", "web-legacy/old/kiosk"}) {
		t.Errorf("web-legacy tab shows %q, want shop and kiosk", shown)
	}

	for name, want := range map[string]string{"shop": "", "kiosk": "old"} {
		if got := cellUnder(t, header, rowOf(t, lines, name), titleFolder); got != want {
			t.Errorf("FOLDER of %s in the web-legacy tab = %q, want %q:\n%s\n%s", name, got, want, header, strings.Join(lines, "\n"))
		}
	}
}

// TestFolderTabScrolls checks a tab with more repositories than its pane
// holds: a tab switch starts at the top, the list scrolls with the cursor,
// and "… N more" counts only the tab's rows below the window.
func TestFolderTabScrolls(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	const services, web = 20, 15

	var snaps []drift.Snapshot

	for i := range services + web {
		folder, name := folderServices, fmt.Sprintf("svc-%02d", i+1)
		if i >= services {
			folder, name = folderWeb, fmt.Sprintf("app-%02d", i-services+1)
		}

		snaps = append(snaps, relocated(folder, name, tracking(name, branchMain, fixedNow)))
	}

	m, _ := loadedModel(t, snaps, 100, 24)

	capacity := m.reposCapacity(m.layout().Repos)
	if capacity >= web {
		t.Fatalf("the Repos pane holds %d rows, want fewer than a tab", capacity)
	}

	steps := []struct {
		name   string
		keys   []string
		cursor string
		more   int
	}{
		{"services after the end of All", []string{keyEnd, "]"}, "svc-01", services - (capacity - 1)},
		{"end of services", []string{keyEnd}, "svc-20", 0},
		{"web", []string{"]"}, "app-01", web - (capacity - 1)},
		{"end of web", []string{keyEnd}, "app-15", 0},
	}

	for _, s := range steps {
		m = press(t, m, s.keys...)
		lines := paneInside(t, m, PaneRepos)

		if row := rowOf(t, lines, s.cursor); !strings.HasPrefix(row, cursorMark) {
			t.Errorf("%s: cursor not on %s:\n%s", s.name, s.cursor, strings.Join(lines, "\n"))
		}

		text := strings.Join(lines, "\n")
		switch want := fmt.Sprintf("… %d more", s.more); {
		case s.more > 0 && !strings.Contains(text, want):
			t.Errorf("%s: no %q:\n%s", s.name, want, text)
		case s.more == 0 && strings.Contains(text, " more"):
			t.Errorf("%s: a more line at the end of the list:\n%s", s.name, text)
		}
	}
}

// TestCursorKeptThroughRefresh checks that a folder tab switch or f that
// keeps the cursor on its repository counts as moving the cursor: a later
// refresh that re-sorts the repository keeps the cursor on it, not on its
// old row.
func TestCursorKeptThroughRefresh(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		keys   []string
		cursor string
		// refreshed is the cursor's repository up to date, which sorts it
		// further down.
		refreshed drift.Snapshot
	}{
		{
			"folder tabs",
			[]string{"]", "]", "]"},
			nameStorefront,
			relocated(folderWeb, nameStorefront, tracking(nameStorefront, branchMain, fixedNow)),
		},
		{
			"f on and off",
			[]string{"f", "f"},
			namePayments,
			relocated(folderServices, namePayments, tracking(namePayments, branchMain, fixedNow)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)

			m = press(t, m, tt.keys...)
			if got := cursorName(m); got != tt.cursor || m.folderTab != "" || m.onlyHere {
				t.Fatalf("after %q: cursor on %q, tab %q, only here %t; want %q in All", tt.keys, got, m.folderTab, m.onlyHere, tt.cursor)
			}

			before := m.cursor
			m = step(t, m, groupRefreshedMsg{commonDir: tt.refreshed.Loc.CommonDir, snaps: []drift.Snapshot{tt.refreshed}})

			if got := cursorName(m); got != tt.cursor || m.cursor == before {
				t.Errorf("after the refresh: cursor on %q at row %d (before %d), want %q on its new row", got, m.cursor, before, tt.cursor)
			}
		})
	}
}

// TestBracketsSwitchCommitTabsInCommits checks that ] and [ switch the
// Commits tabs, not the folder tabs, when the Commits, Files or Diff pane
// has the focus.
func TestBracketsSwitchCommitTabsInCommits(t *testing.T) {
	t.Parallel()

	for _, focus := range []string{"2", "3", "4"} {
		t.Run(focus, func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)

			m = press(t, m, focus, "]")
			if m.detail.tab != TabBase || m.folderTab != "" {
				t.Errorf("] in pane %s: commits tab %d, folder tab %q; want %d, All", focus, m.detail.tab, m.folderTab, TabBase)
			}

			m = press(t, m, "[", "[")
			if m.detail.tab != TabBranches || m.folderTab != "" {
				t.Errorf("[ [ in pane %s: commits tab %d, folder tab %q; want %d, All", focus, m.detail.tab, m.folderTab, TabBranches)
			}
		})
	}
}

// TestFlatWorkspaceHasNoTabs checks that a workspace without folders has no
// folder tabs: the Repos pane keeps its title and ] and [ switch the Commits
// tabs from the Repos pane as before. view_main.golden pins the screen.
func TestFlatWorkspaceHasNoTabs(t *testing.T) {
	t.Parallel()

	m, _ := loadedModel(t, fixtureSnapshots(fixedNow), 100, 24)

	if tabs := folderTabs(m.rows); tabs != nil {
		t.Errorf("folderTabs() = %q, want none", tabs)
	}

	assertViewContains(t, m, "╭─ "+titleRepos+" ─")
	assertViewContains(t, press(t, m, "z"), "╭─ "+titleReposZoomed+" ─")

	m = press(t, m, "]")
	if m.focus != PaneRepos || m.detail.tab != TabBase || m.folderTab != "" {
		t.Errorf("] in Repos: focus %d, commits tab %d, folder tab %q; want Repos, %d, All", m.focus, m.detail.tab, m.folderTab, TabBase)
	}

	if m = press(t, m, "["); m.detail.tab != TabIncoming {
		t.Errorf("[ in Repos: commits tab %d, want %d", m.detail.tab, TabIncoming)
	}
}

// TestViewGoldenNested renders the nested fixture on a 150x30 screen, wide
// enough for the FOLDER column, in the All and the services tab.
func TestViewGoldenNested(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	tests := []struct {
		golden string
		keys   []string
	}{
		{"view_main_nested.golden", nil},
		{"view_tab_services.golden", []string{"]"}},
	}

	for _, tt := range tests {
		t.Run(tt.golden, func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 30)
			m = press(t, moveTo(t, m, namePayments), tt.keys...)

			assertGolden(t, tt.golden, viewText(m)+"\n")
		})
	}
}

// TestDiscoveredFoldersInReposPane feeds a real nested tree, found by
// workspace.Discover and read by the workspace, into the model: the Repos
// title has a tab for each top-level folder, FOLDER shows each folder below
// the workspace, and below the tab once one is chosen.
func TestDiscoveredFoldersInReposPane(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	env := gittest.New(t)
	for _, name := range []string{"handbook", "services/api", "services/platform/billing", "web/api"} {
		env.Init(name).Commit("README.md", "hello\n", "initial")
	}

	root := env.WorkDir()

	groups, err := workspace.Discover(t.Context(), env.Runner(), root, workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	ws := workspace.New(env.Runner(), groups, workspace.DefaultOptions())
	t.Cleanup(ws.Close)

	m := step(t, New(t.Context(), ws, root), tea.WindowSizeMsg{Width: 150, Height: 40})
	for _, g := range groups {
		m = step(t, m, groupRefreshedMsg{commonDir: g.CommonDir, snaps: ws.Refresh(t.Context(), g.CommonDir, false)})
	}

	assertViewContains(t, m, "╭─ 1  [All 4]  services 2  web 1 ─")
	assertViewHasRow(t, m, "api", folderServices, branchMain)
	assertViewHasRow(t, m, "billing", folderPlatform, branchMain)
	assertViewHasRow(t, m, "api", folderWeb, branchMain)
	assertViewHasRow(t, m, "handbook", branchMain)

	m = press(t, m, "]")
	assertViewContains(t, m, "╭─ 1  All 4  [services 2]  web 1 ─")
	assertViewHasRow(t, m, "billing", platformInServices, branchMain)
}
