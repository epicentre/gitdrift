package ui

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/repo"
)

// updateGolden reports whether the tests run with -update, which rewrites
// the golden files in testdata. The flag is read by name: teatest links in
// github.com/charmbracelet/x/exp/golden, which defines -update itself, and a
// second definition would panic.
func updateGolden() bool {
	f := flag.Lookup("update")

	return f != nil && f.Value.String() == "true"
}

// Key names used by the tests.
const (
	keyUp       = "up"
	keyPgUp     = "pgup"
	keyPgDown   = "pgdown"
	keyHome     = "home"
	keyEnd      = "end"
	keyDown     = "down"
	keyLeft     = "left"
	keyRight    = "right"
	keyTab      = "tab"
	keyShiftTab = "shift+tab"
	keyEnter    = "enter"
	keyEsc      = "esc"

	// titleCommitsTabs starts the Commits pane title with the Incoming tab
	// active.
	titleCommitsTabs = "2 [Incoming"
)

// eastAsianWidths reports whether RUNEWIDTH_EASTASIAN turns on East Asian
// widths in this process: x/ansi reads it once at init and then draws
// ambiguous-width runes (· – … ↓ ● ▸) two cells wide.
func eastAsianWidths() bool {
	ea, err := strconv.ParseBool(os.Getenv("RUNEWIDTH_EASTASIAN"))

	return err == nil && ea
}

// skipEastAsianWidths skips a test that asserts exact cell widths when
// East Asian widths are on (see eastAsianWidths).
func skipEastAsianWidths(t *testing.T) {
	t.Helper()

	if eastAsianWidths() {
		t.Skip("RUNEWIDTH_EASTASIAN is set: ambiguous-width runes are two cells wide, so exact widths and golden files do not apply")
	}
}

// assertGolden compares got with testdata/name, or rewrites the file with
// -update.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name)

	if updateGolden() {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}

		return
	}

	want, err := os.ReadFile(path) //nolint:gosec // path is built from a constant directory and a test-chosen name
	if err != nil {
		t.Fatalf("read golden file (run go test ./internal/ui -update): %v", err)
	}

	if diff := cmp.Diff(string(want), got); diff != "" {
		t.Errorf("%s mismatch (-want +got):\n%s", name, diff)
	}
}

// newTestModel builds a model over a fake backend serving snaps, sized
// width x height, with the fixture's home directory and clock.
func newTestModel(t *testing.T, snaps []drift.Snapshot, width, height int) (Model, *fakeBackend) {
	t.Helper()

	fake := newFakeBackend(snaps)
	m := New(t.Context(), fake, fixtureRoot)
	m.home = fixtureHome
	m.now = func() time.Time { return fixedNow }

	return step(t, m, tea.WindowSizeMsg{Width: width, Height: height}), fake
}

// loadAll delivers every group's snapshots, as a finished refresh would.
func loadAll(t *testing.T, m Model, fake *fakeBackend) Model {
	t.Helper()

	for _, g := range fake.groups {
		m = step(t, m, groupRefreshedMsg{commonDir: g.CommonDir, snaps: fake.snaps[g.CommonDir]})
	}

	return m
}

func step(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()

	next, _ := m.Update(msg)

	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want ui.Model", next)
	}

	return got
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()

	for _, k := range keys {
		m = step(t, m, keyPress(k))
	}

	return m
}

func keyPress(name string) tea.KeyPressMsg {
	special := map[string]tea.KeyPressMsg{
		keyUp:        {Code: tea.KeyUp},
		keyDown:      {Code: tea.KeyDown},
		keyLeft:      {Code: tea.KeyLeft},
		keyRight:     {Code: tea.KeyRight},
		keyPgUp:      {Code: tea.KeyPgUp},
		keyPgDown:    {Code: tea.KeyPgDown},
		keyHome:      {Code: tea.KeyHome},
		keyEnd:       {Code: tea.KeyEnd},
		keyTab:       {Code: tea.KeyTab},
		keyShiftTab:  {Code: tea.KeyTab, Mod: tea.ModShift},
		keyEnter:     {Code: tea.KeyEnter},
		keyEsc:       {Code: tea.KeyEscape},
		keyBackspace: {Code: tea.KeyBackspace},
		"space":      {Code: tea.KeySpace, Text: " "},
		"ctrl+c":     {Code: 'c', Mod: tea.ModCtrl},
	}

	if k, ok := special[name]; ok {
		return k
	}

	r, _ := utf8.DecodeRuneInString(name)

	return tea.KeyPressMsg{Code: r, Text: name}
}

