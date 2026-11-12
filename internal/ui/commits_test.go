package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/epicentre/gitdrift/internal/repo"
)

// pad pads s with spaces to width cells.
func pad(s string, width int) string { return s + strings.Repeat(" ", width-ansi.StringWidth(s)) }

// TestCommitsPaneAgesAtRightEdge checks the commit lists of the Commits pane
// and of the Only here pane: subjects in full when there is room, every age
// ending at the pane's right edge.
func TestCommitsPaneAgesAtRightEdge(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := paymentsModel(t, 120, 40)

	tests := []struct {
		name  string
		keys  []string
		pane  Pane
		lines map[string]string
	}{
		{
			"incoming",
			[]string{"2"},
			PaneCommits,
			map[string]string{"Merge branch 'feature/refunds'": "2d", "feat: add refund endpoint": "3d", "fix: round invoice totals": "4d"},
		},
		{
			"only here",
			[]string{"[", "3"},
			PaneFiles,
			map[string]string{"fix: cash rounding": "3w", "wip: cash test": "3w"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lines := paneInside(t, pressSettle(t, m, tt.keys...), tt.pane)

			for subject, age := range tt.lines {
				row := rowOf(t, lines, subject)
				if !strings.HasSuffix(row, " "+age) {
					t.Errorf("row %q does not end with its age %s at the right edge", row, age)
				}
			}
		})
	}
}

// TestCommitsPaneKeepsColumnsWhileScrolling checks that a list longer than
// its pane keeps its columns while it scrolls: the widest age sits below the
// fold, and the subjects are cut at the same column before and after the end
// of the list comes into view.
func TestCommitsPaneKeepsColumnsWhileScrolling(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	commits := make([]repo.Commit, 30)
	for i := range commits {
		commits[i] = repo.Commit{
			SHA: fmt.Sprintf("c%06d", i), Short: fmt.Sprintf("c%06d", i),
			Subject: fmt.Sprintf("feat: change %02d with a subject too long for the pane", i), When: fixedNow.Add(-2 * day),
		}
	}

	commits[29].When = fixedNow.Add(-330 * day) // "11mo", the widest age

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 100, 24)
	detail := paymentsDetail(fixedNow)
	detail.commits, detail.total = commits, len(commits)
	fake.setDetail(detail)
	m = pressSettle(t, loadAll(t, m, fake), keyDown, "2")

	cutAt := func(m Model, sha string) int {
		t.Helper()

		row := rowOf(t, paneInside(t, m, PaneCommits), sha)

		return runeIndex(row, "…", 0)
	}

	top := cutAt(m, "c000000")
	bottom := cutAt(pressSettle(t, m, keyEnd), "c000029")

	if top < 0 || top != bottom {
		t.Errorf("subjects are cut at column %d at the top and %d at the end, want one column", top, bottom)
	}
}

// padLeft pads s with spaces on the left to width cells.
func padLeft(s string, width int) string { return strings.Repeat(" ", width-ansi.StringWidth(s)) + s }

