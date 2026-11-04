package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/badge"
	"github.com/epicentre/gitdrift/internal/repo"
)

func TestFileRow(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	custom := append(badge.Defaults(),
		badge.Rule{Label: "cash", Dir: "cmd/payments"},
		badge.Rule{Label: "bad\x1b[2J\u202Ename", Name: "*.png"},
	)
	total := repo.FileChange{Path: pathTotal, Status: "M", Added: 1, Deleted: 1}
	lock := repo.FileChange{Path: pathLock, Status: "M", Added: 120, Deleted: 80}
	logo := repo.FileChange{Path: pathLogo, Status: "M", Binary: true}
	controller := repo.FileChange{Path: "internal/checkout/http/handlers/checkout.go", Status: "M", Added: 5, Deleted: 1}

	tests := []struct {
		name  string
		list  []repo.FileChange
		file  repo.FileChange
		rules []badge.Rule
		width int
		want  string
	}{
		// The path takes the width left by the status, the counts and the
		// badges, each column as wide as its longest value in the list.
		{"modified", nil, total, badge.Defaults(), 40, " M " + pad(pathTotal, 30) + "  +1 -1"},
		{"badge", nil, lock, badge.Defaults(), 40, " M " + pad(pathLock, 21) + "  +120 -80  deps"},
		{
			"badge by directory", nil,
			repo.FileChange{Path: "db/migrations/0001_init.sql", Status: "A", Added: 9},
			badge.Defaults(), 50, " A " + pad("db/migrations/0001_init.sql", 35) + "  +9 -0  mig",
		},
		{"binary", nil, logo, badge.Defaults(), 40, " M " + pad(pathLogo, 29) + "  binary"},
		{"untracked", nil, repo.FileChange{Path: "notes.txt", Status: "?"}, badge.Defaults(), 40, " ? " + pad("notes.txt", 37)},
		{
			"long path keeps its end", nil,
			repo.FileChange{Path: "src/very/deep/directory/tree/file.go", Status: "M", Added: 3, Deleted: 2},
			badge.Defaults(), 30, " M …ectory/tree/file.go  +3 -2",
		},
		{"long path keeps its file name", nil, controller, badge.Defaults(), 46, " M …/checkout/http/handlers/checkout.go  +5 -1"},
		{"sanitized", nil, repo.FileChange{Path: "a\x1b[2J\tb.txt", Status: "M", Added: 1}, badge.Defaults(), 40, " M " + pad("ab.txt", 30) + "  +1 -0"},
		{"repository rule", nil, total, custom, 40, " M " + pad(pathTotal, 24) + "  +1 -1  cash"},
		{"label sanitized", nil, logo, custom, 40, " M " + pad(pathLogo, 20) + "  binary  badname"},
		{"no rules", nil, repo.FileChange{Path: pathLock, Status: "M", Added: 1}, nil, 40, " M " + pad(pathLock, 30) + "  +1 -0"},
		{
			"counts right-aligned in the list",
			[]repo.FileChange{total, lock},
			total, badge.Defaults(), 40,
			" M " + pad(pathTotal, 21) + "    +1  -1" + pad("", 6),
		},
		{
			"binary across the count columns",
			[]repo.FileChange{total, logo},
			total, badge.Defaults(), 40,
			" M " + pad(pathTotal, 29) + "   +1 -1",
		},
		{"binary ends with the deleted counts", []repo.FileChange{total, logo}, logo, badge.Defaults(), 40, " M " + pad(pathLogo, 29) + "  binary"},
		{"a narrow pane drops the badges first", nil, lock, badge.Defaults(), 24, " M " + pad(pathLock, 11) + "  +120 -80"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			list := tt.list
			if list == nil {
				list = []repo.FileChange{tt.file}
			}

			fl := newFileList(list, tt.rules)

			got := fileRow(fl.lines[slices.Index(list, tt.file)], fl.layout(tt.width, newStyles(true)))
			if s := ansi.Strip(got); s != tt.want {
				t.Errorf("fileRow() = %q, want %q", s, tt.want)
			}

			if w := ansi.StringWidth(got); w != tt.width {
				t.Errorf("fileRow() width = %d, want %d", w, tt.width)
			}
		})
	}
}