func viewText(m Model) string { return ansi.Strip(m.View().Content) }

// paneInside returns the lines inside the border of pane p as the view draws
// them, without styling. It indexes cells by rune, so callers skip East Asian
// widths.
func paneInside(t *testing.T, m Model, p Pane) []string {
	t.Helper()

	lay := m.layout()

	r := lay.rect(p)
	if r == nil || r.W < 2 || r.H < 2 {
		t.Fatalf("pane %d is not shown", p)
	}

	screen := strings.Split(viewText(m), "\n")
	out := make([]string, 0, r.H-2)

	for y := r.Y + 1; y < r.Y+r.H-1; y++ {
		runes := []rune(screen[y])
		out = append(out, string(runes[r.X+1:r.X+r.W-1]))
	}

	return out
}

// runeIndex is the cell of the first sub in s at or after cell from, or -1;
// every rune of s is one cell wide.
func runeIndex(s, sub string, from int) int {
	runes := []rune(s)
	if from > len(runes) {
		return -1
	}

	i := strings.Index(string(runes[from:]), sub)
	if i < 0 {
		return -1
	}

	return from + utf8.RuneCountInString(string(runes[from:])[:i])
}

// rowPattern matches a line that shows fields in order, separated by
// spaces only.
func rowPattern(fields ...string) *regexp.Regexp {
	quoted := make([]string, len(fields))
	for i, f := range fields {
		quoted[i] = regexp.QuoteMeta(f)
	}

	return regexp.MustCompile(strings.Join(quoted, " +"))
}

// assertViewHasRow checks that a line of the view shows fields in order,
// separated by spaces only, however wide its columns are.
func assertViewHasRow(t *testing.T, m Model, fields ...string) {
	t.Helper()

	if got := viewText(m); !rowPattern(fields...).MatchString(got) {
		t.Errorf("view has no row with %q:\n%s", fields, got)
	}
}

// rowOf returns the line of lines that contains name as a whole word.
func rowOf(t *testing.T, lines []string, name string) string {
	t.Helper()

	for _, l := range lines {
		if strings.Contains(l+" ", " "+name+" ") || strings.Contains(l, "●"+name+" ") {
			return l
		}
	}

	t.Fatalf("no line shows %s:\n%s", name, strings.Join(lines, "\n"))

	return ""
}

func rowNames(m Model) []string {
	var out []string

	for _, i := range m.visible() {
		out = append(out, m.rows[i].snap.Loc.Name)
	}

	return out
}

func cursorName(m Model) string {
	r, ok := m.cursorRow()
	if !ok {
		return ""
	}

	return r.snap.Loc.Name
}

// runCmd executes cmd and every command batched inside it, returning the
// messages they produce.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()

	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}

	var out []tea.Msg
	for _, c := range batch {
		out = append(out, runCmd(c)...)
	}

	return out
}

func navigationSnapshots() []drift.Snapshot {
	return []drift.Snapshot{
		fixtureAuth(fixedNow),
		behindMain(nameAPIGateway, 12, []string{badgeDeps}, fixedNow.Add(-50*time.Hour)),
		fixturePayments(fixedNow),
	}
}

// loadedNavigationModel is a 120x40 model with the navigation snapshots
// refreshed.
func loadedNavigationModel(t *testing.T) Model {
	t.Helper()

	m, fake := newTestModel(t, navigationSnapshots(), 120, 40)

	return loadAll(t, m, fake)
}

func TestModelNavigation(t *testing.T) {
	t.Parallel()

	m, fake := newTestModel(t, navigationSnapshots(), 120, 40)
	if diff := cmp.Diff([]string{nameAPIGateway, nameAuth, namePayments}, rowNames(m)); diff != "" {
		t.Errorf("rows before refresh mismatch (-want +got):\n%s", diff)
	}

	m = loadAll(t, m, fake)
	if diff := cmp.Diff([]string{namePayments, nameAPIGateway, nameAuth}, rowNames(m)); diff != "" {
		t.Errorf("rows after refresh mismatch (-want +got):\n%s", diff)
	}

	steps := []struct{ key, want string }{
		{"", namePayments},
		{keyDown, nameAPIGateway},
		{"j", nameAuth},
		{keyDown, nameAuth},
		{keyUp, nameAPIGateway},
		{"k", namePayments},
		{keyUp, namePayments},
		{"G", nameAuth},
		{"g", namePayments},
		{keyEnd, nameAuth},
		{keyHome, namePayments},
		{keyPgDown, nameAuth},
		{keyPgUp, namePayments},
	}

	for _, s := range steps {
		if s.key != "" {
			m = press(t, m, s.key)
		}

		if got := cursorName(m); got != s.want {
			t.Errorf("after %q cursor = %q, want %q", s.key, got, s.want)
		}
	}
}

