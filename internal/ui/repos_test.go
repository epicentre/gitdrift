package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/repo"
)

// framed wraps a rendered line in bars so that golden files keep its
// trailing spaces visible.
func framed(line string) string { return "|" + ansi.Strip(line) + "|\n" }

func sortedFixture() []drift.Snapshot {
	snaps := fixtureSnapshots(fixedNow)
	drift.Sort(snaps)

	return snaps
}

// Test values of the Repos rows: a repository name, the kinds of compact
// tokens compactTokens lists, and zoom table titles.
const (
	nameWeb       = "web"
	kindLocal     = "local"
	kindUpstream  = "upstream"
	kindBase      = "base"
	kindBadge     = "badge"
	kindHere      = "here"
	kindGone      = "gone"
	titleGone     = "GONE"
	titleUpdated  = "UPDATED"
	titleUpstream = "UPSTREAM"
	titleFolder   = "FOLDER"
	titleRepo     = "REPO"
	titleBranch   = "BRANCH"
	titleLocal    = "LOCAL"
	titleHere     = "HERE"
	titleBase     = "BASE"
	// platformCut is services/platform cut at a folder boundary, as narrow
	// as the FOLDER minimum.
	platformCut = "…/platform"
)

// nestedRows is the nested fixture, read, in the order of the Repos pane.
func nestedRows() []row {
	rows := loadedRows(fixtureNested(fixedNow)...)
	sortRows(rows)

	return rows
}

// nestedOrder lists the repositories of the nested fixture, relative to
// fixtureRoot, in the order of the Repos pane.
func nestedOrder() []string {
	return []string{
		"services/billing", "services/payments", "services/platform/notifications", "web/storefront",
		"docs", "services/platform/search", "web/admin",
	}
}

// relativePaths lists the paths of rows relative to fixtureRoot, so that a
// list shows both the folder and the name of each row.
func relativePaths(rows []row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = strings.TrimPrefix(r.snap.Loc.Path, fixtureRoot+"/")
	}

	return out
}

// loadedRows wraps snaps as rows read by a refresh.
func loadedRows(snaps ...drift.Snapshot) []row {
	rows := make([]row, len(snaps))
	for i, s := range snaps {
		rows[i] = row{snap: s, loaded: true, category: drift.CategoryOf(s)}
	}

	return rows
}

// compactRow renders s alone as a compact row of width cells.
func compactRow(s drift.Snapshot, loaded, selected bool, width int) string {
	st := newStyles(true)

	return repoRow(s, loaded, "", selected, token{}, "", newCompactLayout([]row{{snap: s, loaded: loaded}}, "", width, st), st)
}

// compactTokens lists by kind the status tokens of the fixture rows and of
// fixtureLong called long: local changes, upstream, base, first badge, only
// here and gone.
func compactTokens(long string) map[string]map[string]string {
	return map[string]map[string]string{
		"billing": {kindUpstream: "ok", kindBase: "base↓27"},
		namePayments: {
			kindLocal: "*2", kindUpstream: "↑3↓12", kindBase: "base↓27", kindBadge: badgeDeps, kindHere: "⌂6", kindGone: "✂2",
		},
		nameAPIGateway:    {kindUpstream: "↓12", kindBadge: badgeDeps},
		nameNotifications: {kindUpstream: "↓4", kindBadge: badgeMig},
		nameAuth:          {kindUpstream: "ok", kindHere: "⌂1"},
		"infra":           {kindUpstream: "ok", kindGone: "✂3"},
		long: {
			kindLocal: "*11", kindUpstream: "↓2", kindBase: "base↓3", kindBadge: badgeDeps, kindHere: "⌂4", kindGone: "✂1",
		},
	}
}

// compactColumnOf maps the kinds of compactTokens to their columns.
var compactColumnOf = map[string]int{
	kindLocal: compactLocal, kindUpstream: compactUpstream, kindBase: compactBase,
	kindBadge: compactIncoming, kindHere: compactHere, kindGone: compactGone,
}

// assertCompactColumns checks compact rows under their header: every name
// and branch of snaps in full, every kind of token at the column where its
// title starts on every row, and the header, the last title, ending at the
// right edge.
func assertCompactColumns(t *testing.T, header string, lines []string, snaps []drift.Snapshot, tokens map[string]map[string]string) {
	t.Helper()

	titles := compactHeader()
	if !strings.HasSuffix(header, titles[compactGone]) {
		t.Errorf("header %q does not end with %s at the right edge", header, titles[compactGone])
	}

	for _, s := range snaps {
		row := rowOf(t, lines, s.Loc.Name)

		branch := runeIndex(row, " "+s.Status.Branch+" ", 0)
		if branch < 0 {
			t.Errorf("row %q does not show the branch %s in full", row, s.Status.Branch)

			continue
		}

		from := branch + utf8.RuneCountInString(s.Status.Branch) + 1

		for kind, text := range tokens[s.Loc.Name] {
			title := titles[compactColumnOf[kind]]

			want := runeIndex(header, " "+title, 0) + 1
			if i := runeIndex(row, " "+text, from-1) + 1; i <= 0 || i != want {
				t.Errorf("%s token %s of %s starts at column %d, want %d under %s:\n%s\n%s",
					kind, text, s.Loc.Name, i, want, title, header, strings.Join(lines, "\n"))
			}
		}
	}
}