func TestCommitRow(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	commits := incomingCommits(fixedNow)
	hostile := repo.Commit{SHA: shaRefunds, Short: "f00dfee", Subject: "fix\x1b[2J: \u202Eoops", When: fixedNow}
	noShort := repo.Commit{SHA: shaInvoices, Subject: "x", When: fixedNow.Add(-13 * month)}
	recent := repo.Commit{SHA: shaRefunds, Short: "f00dfee", Subject: "feat: refunds", When: fixedNow.Add(-2 * day)}
	older := repo.Commit{SHA: shaInvoices, Short: "0badc0d", Subject: "fix: totals", When: fixedNow.Add(-45 * day)}
	authored := recent
	authored.Author = "Jane Doe"
	longAuthor := recent
	longAuthor.Author = "Mehmet Yilmaz-Kaya"
	longSubject := authored
	longSubject.Subject = "feat: add refund endpoint"
	hostileAuthor := recent
	hostileAuthor.Subject, hostileAuthor.Author = "x", "Ev\x1b[2Je\u202E"

	tests := []struct {
		name   string
		list   []repo.Commit
		commit repo.Commit
		width  int
		want   string
	}{
		// The subject column takes the width left by the indent, the sha,
		// the gaps, the author column and the age column, as wide as the
		// list's longest author and age.
		{"merge", commits, commits[0], 50, " a1b2c3d " + pad("Merge branch 'feature/refunds'", 31) + "  Dana  2d"},
		// The subject gives way down to its minimum while the short author
		// stays whole.
		{"plain", commits, commits[1], 40, " f00dfee feat: add refund end…  Dana  3d"},
		// Too narrow for both minimums: the author column is left out.
		{"cut subject", commits, commits[1], 30, " f00dfee feat: add refund…  3d"},
		{"sanitized", []repo.Commit{hostile}, hostile, 30, " f00dfee " + pad("fix: oops", 16) + "  now"},
		{"no short sha", []repo.Commit{noShort}, noShort, 20, " 0badc0d " + pad("x", 7) + "  1y"},
		{"ages right-aligned", []repo.Commit{recent, older}, recent, 40, " f00dfee " + pad("feat: refunds", 26) + "  " + padLeft("2d", 3)},
		{"author", []repo.Commit{authored}, authored, 40, " f00dfee " + pad("feat: refunds", 17) + "  Jane Doe  2d"},
		// A long author is cut first, down to its minimum, before the
		// subject gives way.
		{"long author cut first", []repo.Commit{longAuthor}, longAuthor, 40, " f00dfee feat: refunds  Mehmet Yilm…  2d"},
		{"author left out when narrow", []repo.Commit{longSubject}, longSubject, 34, " f00dfee feat: add refund end…  2d"},
		{"sanitized author", []repo.Commit{hostileAuthor}, hostileAuthor, 30, " f00dfee " + pad("x", 12) + "  Eve  2d"},
	}

	st := newStyles(true)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := commitRow(tt.commit, newCommitLayout(tt.list, tt.width, fixedNow, st), fixedNow, st)
			if s := ansi.Strip(got); s != tt.want {
				t.Errorf("commitRow() = %q, want %q", s, tt.want)
			}

			if w := ansi.StringWidth(got); w != tt.width {
				t.Errorf("commitRow() width = %d, want %d", w, tt.width)
			}
		})
	}
}

// TestCommitRowAuthorMuted draws the author in the muted style, so that the
// subject reads first.
func TestCommitRowAuthorMuted(t *testing.T) {
	t.Parallel()

	st := newStyles(true)
	c := repo.Commit{SHA: shaRefunds, Subject: "feat: refunds", Author: "Jane Doe", When: fixedNow.Add(-2 * day)}

	got := commitRow(c, newCommitLayout([]repo.Commit{c}, 40, fixedNow, st), fixedNow, st)
	if !strings.Contains(got, st.Muted.Render("Jane Doe")) {
		t.Errorf("commitRow() = %q, want the author in the muted style", got)
	}
}

// TestCursorBar draws the row under the cursor of the focused pane in the
// cursor line style across the pane: the row's own styling gives way to the
// bar, a short row is padded to the pane width and a long one is cut.
func TestCursorBar(t *testing.T) {
	t.Parallel()

	st := newStyles(true)
	line := st.Cursor.Render(cursorMark) + st.Attention.Render("!") + " payments  " + st.Badge.Render("deps")

	tests := []struct {
		name  string
		width int
		want  string
	}{
		{"padded to the pane", 24, pad("▸! payments  deps", 24)},
		{"cut at the pane", 8, "▸! payme"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := cursorBar(line, tt.width, st)
			if want := st.CursorLine.Render(tt.want); got != want {
				t.Errorf("cursorBar() = %q, want %q", got, want)
			}

			if w := ansi.StringWidth(got); w != tt.width {
				t.Errorf("cursorBar() width = %d, want %d", w, tt.width)
			}
		})
	}
}