func TestModelNavigationCursorStaysOnTopUntilMoved(t *testing.T) {
	t.Parallel()

	m, fake := newTestModel(t, navigationSnapshots(), 120, 40)
	m = press(t, m, "p", "?", "r")
	m = loadAll(t, m, fake)

	if got := cursorName(m); got != namePayments {
		t.Errorf("cursor = %q, want the first row %q", got, namePayments)
	}
}

func TestModelNavigationCursorFollowsRepo(t *testing.T) {
	t.Parallel()

	m := press(t, loadedNavigationModel(t), "G")

	behind := fixtureAuth(fixedNow)
	behind.Status.Behind = 3
	m = step(t, m, groupRefreshedMsg{commonDir: behind.Loc.CommonDir, snaps: []drift.Snapshot{behind}})

	if diff := cmp.Diff([]string{namePayments, nameAPIGateway, nameAuth}, rowNames(m)); diff != "" {
		t.Errorf("rows mismatch (-want +got):\n%s", diff)
	}

	if got := cursorName(m); got != nameAuth {
		t.Errorf("cursor = %q, want auth", got)
	}
}

func TestModelNavigationFocusAndZoom(t *testing.T) {
	t.Parallel()

	m := loadedNavigationModel(t)

	steps := []struct {
		key   string
		focus Pane
		zoom  bool
	}{
		{keyTab, PaneCommits, false},
		{keyTab, PaneFiles, false},
		{keyTab, PaneDiff, false},
		{keyTab, PaneRepos, false},
		{keyShiftTab, PaneDiff, false},
		{keyShiftTab, PaneFiles, false},
		{"1", PaneRepos, false},
		{keyEnter, PaneCommits, false},
		{keyEnter, PaneFiles, false},
		{keyEnter, PaneDiff, false},
		{keyEnter, PaneDiff, false},
		{"3", PaneFiles, false},
		{"4", PaneDiff, false},
		{"2", PaneCommits, false},
		{"z", PaneCommits, true},
		{keyEsc, PaneCommits, false},
		{keyEsc, PaneRepos, false},
		{keyEsc, PaneRepos, false},
		{"z", PaneRepos, true},
		{"z", PaneRepos, false},
	}

	for i, s := range steps {
		m = press(t, m, s.key)
		if m.focus != s.focus || m.zoom != s.zoom {
			t.Errorf("step %d %q: focus %d zoom %t, want focus %d zoom %t", i, s.key, m.focus, m.zoom, s.focus, s.zoom)
		}
	}
}

func TestModelNavigationOnlyHereFilter(t *testing.T) {
	t.Parallel()

	m := press(t, loadedNavigationModel(t), keyDown, "f")
	if diff := cmp.Diff([]string{namePayments, nameAuth}, rowNames(m)); diff != "" {
		t.Errorf("filtered rows mismatch (-want +got):\n%s", diff)
	}

	if got := cursorName(m); got == "" {
		t.Error("cursor is lost after filtering")
	}

	m = press(t, m, "f")
	if diff := cmp.Diff([]string{namePayments, nameAPIGateway, nameAuth}, rowNames(m)); diff != "" {
		t.Errorf("unfiltered rows mismatch (-want +got):\n%s", diff)
	}
}

func TestModelNavigationQuit(t *testing.T) {
	t.Parallel()

	for _, k := range []string{"q", "ctrl+c"} {
		_, cmd := loadedNavigationModel(t).Update(keyPress(k))
		if cmd == nil {
			t.Fatalf("%q returned no command", k)
		}

		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%q command produced %T, want tea.QuitMsg", k, cmd())
		}
	}
}