// TestRepoRows renders the fixture with a 28-character repository on a
// 30-character branch and a row not read yet, wide and at 44 cells.
func TestRepoRows(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	st := newStyles(true)

	snaps := append(sortedFixture(), fixtureLong(nameLong, branchLong, fixedNow))
	rows := append(loadedRows(snaps...), row{snap: drift.Snapshot{Loc: fixtureLoc("ledger")}})

	var b strings.Builder

	lines := map[int][]string{}
	headers := map[int]string{}

	for _, width := range []int{120, 44} {
		l := newCompactLayout(rows, "", width, st)
		headers[width] = ansi.Strip(repoHeader(l, st))

		b.WriteString("width " + strconv.Itoa(width) + "\n")
		b.WriteString(framed(repoHeader(l, st)))

		for _, r := range rows {
			line := repoRow(r.snap, r.loaded, "", false, token{}, "", l, st)
			if got := ansi.StringWidth(line); got != width {
				t.Errorf("repoRow(%s) at %d width = %d, want %d", r.snap.Loc.Name, width, got, width)
			}

			b.WriteString(framed(line))
			lines[width] = append(lines[width], ansi.Strip(line))
		}
	}

	assertGolden(t, "repo_rows.golden", b.String())

	assertCompactColumns(t, headers[120], lines[120], snaps, compactTokens(nameLong))

	// At 44 cells the branches are cut down to 10 cells, then the names, and
	// the status columns go from the right, titles and all, until the local
	// and upstream ones fit, each at least as wide as its title.
	if want := "  REPO             BRANCH     LOCAL UPSTREAM"; headers[44] != want {
		t.Errorf("header at 44 cells = %q, want %q", headers[44], want)
	}

	for _, want := range []string{
		"! settlement-reco… feature/m… *11   ↓2      ",
		"! payments         feature/c… *2    ↑3↓12   ",
		"! billing          feature/v…       ok      ",
		"  ledger" + strings.Repeat(" ", 36),
	} {
		if !slices.Contains(lines[44], want) {
			t.Errorf("rows at 44 cells lack %q:\n%s", want, strings.Join(lines[44], "\n"))
		}
	}

	for _, dropped := range []string{"base↓", badgeDeps, "⌂", "✂"} {
		if narrow := strings.Join(lines[44], "\n"); strings.Contains(narrow, dropped) {
			t.Errorf("rows at 44 cells still show %q:\n%s", dropped, narrow)
		}
	}
}

// TestBusyRowShowsSpinnerAndMutedTokens checks a row whose fetch runs, in the
// compact rows and the zoom table: the spinner takes the place of its
// marker, and its tokens stay, drawn muted.
func TestBusyRowShowsSpinnerAndMutedTokens(t *testing.T) {
	t.Parallel()

	st := newStyles(true)
	s := behindMain(nameAPIGateway, 12, []string{badgeDeps}, fixedNow)
	rows := []row{{snap: s, loaded: true, category: drift.CategoryOf(s)}}

	compact := newCompactLayout(rows, "", 80, st)
	table := newTableLayout(rows, "", 120, fixedNow)

	tests := []struct {
		name       string
		idle, busy string
	}{
		{
			"compact",
			repoRow(s, true, "", false, token{}, "", compact, st),
			repoRow(s, true, spinnerFrames[1], false, token{}, "", compact, st),
		},
		{
			"zoom table",
			repoTableRow(s, true, "", false, token{}, "", table, st, fixedNow),
			repoTableRow(s, true, spinnerFrames[1], false, token{}, "", table, st, fixedNow),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			idle, busy := []rune(ansi.Strip(tt.idle)), []rune(ansi.Strip(tt.busy))
			if want := spinnerFrames[1] + string(idle[1:]); string(busy) != want {
				t.Errorf("busy row = %q, want %q", string(busy), want)
			}

			if muted := st.Muted.Render("↓12"); !strings.Contains(tt.busy, muted) || strings.Contains(tt.idle, muted) {
				t.Errorf("↓12 muted in the busy row %v and in the idle one %v, want only in the busy one",
					strings.Contains(tt.busy, muted), strings.Contains(tt.idle, muted))
			}
		})
	}
}

func TestRepoRowMarkers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		snap     drift.Snapshot
		loaded   bool
		selected bool
		want     string
	}{
		{"needs attention", fixtureBilling(fixedNow), true, false, "! billing "},
		{"behind", behindMain(nameAPIGateway, 1, nil, fixedNow), true, false, "↓ api-gateway "},
		{"up to date", fixtureAuth(fixedNow), true, false, "✓ auth "},
		{"selected for pull", behindMain(nameAPIGateway, 1, nil, fixedNow), true, true, "↓●api-gateway "},
		{"unloaded", fixtureBilling(fixedNow), false, false, "  billing "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ansi.Strip(compactRow(tt.snap, tt.loaded, tt.selected, 44))
			if !strings.HasPrefix(got, tt.want) {
				t.Errorf("repoRow() = %q, want prefix %q", got, tt.want)
			}
		})
	}
}

