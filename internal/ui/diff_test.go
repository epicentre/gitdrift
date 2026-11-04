package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/repo"
)

func TestDiffLineKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		line string
		want lineKind
	}{
		{"+x", lineAdd},
		{"-x", lineDel},
		{"@@ -1 +1 @@", lineHunk},
		{"diff --git a b", lineMeta},
		{"index 1..2", lineMeta},
		{"--- a/x", lineMeta},
		{"+++ b/x", lineMeta},
		{" x", lineContext},
		{"", lineContext},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			t.Parallel()

			if got := diffLineKind(tt.line); got != tt.want {
				t.Errorf("diffLineKind(%q) = %d, want %d", tt.line, got, tt.want)
			}
		})
	}
}

func TestParseDiff(t *testing.T) {
	t.Parallel()

	text := "diff --git a/db.sql b/db.sql\n" +
		"--- a/db.sql\n" +
		"+++ b/db.sql\n" +
		"@@ -1,3 +1,3 @@\n" +
		"-- old comment\n" +
		"++ new comment\n" +
		" \tSELECT 1;\n" +
		"+ab\tc\x1b[2J\x1b]0;owned\x07d\n" +
		"\\ No newline at end of file\n" +
		"diff --git a/b.txt b/b.txt\n" +
		"--- a/b.txt\n"

	want := []diffLine{
		{"diff --git a/db.sql b/db.sql", lineMeta},
		{"--- a/db.sql", lineMeta},
		{"+++ b/db.sql", lineMeta},
		{"@@ -1,3 +1,3 @@", lineHunk},
		{"-- old comment", lineDel},
		{"++ new comment", lineAdd},
		{"    SELECT 1;", lineContext},
		{"+ab cd", lineAdd},
		{"\\ No newline at end of file", lineMeta},
		{"diff --git a/b.txt b/b.txt", lineMeta},
		{"--- a/b.txt", lineMeta},
	}

	if diff := cmp.Diff(want, parseDiff(text), cmp.AllowUnexported(diffLine{})); diff != "" {
		t.Errorf("parseDiff() mismatch (-want +got):\n%s", diff)
	}

	if got := parseDiff(""); len(got) != 0 {
		t.Errorf("parseDiff(\"\") = %v, want no lines", got)
	}
}