func TestModelNarrowDiffFocus(t *testing.T) {
	t.Parallel()

	m, fake := newTestModel(t, navigationSnapshots(), 80, 30)
	m = loadAll(t, m, fake)

	steps := []struct {
		key        string
		focus      Pane
		title, not string
	}{
		{"4", PaneDiff, titleDiff, titleRepos},
		{"z", PaneDiff, titleDiff, titleRepos},
		{keyTab, PaneRepos, titleRepos, "zoomed"},
		{"2", PaneCommits, titleCommitsTabs, titleDiff},
		{"4", PaneDiff, titleDiff, titleCommitsTabs},
		{keyEsc, PaneCommits, titleCommitsTabs, titleDiff},
		{"z", PaneCommits, titleCommitsTabs, titleRepos},
		{keyTab, PaneFiles, titleFiles, titleCommitsTabs},
		{keyTab, PaneDiff, titleDiff, titleFiles},
		{keyEsc, PaneFiles, titleFiles, titleDiff},
		{keyEsc, PaneCommits, titleCommitsTabs, titleDiff},
	}

	for i, s := range steps {
		m = press(t, m, s.key)

		if m.focus != s.focus {
			t.Errorf("step %d %q: focus %d, want %d", i, s.key, m.focus, s.focus)
		}

		if m.focus == PaneDiff && m.zoom {
			t.Errorf("step %d %q: a zoom flag hides behind the narrow Diff", i, s.key)
		}

		got := viewText(m)
		if !strings.Contains(got, s.title) || strings.Contains(got, s.not) {
			t.Errorf("step %d %q: view shows %q=%t %q=%t, want only the first:\n%s",
				i, s.key, s.title, strings.Contains(got, s.title), s.not, strings.Contains(got, s.not), got)
		}
	}
}

// rowState summarizes the rows of a model for comparisons.
func rowState(m Model) []string {
	out := make([]string, 0, len(m.rows))

	for _, r := range m.rows {
		out = append(out, fmt.Sprintf("%s loaded=%t behind=%d", r.snap.Loc.Name, r.loaded, r.snap.Status.Behind))
	}

	return out
}

func TestUpdateLeavesPreviousModelUnchanged(t *testing.T) {
	t.Parallel()

	t.Run("first refresh", func(t *testing.T) {
		t.Parallel()

		m, fake := newTestModel(t, navigationSnapshots(), 120, 40)
		before := rowState(m)

		_ = loadAll(t, m, fake)

		if diff := cmp.Diff(before, rowState(m)); diff != "" {
			t.Errorf("previous model changed (-before +after):\n%s", diff)
		}
	})

	t.Run("later refresh", func(t *testing.T) {
		t.Parallel()

		m := loadedNavigationModel(t)
		before := rowState(m)

		behind := fixtureAuth(fixedNow)
		behind.Status.Behind = 3
		_ = step(t, m, groupRefreshedMsg{commonDir: behind.Loc.CommonDir, snaps: []drift.Snapshot{behind}})

		if diff := cmp.Diff(before, rowState(m)); diff != "" {
			t.Errorf("previous model changed (-before +after):\n%s", diff)
		}
	})
}

func TestHeaderCountsEveryRepo(t *testing.T) {
	t.Parallel()

	m, fake := newTestModel(t, navigationSnapshots(), 120, 40)
	auth := fake.groups[0]
	m = step(t, m, groupRefreshedMsg{commonDir: auth.CommonDir, snaps: fake.snaps[auth.CommonDir]})

	header, _, _ := strings.Cut(viewText(m), "\n")
	if want := "3 repos · 0 behind · 0 need attention · 1 only here"; !strings.HasSuffix(header, want) {
		t.Errorf("header = %q, want suffix %q", header, want)
	}
}

func TestReposPaneScrolls(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, fake := newTestModel(t, headerSnapshots(fixedNow), 100, 24)
	m = loadAll(t, m, fake)

	// 13 inner lines: the header, 11 repositories and … 21 more.
	lines := paneInside(t, m, PaneRepos)
	if !strings.HasPrefix(lines[0], "   REPO ") || !strings.HasPrefix(lines[12], "   … 21 more") {
		t.Errorf("Repos pane does not start with the header and end with … 21 more:\n%s", strings.Join(lines, "\n"))
	}

	m = press(t, m, keyEnd)

	lines = paneInside(t, m, PaneRepos)
	if !strings.HasPrefix(lines[0], "   REPO ") || strings.Contains(strings.Join(lines, "\n"), "more") ||
		!strings.HasPrefix(lines[12], "▸✓ service-26") {
		t.Errorf("after end the header does not stay above the last 12 rows:\n%s", strings.Join(lines, "\n"))
	}

	m = press(t, m, keyHome, "z")
	if got := viewText(m); !strings.Contains(got, "     … 15 more") || !strings.Contains(got, "REPO") {
		t.Errorf("zoom table does not end with … 15 more:\n%s", got)
	}
}