// TestCompactCells checks the cell of every compact column: name, branch,
// local changes, upstream, base, badges, only here and gone.
func TestCompactCells(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mut      func(s *drift.Snapshot)
		unloaded bool
		want     []string
	}{
		{"diverged with changes", func(s *drift.Snapshot) {
			s.Status.Ahead, s.Status.Behind, s.Status.Tracked, s.Status.Untracked = 3, 12, 1, 2
		}, false, []string{nameWeb, "", branchMain, "*3", "↑3↓12", "", "", "", ""}},
		{"ahead only", func(s *drift.Snapshot) { s.Status.Ahead = 2 }, false, []string{nameWeb, "", branchMain, "", "↑2", "", "", "", ""}},
		{"no upstream", func(s *drift.Snapshot) { s.Status.Upstream = "" }, false, []string{nameWeb, "", branchMain, "", noUpstream, "", "", "", ""}},
		{"upstream gone", func(s *drift.Snapshot) { s.Status.UpstreamGone = true }, false, []string{nameWeb, "", branchMain, "", "gone", "", "", "", ""}},
		{"no remote", func(s *drift.Snapshot) {
			s.Status.Upstream = ""
			s.Work = repo.Work{NoRemote: true, OnlyHereCommits: 4}
		}, false, []string{nameWeb, "", branchMain, "", noRemote, "", "", "", ""}},
		{"base not found", func(s *drift.Snapshot) {
			s.Base = repo.Base{Configured: "develop", Source: repo.SourceRepo, NotFound: true}
		}, false, []string{nameWeb, "", branchMain, "", "ok", "base?", "", "", ""}},
		{"detached", func(s *drift.Snapshot) {
			s.Status.Branch = ""
			s.Status.Detached = true
			s.Status.Upstream = ""
		}, false, []string{nameWeb, "", "(3c4d5e6)", "", noUpstream, "", "", "", ""}},
		{"every token", func(s *drift.Snapshot) { *s = fixturePayments(fixedNow) }, false, []string{
			namePayments, "", branchCheckout, "*2", "↑3↓12", "base↓27", "deps mig", "⌂6", "✂2",
		}},
		{"in a folder", func(s *drift.Snapshot) { s.Loc = fixtureLocIn(folderPlatform, nameWeb) }, false, []string{
			nameWeb, folderPlatform, branchMain, "", "ok", "", "", "", "",
		}},
		{"not read yet", func(*drift.Snapshot) {}, true, []string{nameWeb, "", "", "", "", "", "", "", ""}},
		{"not read yet in a folder", func(s *drift.Snapshot) { s.Loc = fixtureLocIn(folderWeb, nameWeb) }, true, []string{
			nameWeb, folderWeb, "", "", "", "", "", "", "",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := tracking(nameWeb, branchMain, fixedNow)
			tt.mut(&s)

			if diff := cmp.Diff(tt.want, compactCells(s, !tt.unloaded, "")); diff != "" {
				t.Errorf("compactCells() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// cellUnder returns the cell of line under the header title: from the
// title's first cell to the next title. Every cell of these lines is one rune
// wide.
func cellUnder(t *testing.T, header, line, title string) string {
	t.Helper()

	i := strings.Index(header, title)
	if i < 0 {
		t.Fatalf("header %q lacks %s", header, title)
	}

	end := len(header)

	rest := header[i+len(title):]
	if next := strings.IndexFunc(rest, func(r rune) bool { return r != ' ' }); next >= 0 {
		end = i + len(title) + next
	}

	start, stop := utf8.RuneCountInString(header[:i]), utf8.RuneCountInString(header[:end])
	runes := []rune(line)

	return strings.TrimRight(string(runes[start:min(stop, len(runes))]), " ")
}

// TestNoRemoteRows checks that a repository without any remote shows
// "no remote" in place of the count of work that only it has.
func TestNoRemoteRows(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	s := tracking("scratch", branchMain, fixedNow)
	s.Status.Upstream = ""
	s.Work = repo.Work{NoRemote: true, OnlyHereCommits: 4, Stashes: []repo.Stash{{Ref: "stash@{0}"}}}
	st := newStyles(true)

	if row := ansi.Strip(compactRow(s, true, false, 60)); !strings.Contains(row, " no remote") || strings.Contains(row, "⌂") {
		t.Errorf("repoRow() = %q, want no remote in place of the ⌂ count", row)
	}

	payments := fixturePayments(fixedNow)
	tl := newTableLayout(loadedRows(s, payments), "", 120, fixedNow)
	header := ansi.Strip(repoTableHeader(tl))
	row := ansi.Strip(repoTableRow(s, true, "", false, token{}, "", tl, st, fixedNow))
	other := ansi.Strip(repoTableRow(payments, true, "", false, token{}, "", tl, st, fixedNow))

	for _, c := range []struct{ line, title, want string }{
		{row, titleUpstream, "no remote"},
		{row, titleHere, "no remote"},
		{row, titleGone, none},
		{other, titleHere, "6"},
		{other, titleGone, "2"},
	} {
		if got := cellUnder(t, header, c.line, c.title); got != c.want {
			t.Errorf("%s of %q = %q, want %q (header %q)", c.title, c.line, got, c.want, header)
		}
	}
}

// TestRepoRowColumns checks the shrink order of a compact row: the branch
// is cut first, then the name, down to 10 and 13 cells; then the status
// columns go, the upstream state last.
func TestRepoRowColumns(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	const longer = "notifications2"

	tests := []struct {
		name, repo, branch string
		width              int
		want               string
	}{
		// UPSTREAM is as wide as its title, BRANCH at least as wide as its own.
		{"a name of 13 and a branch of 11 fit", nameNotifications, "feature/vat", 36, "✓ notifications feature/vat ok      "},
		{"the branch is cut first", longer, "feature/vat2", 37, "✓ notifications2 feature/va… ok      "},
		{"down to 10", longer, "feature/vat2", 36, "✓ notifications2 feature/v… ok      "},
		{"then the name, down to 13", longer, "feature/vat2", 35, "✓ notification… feature/v… ok      "},
		{"a short branch leaves the name to cut", longer, branchMain, 31, "✓ notification… main   ok      "},
		{"at the minimums the status stays", nameNotifications, "feature/vat", 35, "✓ notifications feature/v… ok      "},
		{"below the minimums the status goes", nameNotifications, "feature/vat", 34, "✓ notifications        feature/vat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := compactRow(tracking(tt.repo, tt.branch, fixedNow), true, false, tt.width)
			if s := ansi.Strip(got); s != tt.want {
				t.Errorf("repoRow() = %q, want %q", s, tt.want)
			}

			if w := ansi.StringWidth(got); w != tt.width {
				t.Errorf("repoRow() width = %d, want %d", w, tt.width)
			}
		})
	}
}

// TestCompactDropOrder checks that the local changes go before the upstream
// state, the last status column to go.
func TestCompactDropOrder(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	s := tracking(nameNotifications, "feature/vat", fixedNow)
	s.Status.Tracked = 3

	tests := []struct {
		name  string
		width int
		want  string
	}{
		{"both fit", 41, "✓ notifications feature/v… *3    ok      "},
		{"the local changes go first", 40, "✓ notifications   feature/vat   ok      "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ansi.Strip(compactRow(s, true, false, tt.width)); got != tt.want {
				t.Errorf("repoRow() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepoTable(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	const width = 100

	st := newStyles(true)

	rows := append(loadedRows(sortedFixture()...), row{snap: drift.Snapshot{Loc: fixtureLoc(namePayments)}})
	tl := newTableLayout(rows, "", width, fixedNow)

	var b strings.Builder

	b.WriteString(framed(repoTableHeader(tl)))

	for _, r := range rows {
		line := repoTableRow(r.snap, r.loaded, "", false, token{}, "", tl, st, fixedNow)
		if got := ansi.StringWidth(line); got != width {
			t.Errorf("repoTableRow(%s) width = %d, want %d", r.snap.Loc.Name, got, width)
		}

		b.WriteString(framed(line))
	}

	assertGolden(t, "repo_table.golden", b.String())
}

func TestTableLayout(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	fixture := loadedRows(sortedFixture()...)
	long := tracking(nameAuth, "feature/a-very-long-branch-name", fixedNow)
	unloaded := drift.Snapshot{Loc: fixtureLoc("notifications")}
	hostile := tracking("ab\x1b[2Jcd", branchMain, fixedNow)
	scratch := tracking("scratch", branchMain, fixedNow)
	scratch.Work.NoRemote = true

	tests := []struct {
		name                           string
		rows                           []row
		width                          int
		wantName, wantBranch, wantHere int
		wantLast                       string
	}{
		{"short values take the titles' widths", loadedRows(fixtureAuth(fixedNow)), 120, 4, 6, 4, titleUpdated},
		{"fixture fits in full", fixture, 100, 13, 16, 4, titleUpdated},
		{"fixture at its own width", fixture, 90, 13, 16, 4, titleUpdated},
		{"one cell short cuts the branch", fixture, 89, 13, 15, 4, titleUpdated},
		{"the branch down to its minimum", fixture, 88, 13, 14, 4, titleUpdated},
		{"then the name", fixture, 87, 12, 14, 4, titleUpdated},
		{"the name down to its minimum", fixture, 86, 11, 14, 4, titleUpdated},
		{"then UPDATED goes and the others get their room back", fixture, 85, 13, 16, 4, titleGone},
		{"the narrowest screen keeps the first columns", fixture, 55, 13, 16, 0, titleUpstream},
		{"one long branch takes all the room", loadedRows(long), 100, 4, 31, 4, titleUpdated},
		{"unloaded rows count by name", []row{{snap: unloaded}}, 120, 13, 6, 4, titleUpdated},
		{"names are measured sanitized", loadedRows(hostile), 120, 4, 6, 4, titleUpdated},
		{"no rows", nil, 120, 2, 6, 4, titleUpdated},
		{"no remote widens HERE", append(fixture, loadedRows(scratch)...), 100, 13, 16, 9, titleUpdated},
	}

	titles := tableHeader()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := newTableLayout(tt.rows, "", tt.width, fixedNow)

			var last string

			for i, title := range titles {
				if l.shown[i] {
					last = title
				}
			}

			var here int
			if l.shown[tableHere] {
				here = l.widths[tableHere]
			}

			got := []any{l.widths[tableRepo] - tableSelectWidth, l.widths[tableBranch], here, last}
			want := []any{tt.wantName, tt.wantBranch, tt.wantHere, tt.wantLast}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("newTableLayout() name, branch, HERE, last title mismatch (-want +got):\n%s", diff)
			}

			if w := ansi.StringWidth(repoTableHeader(l)); w != tt.width {
				t.Errorf("repoTableHeader() width = %d, want %d", w, tt.width)
			}
		})
	}
}

func TestRepoTableTruncatesOnlyWhenNarrow(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	snaps := sortedFixture()
	notifications, payments := snaps[3], snaps[1]

	tests := []struct {
		name  string
		width int
		want  []string
	}{
		{"wide", 100, []string{"↓ notifications ", " feature/checkout "}},
		{"narrow", 87, []string{"↓ notificatio… ", " feature/check… "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tl := newTableLayout(loadedRows(snaps...), "", tt.width, fixedNow)
			got := []string{
				ansi.Strip(repoTableRow(notifications, true, "", false, token{}, "", tl, newStyles(true), fixedNow)),
				ansi.Strip(repoTableRow(payments, true, "", false, token{}, "", tl, newStyles(true), fixedNow)),
			}

			for i, w := range tt.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("row %d = %q, want it to contain %q", i, got[i], w)
				}
			}
		})
	}
}

func TestRepoTableSelectedAndEmptyCells(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	s := behindMain(nameAPIGateway, 12, nil, fixedNow)
	s.Status.LastUpdate = fixedNow.Add(-30 * time.Minute)

	tl := newTableLayout(loadedRows(s), "", 100, fixedNow)
	header := ansi.Strip(repoTableHeader(tl))
	got := ansi.Strip(repoTableRow(s, true, "", true, token{}, "", tl, newStyles(true), fixedNow))

	for _, c := range []struct{ title, want string }{
		{titleRepo, "api-gateway ●"},
		{titleBranch, branchMain},
		{titleLocal, "clean"},
		{titleUpstream, "↓12"},
		{"VS BASE", none},
		{"INCOMING", none},
		{titleHere, none},
		{titleGone, none},
		{titleUpdated, "30m ago"},
	} {
		if cell := cellUnder(t, header, got, c.title); cell != c.want {
			t.Errorf("%s = %q, want %q:\n%s\n%s", c.title, cell, c.want, header, got)
		}
	}
}

func TestRepoRowSanitizesBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mut  func(s *drift.Snapshot)
		want []string
	}{
		{"branch", func(s *drift.Snapshot) { s.Status.Branch = "evil\x1b[2Jname" }, []string{"evil", "name"}},
		{"repo name", func(s *drift.Snapshot) { s.Loc.Name = "bad\nre\x07po" }, []string{"badrepo"}},
		{"folder", func(s *drift.Snapshot) { s.Loc.Folder = "ser\x1b[31mvices/\tplat\x07form" }, []string{folderPlatform}},
		{"badge", func(s *drift.Snapshot) {
			s.Status.Behind = 1
			s.Badges = []string{"de\x1b]0;title\x07ps"}
		}, []string{badgeDeps}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := fixtureBilling(fixedNow)
			tt.mut(&s)

			rows := map[string]string{
				"compact": repoRow(s, true, "", false, token{}, "", newCompactLayout(loadedRows(s), "", 60, Styles{}), Styles{}),
				"table":   repoTableRow(s, true, "", false, token{}, "", newTableLayout(loadedRows(s), "", 120, fixedNow), Styles{}, fixedNow),
			}

			for kind, row := range rows {
				if strings.ContainsFunc(row, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
					t.Errorf("%s row %q contains a control character", kind, row)
				}

				for _, w := range tt.want {
					if !strings.Contains(row, w) {
						t.Errorf("%s row %q does not contain %q", kind, row, w)
					}
				}
			}
		})
	}
}