func TestExpandTabs(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	tests := []struct{ in, want string }{
		{"\tx", "    x"},
		{"a\tb", "a   b"},
		{"abcd\te", "abcd    e"},
		{"+\t\tx", "+       x"},
		{"é\tx", "é   x"},
		{"no tabs", "no tabs"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			if got := expandTabs(tt.in); got != tt.want {
				t.Errorf("expandTabs(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCleanDiffLine(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, in, want string }{
		{"plain tab", "+a\tb", "+a  b"},
		{"escape before a tab", "+\x1b[31mab\x1b[0m\tc", "+ab c"},
		{"title escape before a tab", "-x\x1b]0;owned\x07\ty", "-x  y"},
		{"dropped runes before a tab", " \u200Bab\u202E\tc", " ab c"},
		{"wide runes before a tab", "+漢字\tx", "+漢字   x"},
		{"carriage return", "+line\r", "+line"},
		{"other controls", "+a\x07b\x7fc", "+abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := cleanDiffLine(tt.in); got != tt.want {
				t.Errorf("cleanDiffLine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDiffPaneCutsLines(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	d := diffState{lines: parseDiff("+" + strings.Repeat("x", 30) + "\n context\n")}

	got := d.render(12, 5, newStyles(true))
	if len(got) != 2 {
		t.Fatalf("render() = %d lines, want 2", len(got))
	}

	if line := ansi.Strip(got[0]); line != " +xxxxxxxxx…" {
		t.Errorf("long line = %q, want the gutter and the line cut to 12 cells with …", line)
	}

	if line := ansi.Strip(got[1]); line != "  context" {
		t.Errorf("short line = %q, want the gutter and the line unchanged", line)
	}
}

func TestDiffBinaryAndTruncated(t *testing.T) {
	t.Parallel()

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 120, 40)

	detail := paymentsDetail(fixedNow)
	detail.files = []repo.FileChange{
		{Path: pathLogo, Status: "M", Binary: true},
		{Path: pathTotal, Status: "M", Added: 1, Deleted: 1},
	}
	detail.truncated = map[string]bool{pathTotal: true}
	fake.setDetail(detail)

	m = pressSettle(t, loadAll(t, m, fake), keyDown)

	assertViewContains(t, m, "4 Diff · "+pathLogo, "binary file")
	assertViewHasRow(t, m, "M "+pathLogo, binaryLabel)

	if calls := fake.Calls("FileDiff"); len(calls) != 0 {
		t.Errorf("a binary file was diffed: %v", calls)
	}

	m = pressSettle(t, m, "3", keyDown)

	assertCalls(t, fake, "FileDiff", []string{
		fmt.Sprintf("FileDiff payments %+v %s %d", incomingSpec, pathTotal, repo.MaxDiffBytes),
	})
	assertViewContains(t, m, "4 Diff · "+pathTotal, totalDiffHead, "diff truncated at 1 MiB")

	lines := m.detail.diff.render(60, 30, newStyles(true))
	if got := ansi.Strip(lines[len(lines)-1]); got != " diff truncated at 1 MiB" {
		t.Errorf("last diff line = %q, want diff truncated at 1 MiB", got)
	}
}

func TestDiffRenamedFileGetsOldPath(t *testing.T) {
	t.Parallel()

	const renamed = "cmd/checkout/total.go"

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 120, 40)

	detail := paymentsDetail(fixedNow)
	detail.files = []repo.FileChange{{Path: renamed, OldPath: pathTotal, Status: "R", Added: 1, Deleted: 1}}
	fake.setDetail(detail)

	pressSettle(t, loadAll(t, m, fake), keyDown)

	assertCalls(t, fake, "FileDiff", []string{
		fmt.Sprintf("FileDiff payments %+v %s from %s %d", incomingSpec, renamed, pathTotal, repo.MaxDiffBytes),
	})
}

func TestDiffScroll(t *testing.T) {
	t.Parallel()

	var long strings.Builder

	long.WriteString(totalDiffHead + "\n@@ -1,40 +1,40 @@\n")

	for i := range 40 {
		fmt.Fprintf(&long, "+line %02d\n", i)
	}

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 120, 28)

	detail := paymentsDetail(fixedNow)
	detail.diffs[pathTotal] = long.String()
	detail.diffs[pathLock] = "diff --git a/go.sum b/go.sum\n"
	fake.setDetail(detail)

	m = press(t, pressSettle(t, loadAll(t, m, fake), keyDown), "4")

	// 42 lines in a Diff pane with 15 inner lines: the last offset is 27
	// and a page is 14 lines.
	steps := []struct {
		key  string
		want int
	}{
		{keyDown, 1},
		{"j", 2},
		{keyUp, 1},
		{keyPgDown, 15},
		{keyEnd, 27},
		{keyDown, 27},
		{keyPgUp, 13},
		{keyHome, 0},
		{keyUp, 0},
	}

	for i, s := range steps {
		m = press(t, m, s.key)
		if m.detail.diff.scroll != s.want {
			t.Errorf("step %d %q: scroll = %d, want %d", i, s.key, m.detail.diff.scroll, s.want)
		}
	}

	assertViewContains(t, m, "┃ +line 12")

	m = pressSettle(t, press(t, m, keyEnd), "3", keyDown)

	if m.detail.diff.scroll != 0 {
		t.Errorf("scroll = %d after a new diff loaded, want 0", m.detail.diff.scroll)
	}

	assertViewContains(t, m, "4 Diff · "+pathLock)
}

func TestDiffWideAndCRLFStayInPane(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	text := "diff --git a/docs/readme.txt b/docs/readme.txt\r\n" +
		"--- a/docs/readme.txt\r\n" +
		"+++ b/docs/readme.txt\r\n" +
		"@@ -1,2 +1,2 @@\r\n" +
		"-" + strings.Repeat("漢字", 40) + "\r\n" +
		"+" + strings.Repeat("a漢", 40) + "\rtrailing\r\n" +
		" old mac line\rmore\n"

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 100, 24)

	detail := paymentsDetail(fixedNow)
	detail.diffs[pathTotal] = text
	fake.setDetail(detail)

	m = pressSettle(t, loadAll(t, m, fake), keyDown)

	got := viewText(m)
	if !strings.Contains(got, "│ -漢字漢字") || !strings.Contains(got, "│ +a漢a漢") {
		t.Fatalf("view does not show the wide lines:\n%s", got)
	}

	if strings.Contains(got, "\r") {
		t.Error("a carriage return reached the view")
	}

	for i, line := range strings.Split(got, "\n") {
		if w := ansi.StringWidth(line); w > 100 {
			t.Errorf("line %d is %d cells wide, want at most 100: %q", i, w, line)
		}

		if strings.ContainsAny(line, "│╮╯") && ansi.StringWidth(line) != 100 {
			t.Errorf("pane line %d is %d cells wide, want 100: %q", i, ansi.StringWidth(line), line)
		}
	}
}

func TestDiffResizeKeepsScrollInBounds(t *testing.T) {
	t.Parallel()

	var long strings.Builder

	long.WriteString(totalDiffHead + "\n@@ -1,40 +1,40 @@\n")

	for i := range 40 {
		fmt.Fprintf(&long, "+line %02d\n", i)
	}

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 120, 24)

	detail := paymentsDetail(fixedNow)
	detail.diffs[pathTotal] = long.String()
	fake.setDetail(detail)

	m = press(t, pressSettle(t, loadAll(t, m, fake), keyDown), "4", keyEnd)

	// 42 lines; 11 inner lines at 120x24, 35 at 120x48.
	if m.detail.diff.scroll != 31 {
		t.Fatalf("scroll = %d at the end, want 31", m.detail.diff.scroll)
	}

	steps := []struct {
		width, height int
		scroll        int
		shows         string
	}{
		{120, 48, 7, "+line 39"},
		{120, 24, 7, "+line 05"},
		{59, 15, 7, ""},
		{80, 24, 7, "+line 05"},
		{200, 68, 0, "+line 39"},
		{120, 24, 0, totalDiffHead},
	}

	for i, s := range steps {
		m = step(t, m, tea.WindowSizeMsg{Width: s.width, Height: s.height})

		if m.detail.diff.scroll != s.scroll {
			t.Errorf("step %d %dx%d: scroll = %d, want %d", i, s.width, s.height, m.detail.diff.scroll, s.scroll)
		}

		got := viewText(m)
		if got == "" || (s.shows != "" && !strings.Contains(got, s.shows)) {
			t.Errorf("step %d %dx%d: view does not show %q:\n%s", i, s.width, s.height, s.shows, got)
		}
	}
}
