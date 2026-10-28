package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/repo"
)

// itemLabels returns the labels of items.
func itemLabels(items []branchItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.label
	}

	return out
}

func TestBranchItems(t *testing.T) {
	t.Parallel()

	w := fixturePayments(fixedNow).Work
	items := branchItems(w, branchCheckout, fixedNow)

	want := []string{
		headerGone,
		"[ ] feature/cash-rounding  2 only here  3w",
		"[ ] feature/qr-login       0 only here  1mo",
		headerOnlyHere,
		"feature/checkout       3 not pushed",
		"stash@{0}              WIP on main  5d",
	}

	if diff := cmp.Diff(want, itemLabels(items)); diff != "" {
		t.Fatalf("labels mismatch (-want +got):\n%s", diff)
	}

	kinds := make([]branchKind, len(items))
	refs := make([]string, len(items))

	for i, it := range items {
		kinds[i], refs[i] = it.kind, it.ref
	}

	wantKinds := []branchKind{
		branchHeader, branchGone, branchGone, branchHeader, branchLocalOnly, branchStash,
	}
	if diff := cmp.Diff(wantKinds, kinds); diff != "" {
		t.Errorf("kinds mismatch (-want +got):\n%s", diff)
	}

	wantRefs := []string{"", branchCashRounding, branchQRLogin, "", branchCheckout, refStash}
	if diff := cmp.Diff(wantRefs, refs); diff != "" {
		t.Errorf("refs mismatch (-want +got):\n%s", diff)
	}

	if items[1].gone == nil || items[1].gone.SHA != shaGone {
		t.Errorf("gone row carries %+v, want the feature/cash-rounding branch", items[1].gone)
	}

	if items[5].stash == nil || items[5].stash.SHA != shaStash {
		t.Errorf("stash row carries %+v, want stash@{0}", items[5].stash)
	}
}