// TestReposPaneColumns checks the compact rows of a wide Repos pane: every
// name and branch in full, every kind of token at one column on every row,
// and the last column ending at the pane's right edge.
func TestReposPaneColumns(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	const nameMedium, branchMedium = "merchant-settlement", "feature/batches"

	snaps := append(fixtureSnapshots(fixedNow), fixtureLong(nameMedium, branchMedium, fixedNow))
	m, _ := loadedModel(t, snaps, 200, 30)

	lines := paneInside(t, m, PaneRepos)
	assertCompactColumns(t, lines[0], lines[1:], snaps, compactTokens(nameMedium))
}

// TestCompactHeaderWidths checks that a compact column with values is at
// least as wide as its title, so that no title is ever cut, and that a
// column without values takes no room and has no title.
func TestCompactHeaderWidths(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	st := newStyles(true)
	s := tracking(nameWeb, branchMain, fixedNow)
	l := newCompactLayout(loadedRows(s), "", 22, st)

	got := []string{ansi.Strip(repoHeader(l, st)), ansi.Strip(repoRow(s, true, "", false, token{}, "", l, st))}
	want := []string{"  REPO BRANCH UPSTREAM", "✓ web  main   ok      "}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("header and row mismatch (-want +got):\n%s", diff)
	}
}