func TestInitRefreshesAllGroups(t *testing.T) {
	t.Parallel()

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 120, 40)

	var refreshed []string

	for _, msg := range runCmd(m.Init()) {
		if r, ok := msg.(groupRefreshedMsg); ok {
			refreshed = append(refreshed, r.commonDir)
		}
	}

	var want, wantDirs []string

	for _, g := range fake.groups {
		want = append(want, "Refresh "+g.CommonDir+" fetch=true")
		wantDirs = append(wantDirs, g.CommonDir)
	}

	got := fake.Calls("Refresh")
	slices.Sort(got)
	slices.Sort(want)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Refresh calls mismatch (-want +got):\n%s", diff)
	}

	slices.Sort(refreshed)
	slices.Sort(wantDirs)

	if diff := cmp.Diff(wantDirs, refreshed); diff != "" {
		t.Errorf("groupRefreshedMsg mismatch (-want +got):\n%s", diff)
	}
}

func TestViewTooSmall(t *testing.T) {
	t.Parallel()

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 59, 15)
	m = loadAll(t, m, fake)

	v := m.View()
	if !v.AltScreen {
		t.Error("View().AltScreen = false, want true")
	}

	if got := ansi.Strip(v.Content); !strings.Contains(got, "terminal too small (need 60×15)") {
		t.Errorf("view = %q, want the too-small notice", got)
	}
}

func TestViewResizeSequence(t *testing.T) {
	t.Parallel()

	m, fake := newTestModel(t, headerSnapshots(fixedNow), 80, 24)
	m = loadAll(t, m, fake)

	sizes := []tea.WindowSizeMsg{
		{Width: 200, Height: 60},
		{Width: 60, Height: 15},
		{Width: 59, Height: 15},
		{Width: 1, Height: 1},
		{Width: 0, Height: 0},
		{Width: 90, Height: 15},
		{Width: 89, Height: 16},
		{Width: 120, Height: 40},
	}

	for _, size := range sizes {
		m = step(t, m, size)

		for _, k := range []string{"G", "z", "2", "z", "4", keyEsc, "z", "z", "1", "g"} {
			m = press(t, m, k)
			_ = m.View()
		}
	}

	got := viewText(m)
	if got == "" || !strings.Contains(got, titleRepos) {
		t.Errorf("final view = %q, want the repos pane", got)
	}
}

// TestViewResizeSweep draws every width from 60 to 200 at sampled heights,
// plus the sizes below the minimum, with the Repos pane compact and zoomed,
// in the All tab and in its folder's tab, filtered to its folder and to no
// row, and with the Commits, Files and Branches lists focused. The
// repository under the cursor, its folder (and so its folder tab and the
// filter) and its lists carry wide text: fullwidth
// letters, East Asian characters and emoji. The view never panics and never
// draws beyond the screen, every line of every pane is exactly the pane's
// inner width, and every Repos row is exactly that wide before the pane pads
// it. Each width runs in its own subtest from the same models, so the race
// detector also checks that resizing never writes to shared state.
func TestViewResizeSweep(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	wide := fixtureWide(fixedNow)
	wide.Loc.Folder = folderWide
	snaps := append(headerSnapshots(fixedNow), fixtureLong(nameLong, branchLong, fixedNow), wide)
	m, fake := newTestModel(t, snaps, 120, 40)
	fake.setDetail(wideDetail(fixedNow))

	base := moveTo(t, loadAll(t, m, fake), nameWide)
	load := base.reload()
	base = settle(t, base, load)

	// Every state with the heights it is drawn at: the Repos pane at many,
	// the detail lists at a short and a tall screen.
	repos := []int{minHeight, minHeight + 1, 24, 40, 59, 60}
	detail := []int{minHeight, 40}
	states := []struct {
		name    string
		m       Model
		heights []int
	}{
		{"compact", base, repos},
		{"zoomed", press(t, base, "z"), repos},
		{"commits", press(t, base, "2"), detail},
		{"files", press(t, base, "3"), detail},
		{"branches", pressSettle(t, base, "2", "["), detail},
		{"folder tab", press(t, base, "]"), repos},
		{"folder tab zoomed", press(t, base, "]", "z"), repos},
		{"filtered", typeFilter(t, base, folderWide), repos},
		{"no match", typeFilter(t, base, filterNothing), repos},
		{"no match zoomed", press(t, typeFilter(t, base, filterNothing), keyEnter, "z"), repos},
	}

	t.Run("below the minimum", func(t *testing.T) {
		t.Parallel()

		for _, st := range states[:2] {
			m := st.m
			for _, size := range []tea.WindowSizeMsg{{Width: 1, Height: 1}, {Width: 0, Height: 0}, {Width: 59, Height: 15}, {Width: 60, Height: 14}} {
				m = step(t, m, size)
				assertViewFits(t, m, size)
			}
		}
	})

	for w := minWidth; w <= 200; w++ {
		t.Run(strconv.Itoa(w), func(t *testing.T) {
			t.Parallel()

			for _, st := range states {
				m := st.m

				for _, h := range st.heights {
					size := tea.WindowSizeMsg{Width: w, Height: h}
					m = step(t, m, size)
					assertViewFits(t, m, size)
				}
			}
		})
	}
}

