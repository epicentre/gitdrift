package ui

import (
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
)

// loadedRows wraps snaps as rows read by a refresh.
func loadedRows(snaps ...drift.Snapshot) []row {
	rows := make([]row, len(snaps))
	for i, s := range snaps {
		rows[i] = row{snap: s, loaded: true}
	}

	return rows
}

// compactRow renders s alone as a compact row of width cells.
func compactRow(s drift.Snapshot, loaded, selected bool, width int) string {
	st := newStyles(true)

	return repoRow(s, loaded, selected, token{}, newCompactLayout([]row{{snap: s, loaded: loaded}}, width, st), st)
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
		l := newCompactLayout(rows, width, st)
		headers[width] = ansi.Strip(repoHeader(l, st))

		b.WriteString("width " + strconv.Itoa(width) + "\n")
		b.WriteString(framed(repoHeader(l, st)))

		for _, r := range rows {
			line := repoRow(r.snap, r.loaded, false, token{}, l, st)
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
		"  ledger                      fetching…     ",
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
		}, false, []string{nameWeb, branchMain, "*3", "↑3↓12", "", "", "", ""}},
		{"ahead only", func(s *drift.Snapshot) { s.Status.Ahead = 2 }, false, []string{nameWeb, branchMain, "", "↑2", "", "", "", ""}},
		{"no upstream", func(s *drift.Snapshot) { s.Status.Upstream = "" }, false, []string{nameWeb, branchMain, "", noUpstream, "", "", "", ""}},
		{"upstream gone", func(s *drift.Snapshot) { s.Status.UpstreamGone = true }, false, []string{nameWeb, branchMain, "", "gone", "", "", "", ""}},
		{"no remote", func(s *drift.Snapshot) {
			s.Status.Upstream = ""
			s.Work = repo.Work{NoRemote: true, OnlyHereCommits: 4}
		}, false, []string{nameWeb, branchMain, "", noRemote, "", "", "", ""}},
		{"base not found", func(s *drift.Snapshot) {
			s.Base = repo.Base{Configured: "develop", Source: repo.SourceRepo, NotFound: true}
		}, false, []string{nameWeb, branchMain, "", "ok", "base?", "", "", ""}},
		{"detached", func(s *drift.Snapshot) {
			s.Status.Branch = ""
			s.Status.Detached = true
			s.Status.Upstream = ""
		}, false, []string{nameWeb, "(3c4d5e6)", "", noUpstream, "", "", "", ""}},
		{"every token", func(s *drift.Snapshot) { *s = fixturePayments(fixedNow) }, false, []string{
			namePayments, branchCheckout, "*2", "↑3↓12", "base↓27", "deps mig", "⌂6", "✂2",
		}},
		{"not read yet", func(*drift.Snapshot) {}, true, []string{nameWeb, "", "", "", "", "", "", ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := tracking(nameWeb, branchMain, fixedNow)
			tt.mut(&s)

			if diff := cmp.Diff(tt.want, compactCells(s, !tt.unloaded)); diff != "" {
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
	tl := newTableLayout(loadedRows(s, payments), 120, fixedNow)
	header := ansi.Strip(repoTableHeader(tl))
	row := ansi.Strip(repoTableRow(s, true, false, token{}, tl, st, fixedNow))
	other := ansi.Strip(repoTableRow(payments, true, false, token{}, tl, st, fixedNow))

	for _, c := range []struct{ line, title, want string }{
		{row, titleUpstream, "no remote"},
		{row, "HERE", "no remote"},
		{row, titleGone, none},
		{other, "HERE", "6"},
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
	tl := newTableLayout(rows, width, fixedNow)

	var b strings.Builder

	b.WriteString(framed(repoTableHeader(tl)))

	for _, r := range rows {
		line := repoTableRow(r.snap, r.loaded, false, token{}, tl, st, fixedNow)
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

			l := newTableLayout(tt.rows, tt.width, fixedNow)

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

			tl := newTableLayout(loadedRows(snaps...), tt.width, fixedNow)
			got := []string{
				ansi.Strip(repoTableRow(notifications, true, false, token{}, tl, newStyles(true), fixedNow)),
				ansi.Strip(repoTableRow(payments, true, false, token{}, tl, newStyles(true), fixedNow)),
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

	tl := newTableLayout(loadedRows(s), 100, fixedNow)
	header := ansi.Strip(repoTableHeader(tl))
	got := ansi.Strip(repoTableRow(s, true, true, token{}, tl, newStyles(true), fixedNow))

	for _, c := range []struct{ title, want string }{
		{"REPO", "api-gateway ●"},
		{"BRANCH", branchMain},
		{"LOCAL", "clean"},
		{titleUpstream, "↓12"},
		{"VS BASE", none},
		{"INCOMING", none},
		{"HERE", none},
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
				"compact": repoRow(s, true, false, token{}, newCompactLayout(loadedRows(s), 60, Styles{}), Styles{}),
				"table":   repoTableRow(s, true, false, token{}, newTableLayout(loadedRows(s), 120, fixedNow), Styles{}, fixedNow),
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
	l := newCompactLayout(loadedRows(s), 22, st)

	got := []string{ansi.Strip(repoHeader(l, st)), ansi.Strip(repoRow(s, true, false, token{}, l, st))}
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

	payments := compactCells(fixturePayments(fixedNow), true)

	tests := []struct {
		name          string
		width, height int
		titles        []string
	}{
		{"120x40", 120, 40, []string{"REPO", "BRANCH", "LOCAL", titleUpstream, "BASE"}},
		{"100x24", 100, 24, []string{"REPO", "BRANCH", "LOCAL", titleUpstream}},
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
			for i, title := range tt.titles {
				got := cellUnder(t, header, row, title)
				if cut, isCut := strings.CutSuffix(got, "…"); got == "" || !strings.HasPrefix(payments[i], cut) || (!isCut && got != payments[i]) {
					t.Errorf("under %s the row %q shows %q, want %q", title, row, got, payments[i])
				}
			}
		})
	}
}

// TestReposPaneHeaderCollapsed checks that a collapsed Repos pane gives
// every line to repositories.
func TestReposPaneHeaderCollapsed(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)
	m = press(t, m, "2")
	lines := paneInside(t, m, PaneRepos)

	if len(lines) != 3 {
		t.Fatalf("collapsed Repos pane has %d lines, want 3", len(lines))
	}

	for _, l := range lines {
		if strings.Contains(l, "REPO") {
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