// TestReposPaneHeader checks the column titles above the compact rows:
// they name the columns shown, each title starts where its column's values
// start, and a dropped column drops its title.
func TestReposPaneHeader(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	payments := compactCells(fixturePayments(fixedNow), true, "")

	tests := []struct {
		name          string
		width, height int
		titles        []string
	}{
		{"120x40", 120, 40, []string{titleRepo, titleBranch, titleLocal, titleUpstream, titleBase, "INCOMING", titleHere, titleGone}},
		{"60x24", 60, 24, []string{titleRepo, titleBranch, titleLocal, titleUpstream, titleBase}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, fixtureSnapshots(fixedNow), tt.width, tt.height)
			lines := paneInside(t, m, PaneRepos)
			header := lines[0]

			if diff := cmp.Diff(tt.titles, strings.Fields(header)); diff != "" {
				t.Fatalf("header %q titles mismatch (-want +got):\n%s", header, diff)
			}

			// Each value starts under its title, whole or cut with "…".
			row := rowOf(t, lines[1:], namePayments)

			for _, title := range tt.titles {
				want := payments[slices.Index(compactHeader(), title)]

				got := cellUnder(t, header, row, title)
				if cut, isCut := strings.CutSuffix(got, "…"); got == "" || !strings.HasPrefix(want, cut) || (!isCut && got != want) {
					t.Errorf("under %s the row %q shows %q, want %q", title, row, got, want)
				}
			}
		})
	}
}