// assertViewFits checks that the view stays inside size, that every line of
// every pane is exactly the pane's inner width between its borders, that no
// line of the Commits and Files lists is wider than its pane before the pane
// cuts it, and that every Repos row spans its pane before the pane pads it.
func assertViewFits(t *testing.T, m Model, size tea.WindowSizeMsg) {
	t.Helper()

	at := fmt.Sprintf("%dx%d focus=%d zoom=%t", size.Width, size.Height, m.focus, m.zoom)

	// StringWidth skips the styling, so the view is measured as drawn.
	lines := strings.Split(m.View().Content, "\n")
	if len(lines) > max(size.Height, 1) {
		t.Fatalf("%s: view has %d lines", at, len(lines))
	}

	for i, l := range lines {
		if w := ansi.StringWidth(l); w > max(size.Width, ansi.StringWidth(tooSmallNotice)) {
			t.Fatalf("%s: line %d is %d cells wide:\n%s", at, i, w, ansi.Strip(l))
		}
	}

	lay := m.layout()
	if lay.TooSmall {
		return
	}

	assertPanesFit(t, at, lines, lay)
	assertListsFit(t, at, m, lay)
	assertReposHeader(t, at, m, lay)

	inner := lay.Repos.W - 2
	for i, l := range m.reposLines(lay.Repos) {
		w := ansi.StringWidth(l)
		// "… N more" and the line in place of no rows are not padded.
		isNote := strings.Contains(ansi.Strip(l), " more") || len(m.visible()) == 0

		if w > inner || (!isNote && w != inner) {
			t.Fatalf("%s: repos line %d is %d cells wide, want %d:\n%s", at, i, w, inner, ansi.Strip(l))
		}
	}
}

// assertReposHeader checks that the compact header names exactly the
// columns shown, every title whole. A pane without rows has no header.
func assertReposHeader(t *testing.T, at string, m Model, lay Layout) {
	t.Helper()

	if m.tableMode() || !m.reposHasHeader(lay.Repos) || len(m.visible()) == 0 {
		return
	}

	l := newCompactLayout(m.rowsAt(m.visible()), m.folderTab, lay.Repos.W-2-compactGutter, m.styles)

	var want []string

	for i, title := range compactHeader() {
		if l.shown[i] {
			want = append(want, title)
		}
	}

	if got := strings.Fields(ansi.Strip(m.reposLines(lay.Repos)[0])); !slices.Equal(want, got) {
		t.Fatalf("%s: header titles %q, want %q", at, got, want)
	}
}

// assertListsFit checks that no line of the Commits and Files lists is wider
// than its pane before the pane cuts it.
func assertListsFit(t *testing.T, at string, m Model, lay Layout) {
	t.Helper()

	for p, draw := range map[Pane]func(Rect) []string{PaneCommits: m.commitsLines, PaneFiles: m.filesLines} {
		r := lay.rect(p)
		if r.W <= 0 || r.H <= 0 {
			continue
		}

		for i, l := range draw(*r) {
			if w := ansi.StringWidth(l); w > r.W-2 {
				t.Fatalf("%s: line %d of pane %d is %d cells wide, want at most %d:\n%s", at, i, p, w, r.W-2, ansi.Strip(l))
			}
		}
	}
}

// assertPanesFit checks that every line of every pane of lay ends at the
// pane's right border: with each pane's line exactly as wide as the pane,
// the borders sit at the pane's first and last cell.
func assertPanesFit(t *testing.T, at string, lines []string, lay Layout) {
	t.Helper()

	for _, p := range []Pane{PaneRepos, PaneCommits, PaneFiles, PaneDiff} {
		r := lay.rect(p)
		if r.W <= 0 || r.H <= 0 {
			continue
		}

		for y := r.Y; y < r.Y+r.H; y++ {
			cells := cellsOf(lines[y])
			if len(cells) < r.X+r.W || !isBorder(cells[r.X]) || !isBorder(cells[r.X+r.W-1]) {
				t.Fatalf("%s: line %d of pane %d is not %d cells between its borders at %d:\n%s",
					at, y-r.Y, p, r.W-2, r.X, ansi.Strip(lines[y]))
			}
		}
	}
}