func TestBranchItemLabels(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	week := fixedNow.Add(-7 * day)

	tests := []struct {
		name    string
		work    repo.Work
		current string
		want    []string
	}{
		{"nothing only here", repo.Work{}, branchMain, nil},
		{
			"long names are cut",
			repo.Work{
				Gone:      []repo.GoneBranch{{Name: "feature/a-very-long-branch-name", OnlyHere: 12, LastCommit: week}},
				LocalOnly: []repo.LocalOnlyBranch{{Name: "feature/exactly-22-chr", Commits: 1}},
			},
			branchMain,
			[]string{
				"GONE · deleted on remote",
				"[ ] feature/a-very-long-…  12 only here  1w",
				"ONLY ON THIS MACHINE",
				"feature/exactly-22-c…  1 not pushed",
			},
		},
		{
			"no upstream",
			repo.Work{LocalOnly: []repo.LocalOnlyBranch{
				{Name: "spike/token-cache", Commits: 1, NoUpstream: true},
				{Name: "spike/retry", Commits: 4, NoUpstream: true},
			}},
			branchMain,
			[]string{
				"ONLY ON THIS MACHINE",
				"spike/token-cache      1 commit, no upstream",
				"spike/retry            4 commits, no upstream",
			},
		},
		{
			"the current branch cannot be selected",
			repo.Work{Gone: []repo.GoneBranch{{Name: "fix/tls-renewal", OnlyHere: 0, LastCommit: week}}},
			"fix/tls-renewal",
			[]string{"GONE · deleted on remote", "    fix/tls-renewal        0 only here  1w"},
		},
		{
			"repository text is sanitized",
			repo.Work{
				Gone: []repo.GoneBranch{{Name: "fix/\x1b[31mred\x1b[0m", OnlyHere: 1, LastCommit: week}},
				Stashes: []repo.Stash{
					{Ref: refStash, SHA: shaStash, Subject: "WIP\x1b]0;owned\x07 on\tmain", When: fixedNow.Add(-time.Hour)},
				},
			},
			branchMain,
			[]string{
				"GONE · deleted on remote",
				"[ ] fix/red                1 only here  1w",
				"ONLY ON THIS MACHINE",
				"stash@{0}              WIP onmain  1h",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := itemLabels(branchItems(tt.work, tt.current, fixedNow))
			if len(got) == 0 {
				got = nil
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("labels mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBranchRowsFitThePane(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	w := fixturePayments(fixedNow).Work
	w.Stashes = append(w.Stashes, repo.Stash{
		Ref: "stash@{1}", SHA: shaStash, Subject: "On main: try a much longer stash subject", When: fixedNow.Add(-2 * day),
	})
	items := branchItems(w, branchCheckout, fixedNow)

	// row lays a row out as name, info and age columns of the given widths.
	row := func(lead, name string, nameW int, info string, infoW int, age string) string {
		return lead + pad(name, nameW) + "  " + pad(info, infoW) + "  " + padLeft(age, 3)
	}

	tests := []struct {
		name  string
		width int
		want  []string
	}{
		{
			"wide pane: names in full, ages at the right edge", 80,
			[]string{
				headerGone,
				row(selectBox, branchCashRounding, 21, "2 only here", 48, "3w"),
				row(selectBox, branchQRLogin, 21, "0 only here", 48, "1mo"),
				headerOnlyHere,
				row(branchIndent, branchCheckout, 21, "3 not pushed", 48, ""),
				row(branchIndent, refStash, 21, "WIP on main", 48, "5d"),
				row(branchIndent, "stash@{1}", 21, "On main: try a much longer stash subject", 48, "2d"),
			},
		},
		{
			"narrow pane cuts the stash subjects, then the names", 41,
			[]string{
				headerGone,
				row(selectBox, "feature/cash-roun…", 18, "2 only here", 12, "3w"),
				row(selectBox, branchQRLogin, 18, "0 only here", 12, "1mo"),
				headerOnlyHere,
				row(branchIndent, branchCheckout, 18, "3 not pushed", 12, ""),
				row(branchIndent, refStash, 18, "WIP on main", 12, "5d"),
				row(branchIndent, "stash@{1}", 18, "On main: tr…", 12, "2d"),
			},
		},
		{"tiny pane cuts the rest but keeps the ages", 16, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			l := newBranchLayout(items, tt.width)
			got := make([]string, len(items))

			for i, it := range items {
				row := it.row(l, newStyles(true))
				if w := ansi.StringWidth(row); w > tt.width || (it.kind != branchHeader && w != tt.width) {
					t.Errorf("row %d width = %d, want %d", i, w, tt.width)
				}

				got[i] = ansi.Strip(row)
			}

			if tt.want == nil {
				for i, age := range map[int]string{1: "3w", 2: "1mo", 5: "5d", 6: "2d"} {
					if !strings.HasSuffix(got[i], age) {
						t.Errorf("row %d = %q, want it to end with its age %s", i, got[i], age)
					}
				}

				return
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("rows mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestBranchesPaneColumns checks the Branches tab of a wide pane: names in
// full, the counts and subjects in one column, and every age at the pane's
// right edge.
func TestBranchesPaneColumns(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	snaps := withSnap(fixtureSnapshots(fixedNow), namePayments, func(s *drift.Snapshot) {
		s.Work.Gone = append(s.Work.Gone, repo.GoneBranch{
			Name: branchLong, SHA: shaGoneOld, Remote: remoteOrigin,
			RemoteRef: "refs/heads/" + branchLong, OnlyHere: 12, LastCommit: fixedNow.Add(-2 * day),
		})
	})
	m, _ := branchesModel(t, snaps, 140, 40)
	lines := paneInside(t, m, PaneCommits)

	rows := []struct{ name, info, age string }{
		{branchCashRounding, "2 only here", "3w"},
		{branchQRLogin, "0 only here", "1mo"},
		{branchLong, "12 only here", "2d"},
		{branchCheckout, "3 not pushed", ""},
		{refStash, "WIP on main", "5d"},
	}

	infoAt := -1

	for _, r := range rows {
		row := rowOf(t, lines, r.name)

		i := runeIndex(row, " "+r.info, 0) + 1
		if infoAt >= 0 && i != infoAt {
			t.Errorf("info of %q starts at column %d, want %d:\n%s", row, i, infoAt, strings.Join(lines, "\n"))
		}

		infoAt = i

		if r.age != "" && !strings.HasSuffix(row, " "+r.age) {
			t.Errorf("row %q does not end with its age %s at the right edge", row, r.age)
		}
	}
}

// TestBranchesPaneKeepsColumnsWhileScrolling checks that a Branches tab
// longer than its pane keeps its columns while it scrolls: the longest name
// sits below the fold, and the counts start at the same column before and
// after the end of the list comes into view.
func TestBranchesPaneKeepsColumnsWhileScrolling(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	const longest = "feature/a-much-longer-local-branch"

	snaps := withSnap(fixtureSnapshots(fixedNow), namePayments, func(s *drift.Snapshot) {
		s.Work.Stashes = nil
		s.Work.LocalOnly = nil

		for i := range 30 {
			s.Work.LocalOnly = append(s.Work.LocalOnly, repo.LocalOnlyBranch{Name: fmt.Sprintf("spike/b-%02d", i), Commits: 1})
		}

		s.Work.LocalOnly = append(s.Work.LocalOnly, repo.LocalOnlyBranch{Name: longest, Commits: 7})
	})
	m, _ := branchesModel(t, snaps, 140, 24)

	infoAt := func(m Model, name, info string) int {
		t.Helper()

		row := rowOf(t, paneInside(t, m, PaneCommits), name)

		return runeIndex(row, " "+info, 0) + 1
	}

	top := infoAt(m, branchCashRounding, "2 only here")
	bottom := infoAt(pressSettle(t, m, keyEnd), longest, "7 not pushed")

	if top <= 0 || top != bottom {
		t.Errorf("counts start at column %d at the top and %d at the end, want one column", top, bottom)
	}
}

func TestViewGoldenBranches(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := paymentsModel(t, 100, 24)
	m = pressSettle(t, m, "[", "2")

	assertGolden(t, "view_branches.golden", viewText(m)+"\n")
}