// TestReposPaneHeaderCollapsed checks that a Repos pane shrunk to three
// lines, on the smallest screen with focus below it, gives every line to
// repositories.
func TestReposPaneHeaderCollapsed(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := loadedModel(t, fixtureSnapshots(fixedNow), minWidth, minHeight)
	m = press(t, m, "2")
	lines := paneInside(t, m, PaneRepos)

	if len(lines) != 3 {
		t.Fatalf("collapsed Repos pane has %d lines, want 3", len(lines))
	}

	for _, l := range lines {
		if strings.Contains(l, titleRepo) {
			t.Errorf("collapsed Repos pane shows the header: %q", l)
		}
	}

	if !strings.HasPrefix(lines[0], "▸! billing ") || !strings.Contains(lines[2], "… 4 more") {
		t.Errorf("collapsed Repos pane = %q, want two repositories and … 4 more", lines)
	}
}

// TestViewZoomSpansPane checks that the zoom table ends at the pane's right
// edge, the last title above the values of its column.
func TestViewZoomSpansPane(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := loadedModel(t, fixtureSnapshots(fixedNow), 120, 24)
	lines := paneInside(t, press(t, m, "z"), PaneRepos)
	header := lines[0]

	if !strings.HasSuffix(header, titleUpdated) {
		t.Fatalf("header %q does not end at the pane's right edge", header)
	}

	col := runeIndex(header, titleUpdated, 0)

	for _, row := range lines[1:7] {
		if runes := []rune(row); runes[col] == ' ' || runes[col-1] != ' ' {
			t.Errorf("row %q has no UPDATED value under the title at column %d", row, col)
		}
	}
}

func TestScrollWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                         string
		total, cursor, top, capacity int
		wantStart, wantEnd           int
	}{
		{"everything fits", 5, 4, 0, 10, 0, 5},
		{"exact fit", 13, 12, 0, 13, 0, 13},
		{"more line at the top", 32, 0, 0, 13, 0, 12},
		{"cursor on the last shown row", 32, 11, 0, 13, 0, 12},
		{"cursor below scrolls by one", 32, 12, 0, 13, 1, 13},
		{"cursor on the last row", 32, 31, 0, 13, 19, 32},
		{"cursor above scrolls up", 32, 0, 19, 13, 0, 12},
		{"cursor inside the bottom window", 32, 20, 19, 13, 19, 32},
		{"cursor just above the bottom window", 32, 18, 19, 13, 18, 30},
		{"stale top is clamped", 5, 0, 9, 3, 0, 2},
		{"capacity of one", 3, 2, 0, 1, 2, 3},
		{"no capacity", 3, 1, 0, 0, 0, 0},
		{"no rows", 0, 0, 0, 5, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			start, end := scrollWindow(tt.total, tt.cursor, tt.top, tt.capacity)
			if diff := cmp.Diff([2]int{tt.wantStart, tt.wantEnd}, [2]int{start, end}); diff != "" {
				t.Errorf("scrollWindow(%d, %d, %d, %d) mismatch (-want +got):\n%s",
					tt.total, tt.cursor, tt.top, tt.capacity, diff)
			}
		})
	}
}