// TestFilesPaneColumns checks the Files pane: the added and deleted counts
// right-aligned in their own columns, binary across both, the badges in one
// column, and a long path cut from the left so that its file name stays.
func TestFilesPaneColumns(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := paymentsModel(t, 120, 40)
	lines := paneInside(t, pressSettle(t, m, "3"), PaneFiles)

	// The end of each count and the start of each badge, by path.
	rows := []struct {
		path, added, deleted, badge string
	}{
		{pathTotal, "+1", "-1", ""},
		{pathLock, "+120", "-80", badgeDeps},
		{"0007_add_refunds_table.sql", "+40", "-0", badgeMig},
		{".env.example", "+2", "-0", "env"},
		{pathLogo, "", "binary", ""},
	}

	at := map[string]int{}

	check := func(kind, row string, i int) {
		t.Helper()

		if want, ok := at[kind]; ok && i != want {
			t.Errorf("%s of %q is at column %d, want %d:\n%s", kind, row, i, want, strings.Join(lines, "\n"))
		}

		at[kind] = i
	}

	for _, r := range rows {
		row := cutPathRow(lines, r.path)
		if row == "" {
			row = rowOf(t, lines, r.path)
		}

		end := runeIndex(row, r.path, 0) + utf8.RuneCountInString(r.path)

		if r.added != "" {
			check("added end", row, runeIndex(row, " "+r.added+" ", end)+1+utf8.RuneCountInString(r.added))
		}

		check("deleted end", row, runeIndex(row, " "+r.deleted, end)+1+utf8.RuneCountInString(r.deleted))

		if r.badge != "" {
			check("badge start", row, runeIndex(row, " "+r.badge, end)+1)
		}
	}
}

// cutPathRow returns the line that shows a path cut from the left down to
// its file name tail, or "".
func cutPathRow(lines []string, tail string) string {
	for _, l := range lines {
		if strings.Contains(l, "…") && strings.Contains(l, "/"+tail+" ") {
			return l
		}
	}

	return ""
}

// TestFilesPaneComputesCellsOnLoad checks that the Files pane draws the
// cells it computed when the list arrived: a View matches no badge rules and
// reads no file again. A new snapshot of the repository reloads the list, so
// the cells never go stale.
func TestFilesPaneComputesCellsOnLoad(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := paymentsModel(t, 120, 40)
	m = pressSettle(t, m, "3")
	want := paneInside(t, m, PaneFiles)

	// What a View would read again if it worked from the files: the rules of
	// the repository and the listed files.
	i := slices.IndexFunc(m.rows, func(r row) bool { return r.snap.Loc.Name == namePayments })
	m.rows[i].snap.BadgeRules = nil

	files := slices.Clone(m.detail.files.files)
	for k := range files {
		files[k].Path = "changed"
	}

	m.detail.files.files = files

	if diff := cmp.Diff(want, paneInside(t, m, PaneFiles)); diff != "" {
		t.Errorf("Files pane changed without a new list (-want +got):\n%s", diff)
	}
}