// cellsOf splits a drawn line into its cells: the grapheme cluster that
// starts at each cell, and "" for the cells a wide cluster covers after its
// first.
func cellsOf(line string) []string {
	s := ansi.Strip(line)

	var cells []string

	for s != "" {
		c, w := ansi.FirstGraphemeCluster(s, ansi.GraphemeWidth)
		s = s[len(c):]

		if w == 0 {
			if len(cells) > 0 {
				cells[len(cells)-1] += c
			}

			continue
		}

		cells = append(cells, c)
		for range w - 1 {
			cells = append(cells, "")
		}
	}

	return cells
}

// isBorder reports whether cell is a rune of a pane border's side.
func isBorder(cell string) bool {
	switch cell {
	case borderVertical, borderTopLeft, borderTopRight, borderBottomLeft, borderBottomRight:
		return true
	default:
		return false
	}
}

func TestViewGoldenMain(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 100, 24)
	m = press(t, loadAll(t, m, fake), keyDown)

	if got := cursorName(m); got != namePayments {
		t.Fatalf("cursor = %q, want payments", got)
	}

	assertGolden(t, "view_main.golden", viewText(m)+"\n")
}

func TestViewGoldenZoom(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 120, 24)
	m = press(t, loadAll(t, m, fake), keyDown, "z")

	assertGolden(t, "view_zoom.golden", viewText(m)+"\n")
}

func TestHeaderLine(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	worktree := fixturePayments(fixedNow)
	worktree.Loc.Path += "-hotfix"
	worktree.Loc.ID += "-hotfix"

	tests := []struct {
		name         string
		snaps        []drift.Snapshot
		total, width int
		right        string
	}{
		{"spec header", headerSnapshots(fixedNow), 32, 100, "32 repos · 6 behind · 2 need attention · 3 only here"},
		{"fixture", fixtureSnapshots(fixedNow), 6, 100, "6 repos · 3 behind · 2 need attention · 2 only here"},
		{
			"repos not read yet count only in the total",
			fixtureSnapshots(fixedNow), 32, 100,
			"32 repos · 3 behind · 2 need attention · 2 only here",
		},
		{"one repo", []drift.Snapshot{fixtureAuth(fixedNow)}, 1, 100, "1 repo · 0 behind · 0 need attention · 1 only here"},
		{
			"worktrees of one repository count once for only here",
			[]drift.Snapshot{fixturePayments(fixedNow), worktree},
			2, 100,
			"2 repos · 2 behind · 2 need attention · 1 only here",
		},
		{"none read yet", nil, 3, 100, "3 repos · 0 behind · 0 need attention · 0 only here"},
		{"no repos", nil, 0, 100, "0 repos · 0 behind · 0 need attention · 0 only here"},
		{"narrow", headerSnapshots(fixedNow), 32, 60, "32 repos · 6 behind · 2 need attention · 3 only here"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ansi.Strip(headerLine("~/code/work", tt.snaps, tt.total, tt.width, newStyles(true)))
			if !strings.HasSuffix(got, tt.right) {
				t.Errorf("headerLine() = %q, want suffix %q", got, tt.right)
			}

			if !strings.HasPrefix(got, "gitd") {
				t.Errorf("headerLine() = %q, want it to start with the program name", got)
			}

			if w := ansi.StringWidth(got); w != tt.width {
				t.Errorf("headerLine() width = %d, want %d", w, tt.width)
			}
		})
	}

	got := ansi.Strip(headerLine("~/code/work", nil, 0, 100, newStyles(true)))
	if !strings.HasPrefix(got, "gitdrift  ~/code/work ") {
		t.Errorf("headerLine() = %q, want prefix %q", got, "gitdrift  ~/code/work ")
	}
}

func TestTildePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, path, home, want string
	}{
		{"inside home", fixtureRoot, fixtureHome, "~/code/work"},
		{"home itself", fixtureHome, fixtureHome, "~"},
		{"sibling with the same prefix", "/home/devops/work", fixtureHome, "/home/devops/work"},
		{"outside home", "/srv/work", fixtureHome, "/srv/work"},
		{"unknown home", "/home/dev/work", "", "/home/dev/work"},
		{"root as home", "/work", "/", "/work"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tildePath(tt.path, tt.home); got != tt.want {
				t.Errorf("tildePath(%q, %q) = %q, want %q", tt.path, tt.home, got, tt.want)
			}
		})
	}
}