// TestSortRowsByFolder checks the order of the Repos rows: category, then
// folder, the root first and each folder's subfolders right after it, then
// name.
func TestSortRowsByFolder(t *testing.T) {
	t.Parallel()

	up := func(folder, name string) drift.Snapshot {
		return relocated(folder, name, tracking(name, branchMain, fixedNow))
	}

	tests := []struct {
		name  string
		snaps []drift.Snapshot
		want  []string
	}{
		{"nested fixture", fixtureNested(fixedNow), nestedOrder()},
		{"subfolders before a sibling that extends the name", []drift.Snapshot{
			up("services-x", "alpha"), up(folderPlatform, "beta"), up(folderServices, "gamma"), up("", "delta"),
		}, []string{"delta", "services/gamma", "services/platform/beta", "services-x/alpha"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rows := loadedRows(tt.snaps...)
			sortRows(rows)

			if diff := cmp.Diff(tt.want, relativePaths(rows)); diff != "" {
				t.Errorf("sortRows() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFolderLabel(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, folder, tab, want string }{
		{"All shows the whole folder", folderPlatform, "", folderPlatform},
		{"a repository directly in the tab", folderServices, folderServices, ""},
		{"a subfolder of the tab", folderPlatform, folderServices, platformInServices},
		{"deeper below the tab", "backend/x/y", "backend", "x/y"},
		{"a root repository in All", "", "", ""},
		{"a root repository in a tab", "", folderServices, ""},
		{"a sibling that extends the tab's name", "services-x", folderServices, "services-x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := folderLabel(tt.folder, tt.tab); got != tt.want {
				t.Errorf("folderLabel(%q, %q) = %q, want %q", tt.folder, tt.tab, got, tt.want)
			}
		})
	}
}

// TestCutFolder checks the cut of a folder too wide for the FOLDER column:
// whole folder names after "…/", as many as fit, and a plain cut from the
// left only when even the last name does not fit.
func TestCutFolder(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	const deep = "team/services/platform"

	tests := []struct {
		name, folder string
		width        int
		want         string
	}{
		{"fits", folderPlatform, 17, folderPlatform},
		{"one name kept", folderPlatform, 16, platformCut},
		{"one name kept at its narrowest", folderPlatform, 10, platformCut},
		{"two names kept", deep, 19, "…/services/platform"},
		{"one name kept when two do not fit", deep, 18, platformCut},
		{"the last name does not fit", folderPlatform, 9, "…platform"},
		{"a single name too wide", nameStorefront, 6, "…front"},
		{"no room", folderPlatform, 0, ""},
		{"wide names kept whole", "ｗｅｂ/ｓｈｏｐ/漢字", 15, "…/ｓｈｏｐ/漢字"},
		{"one wide name kept", "ｗｅｂ/ｓｈｏｐ/漢字", 14, "…/漢字"},
		{"wide fallback on a cell boundary", "ｓｈｏｐ/漢字", 4, "…字"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cutFolder(tt.folder, tt.width)
			if got != tt.want {
				t.Errorf("cutFolder(%q, %d) = %q, want %q", tt.folder, tt.width, got, tt.want)
			}

			if w := ansi.StringWidth(got); w > tt.width {
				t.Errorf("cutFolder(%q, %d) is %d cells wide", tt.folder, tt.width, w)
			}
		})
	}
}

// compactLines renders the header and the compact rows of rows laid out
// width cells wide, without styling.
func compactLines(t *testing.T, rows []row, width int) (header string, lines []string) {
	t.Helper()

	st := newStyles(true)
	l := newCompactLayout(rows, "", width, st)

	for _, r := range rows {
		line := repoRow(r.snap, r.loaded, "", false, token{}, "", l, st)
		if got := ansi.StringWidth(line); got != width {
			t.Errorf("repoRow(%s) at %d width = %d, want %d", r.snap.Loc.Name, width, got, width)
		}

		lines = append(lines, ansi.Strip(line))
	}

	return ansi.Strip(repoHeader(l, st)), lines
}

// TestRepoRowsNestedFolderColumn checks the FOLDER column of the compact
// rows: right after REPO, empty for a repository at the root. In a pane 66
// cells wide BASE stays and the deepest folder name shows whole after "…/";
// with room for every column each folder shows in full.
func TestRepoRowsNestedFolderColumn(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	tests := []struct {
		name    string
		lines   func(t *testing.T) (header string, lines []string)
		titles  []string
		folders map[string]string
	}{
		{
			name: "66x40 pane",
			lines: func(t *testing.T) (string, []string) {
				t.Helper()

				m, _ := loadedModel(t, fixtureNested(fixedNow), 66, 40)
				header, lines, _ := reposPane(t, m)

				return header, lines
			},
			titles:  []string{titleRepo, titleFolder, titleBranch, titleLocal, titleUpstream, titleBase},
			folders: map[string]string{namePayments: folderServices, nameNotifications: platformCut, nameStorefront: folderWeb, "docs": ""},
		},
		{
			name: "rows 120 cells wide",
			lines: func(t *testing.T) (string, []string) {
				t.Helper()

				return compactLines(t, nestedRows(), 120)
			},
			titles:  nestedTitles(),
			folders: map[string]string{namePayments: folderServices, nameNotifications: folderPlatform, nameStorefront: folderWeb, "docs": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			header, lines := tt.lines(t)
			if diff := cmp.Diff(tt.titles, strings.Fields(header)); diff != "" {
				t.Fatalf("header %q titles mismatch (-want +got):\n%s", header, diff)
			}

			for name, want := range tt.folders {
				if got := cellUnder(t, header, rowOf(t, lines, name), titleFolder); got != want {
					t.Errorf("FOLDER of %s = %q, want %q:\n%s\n%s", name, got, want, header, strings.Join(lines, "\n"))
				}
			}
		})
	}
}

// nestedTitles are the titles of the compact rows of the nested fixture
// with every column shown.
func nestedTitles() []string {
	return []string{
		titleRepo, titleFolder, titleBranch, titleLocal, titleUpstream, titleBase, "INCOMING", titleHere, titleGone,
	}
}

// without returns titles less the titles drop.
func without(titles []string, drop ...string) []string {
	return slices.DeleteFunc(slices.Clone(titles), func(s string) bool { return slices.Contains(drop, s) })
}

// TestRepoRowsFolderDropOrder checks how FOLDER gives way in a narrow row:
// it is cut first, from the left at a folder boundary so that the deepest
// folder stays, down to 10 cells, before the branch; columns go in the
// order GONE, HERE, INCOMING, BASE, FOLDER, LOCAL, with UPSTREAM last. At
// 138x40 the Repos pane has dropped BASE and cuts FOLDER; at 100x24 FOLDER
// is gone and LOCAL and UPSTREAM stay.
func TestRepoRowsFolderDropOrder(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	all := nestedTitles()

	// folder is the FOLDER cell of notifications and branch the BRANCH cell
	// of payments; "" skips the check.
	tests := []struct {
		name           string
		width          int
		titles         []string
		folder, branch string
	}{
		{"every column in full", 92, all, folderPlatform, branchCheckout},
		{"the folder is cut first, at a folder boundary", 85, all, platformCut, branchCheckout},
		{"the folder down to 10 cells, then the branch", 79, all, platformCut, "feature/c…"},
		{"GONE goes first", 78, without(all, titleGone), platformCut, ""},
		{"then HERE", 73, without(all, titleGone, titleHere), platformCut, ""},
		{"then INCOMING", 68, without(all, titleGone, titleHere, "INCOMING"), platformCut, branchCheckout},
		{"then BASE", 59, all[:5], platformCut, branchCheckout},
		{"then FOLDER", 51, without(all[:5], titleFolder), "", branchCheckout},
		{"then LOCAL, with UPSTREAM last", 40, []string{titleRepo, titleBranch, titleUpstream}, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			header, lines := compactLines(t, nestedRows(), tt.width)
			if diff := cmp.Diff(tt.titles, strings.Fields(header)); diff != "" {
				t.Fatalf("header %q titles mismatch (-want +got):\n%s", header, diff)
			}

			if tt.folder != "" {
				if got := cellUnder(t, header, rowOf(t, lines, nameNotifications), titleFolder); got != tt.folder {
					t.Errorf("FOLDER of notifications = %q, want %q:\n%s\n%s", got, tt.folder, header, strings.Join(lines, "\n"))
				}
			}

			if tt.branch != "" {
				if got := cellUnder(t, header, rowOf(t, lines, namePayments), titleBranch); got != tt.branch {
					t.Errorf("BRANCH of payments = %q, want %q:\n%s\n%s", got, tt.branch, header, strings.Join(lines, "\n"))
				}
			}
		})
	}

	panes := []struct {
		width, height int
		titles        []string
		folder        string
	}{
		{60, 40, all[:5], platformCut},
		{150, 40, all, folderPlatform},
	}

	for _, p := range panes {
		t.Run(fmt.Sprintf("%dx%d", p.width, p.height), func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, fixtureNested(fixedNow), p.width, p.height)
			header, lines, _ := reposPane(t, m)

			if diff := cmp.Diff(p.titles, strings.Fields(header)); diff != "" {
				t.Fatalf("header %q titles mismatch (-want +got):\n%s", header, diff)
			}

			if p.folder != "" {
				if got := cellUnder(t, header, rowOf(t, lines[1:], nameNotifications), titleFolder); got != p.folder {
					t.Errorf("FOLDER of notifications = %q, want %q:\n%s", got, p.folder, strings.Join(lines, "\n"))
				}
			}
		})
	}
}