// BenchmarkFilesPane draws the Files pane of a list of 5,000 files, as every
// View does.
func BenchmarkFilesPane(b *testing.B) {
	files := make([]repo.FileChange, 5000)
	for i := range files {
		files[i] = repo.FileChange{
			Path: fmt.Sprintf("internal/checkout/module%04d/http/handlers/checkout.go", i), Status: "M", Added: i, Deleted: i % 7,
		}
	}

	files[3].Path = pathLock

	fake := newFakeBackend(fixtureSnapshots(fixedNow))
	m := New(b.Context(), fake, fixtureRoot)
	m.now = func() time.Time { return fixedNow }

	deliver := func(msg tea.Msg) {
		next, _ := m.Update(msg)

		got, ok := next.(Model)
		if !ok {
			b.Fatalf("Update returned %T, want ui.Model", next)
		}

		m = got
	}

	deliver(tea.WindowSizeMsg{Width: 120, Height: 40})

	for _, g := range fake.groups {
		deliver(groupRefreshedMsg{commonDir: g.CommonDir, snaps: fake.snaps[g.CommonDir]})
	}

	deliver(keyPress(keyDown))
	deliver(keyPress("3"))
	deliver(filesLoadedMsg{token: m.detail.token, files: files, spec: incomingSpec})

	if n, _ := m.fileEntries(); n != len(files) {
		b.Fatalf("Files pane lists %d files, want %d", n, len(files))
	}

	r := m.layout().Files

	b.ReportAllocs()

	for b.Loop() {
		_ = m.filesLines(r)
	}
}

// TestFilesPaneKeepsColumnsWhileScrolling checks that a list longer than its
// pane keeps its columns while it scrolls: the widest counts and the only
// badge sit below the fold, and the counts end at the same column before and
// after the end of the list comes into view.
func TestFilesPaneKeepsColumnsWhileScrolling(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	files := make([]repo.FileChange, 30)
	for i := range files {
		files[i] = repo.FileChange{Path: fmt.Sprintf("src/f%02d.go", i), Status: "M", Added: 1, Deleted: 1}
	}

	files[29] = repo.FileChange{Path: pathLock, Status: "M", Added: 12345, Deleted: 6789}

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 100, 24)
	detail := paymentsDetail(fixedNow)
	detail.files = files
	fake.setDetail(detail)
	m = pressSettle(t, loadAll(t, m, fake), keyDown, "3")

	deletedEnd := func(m Model, path, deleted string) int {
		t.Helper()

		row := rowOf(t, paneInside(t, m, PaneFiles), path)

		return runeIndex(row, " "+deleted, 0) + 1 + utf8.RuneCountInString(deleted)
	}

	top := deletedEnd(m, "src/f00.go", "-1")
	bottom := deletedEnd(pressSettle(t, m, keyEnd), pathLock, "-6789")

	if top <= 0 || top != bottom {
		t.Errorf("deleted counts end at column %d at the top and %d at the end, want one column", top, bottom)
	}
}

func TestFilesPaneUsesRepoBadgeRules(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	snaps := fixtureSnapshots(fixedNow)

	snaps[0].BadgeRules = append(badge.Defaults(), badge.Rule{Label: "cash", Dir: "cmd/payments"})

	m, fake := newTestModel(t, snaps, 120, 40)
	fake.setDetail(paymentsDetail(fixedNow))
	m = pressSettle(t, loadAll(t, m, fake), keyDown, "3")

	assertViewHasRow(t, m, "▸ M "+pathTotal, "+1", "-1", "cash")
	assertViewHasRow(t, m, "┃  M "+pathLock, "+120", "-80", badgeDeps)
}

func TestFilesPaneBadgesOnlyOnIncoming(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		keys      []string
		wantTitle string
		badges    bool
	}{
		{"incoming", nil, "[Incoming 12]", true},
		{"base", []string{"]"}, "[Base 27]", false},
		{"local", []string{"]", "]"}, "[Local 2]", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 120, 40)
			fake.setDetail(paymentsDetail(fixedNow))
			m = pressSettle(t, loadAll(t, m, fake), append([]string{keyDown, "3"}, tt.keys...)...)

			assertViewContains(t, m, tt.wantTitle)
			assertViewHasRow(t, m, "┃  M "+pathLock, "+120", "-80")

			if got := rowPattern("+120", "-80", badgeDeps).MatchString(viewText(m)); got != tt.badges {
				t.Errorf("Files pane shows badges = %v, want %v:\n%s", got, tt.badges, viewText(m))
			}
		})
	}
}