func TestStatusLine(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	noBase := tracking("scratch", branchMain, fixedNow)
	noBase.Base = repo.Base{}

	hostile := fixtureAuth(fixedNow)
	hostile.Loc.Name = "au\x1b[2Jth\n"
	hostile.Base.Ref = "origin/\x1b]0;x\x07main"

	fetchFailed := fixtureAuth(fixedNow)
	fetchFailed.Fetch.Errors = map[string]string{
		"origin": "fatal: unable to access 'https://example.com/auth.git/': Could not resolve host\x1b[2J: example.com",
		"backup": "fatal: 'backup' does not appear to be a git repository",
	}

	tests := []struct {
		name   string
		snap   drift.Snapshot
		loaded bool
		width  int
		want   string
	}{
		{
			"needs attention", fixturePayments(fixedNow), true, 0,
			"payments · base origin/main (auto) · diverged · local changes · 27 behind origin/main",
		},
		{"up to date", fixtureAuth(fixedNow), true, 0, "auth · base origin/main (auto)"},
		{"no base", noBase, true, 0, "scratch · no base"},
		{"sanitized", hostile, true, 0, "auth · base origin/main (auto)"},
		{"unloaded", drift.Snapshot{Loc: fixtureLoc(namePayments)}, false, 0, "payments · fetching…"},
		{
			"fetch error messages", fetchFailed, true, 240,
			"auth · base origin/main (auto) · fetch failed: backup (fatal: 'backup' does not appear to be a git repository) · " +
				"fetch failed: origin (fatal: unable to access 'https://example.com/auth.git/': Could not resolve host: example.com)",
		},
		{
			"fetch error message cut to the width", fetchFailed, true, 80,
			"auth · base origin/main (auto) · fetch failed: backup (fatal: 'backup' does not…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			width := tt.width
			if width == 0 {
				width = 120
			}

			if got := ansi.Strip(statusLine(tt.snap, tt.loaded, token{}, width, newStyles(true))); got != tt.want {
				t.Errorf("statusLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFooterLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		focus         Pane
		zoom, folders bool
		tab           Tab
		want          string
	}{
		{
			"repos", PaneRepos, false, false, TabBranches,
			"space select · p pull · P pull all safe · r/R refresh · b base · z zoom · tab pane · ? help",
		},
		{
			"repos zoomed", PaneRepos, true, false, TabIncoming,
			"space select · p pull · P pull all safe · f only-here filter · z unzoom · ? help",
		},
		{
			"repos with folders", PaneRepos, false, true, TabIncoming,
			"space select · p pull · P pull all safe · ←→ folders · / filter · r/R refresh · b base · z zoom · ? help",
		},
		{
			"repos zoomed with folders", PaneRepos, true, true, TabBranches,
			"space select · p pull · P pull all safe · f only-here filter · ←→ folders · / filter · z unzoom · ? help",
		},
		{"commits", PaneCommits, false, false, TabIncoming, "←→ tabs · enter files · z zoom · esc back · ? help"},
		{"commits base with folders", PaneCommits, true, true, TabBase, "←→ tabs · enter files · z zoom · esc back · ? help"},
		{"commits local", PaneCommits, false, false, TabLocal, "←→ tabs · enter files · z zoom · esc back · ? help"},
		{
			"commits branches", PaneCommits, false, true, TabBranches,
			"space select · d delete gone · ←→ tabs · z zoom · esc back · ? help",
		},
		{"files", PaneFiles, true, true, TabBranches, "enter diff · ←→ tabs · z zoom · esc back · ? help"},
		{"diff", PaneDiff, false, true, TabIncoming, "↑↓ scroll · pgup/pgdn page · z zoom · esc back · ? help"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ansi.Strip(footerLine(tt.focus, tt.zoom, tt.folders, tt.tab, 120, newStyles(true))); got != tt.want {
				t.Errorf("footerLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKeyMapHelp(t *testing.T) {
	t.Parallel()

	var keys []string

	for _, group := range defaultKeyMap().FullHelp() {
		for _, b := range group {
			if !b.Enabled() || b.Help().Key == "" || b.Help().Desc == "" {
				t.Errorf("binding %v is disabled or has no help text", b.Keys())
			}

			keys = append(keys, b.Keys()...)
		}
	}

	want := []string{
		"up", "k", "down", "j", "pgup", "pgdown", "home", "g", "end", "G", "tab", "shift+tab",
		"1", "2", "3", "4", "enter", "esc", "left", "h", "[", "right", "l", "]", "z", "space", "p", "P", "r", "R", "b", "d", "f", "/", "?",
		"q", "ctrl+c",
	}

	slices.Sort(keys)
	slices.Sort(want)

	if diff := cmp.Diff(want, keys); diff != "" {
		t.Errorf("bound keys mismatch (-want +got):\n%s", diff)
	}
}