// TestRepoTableNestedFolder renders the zoom table of the nested fixture and
// a row not read yet: FOLDER right after REPO, as wide as the longest
// folder however narrow the table, and empty at the root.
func TestRepoTableNestedFolder(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	const width = 160

	rows := append(nestedRows(), row{snap: drift.Snapshot{Loc: fixtureLocIn(folderWeb, "ledger")}})
	tl := newTableLayout(rows, "", width, fixedNow)
	st := newStyles(true)

	var b strings.Builder

	b.WriteString(framed(repoTableHeader(tl)))

	for _, r := range rows {
		line := repoTableRow(r.snap, r.loaded, "", false, token{}, "", tl, st, fixedNow)
		if got := ansi.StringWidth(line); got != width {
			t.Errorf("repoTableRow(%s) width = %d, want %d", r.snap.Loc.Name, got, width)
		}

		b.WriteString(framed(line))
	}

	assertGolden(t, "repo_table_nested.golden", b.String())

	if titles := strings.Fields(ansi.Strip(repoTableHeader(tl))); len(titles) < 2 || titles[1] != titleFolder {
		t.Errorf("table titles %q, want FOLDER right after REPO", titles)
	}

	for _, w := range []int{width, 55} {
		if l := newTableLayout(rows, "", w, fixedNow); !l.shown[tableFolder] || l.widths[tableFolder] != len(folderPlatform) {
			t.Errorf("at %d cells FOLDER shown = %t, %d cells wide, want %d", w, l.shown[tableFolder], l.widths[tableFolder], len(folderPlatform))
		}
	}
}

// TestFlatFixtureUnchanged checks that a flat workspace, every repository
// directly in the folder, has no FOLDER column at any width, compact or
// zoomed, and keeps the order of category, then name. The golden files of
// the flat fixture (repo_rows, repo_table, view_main and view_zoom) pin its
// rows byte for byte.
func TestFlatFixtureUnchanged(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	st := newStyles(true)
	rows := loadedRows(fixtureSnapshots(fixedNow)...)
	sortRows(rows)

	var want []string
	for _, s := range sortedFixture() {
		want = append(want, s.Loc.Name)
	}

	if diff := cmp.Diff(want, relativePaths(rows)); diff != "" {
		t.Errorf("sortRows() of the flat fixture mismatch (-want +got):\n%s", diff)
	}

	for width := 1; width <= 200; width++ {
		compact, table := newCompactLayout(rows, "", width, st), newTableLayout(rows, "", width, fixedNow)
		headers := ansi.Strip(repoHeader(compact, st)) + ansi.Strip(repoTableHeader(table))

		if compact.shown[compactFolder] || table.shown[tableFolder] || strings.Contains(headers, titleFolder) {
			t.Fatalf("flat fixture at %d cells shows FOLDER: %q", width, headers)
		}
	}

	for _, size := range [][2]int{{100, 24}, {120, 40}} {
		m, _ := loadedModel(t, fixtureSnapshots(fixedNow), size[0], size[1])
		for _, view := range []string{viewText(m), viewText(press(t, m, "z"))} {
			if strings.Contains(view, titleFolder) {
				t.Errorf("flat view at %dx%d shows FOLDER:\n%s", size[0], size[1], view)
			}
		}
	}
}

// TestRepoHeaderIsBold checks that the column titles are drawn in the bold
// heading style, so that they stand apart from the rows.
func TestRepoHeaderIsBold(t *testing.T) {
	t.Parallel()

	m, _ := loadedModel(t, fixtureSnapshots(fixedNow), 150, 40)
	l := newCompactLayout(m.rowsAt(m.visible()), "", 140, m.styles)

	if h := repoHeader(l, m.styles); !strings.Contains(h, m.styles.Heading.Render(titleRepo)) {
		t.Errorf("repoHeader() = %q, want REPO in the heading style", h)
	}
}
