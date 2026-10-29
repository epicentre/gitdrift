package ui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/repo"
)

// incomingSpec is the "All incoming" comparison of the fixture: what a pull
// of payments brings in since the merge base.
var incomingSpec = repo.DiffSpec{Kind: repo.DiffMergeBase, From: "HEAD", To: shaUpstream}

// pressCmd presses one key and returns the model with the command it
// started.
func pressCmd(t *testing.T, m Model, k string) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(keyPress(k))

	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want ui.Model", next)
	}

	return got, cmd
}

// settle runs cmds and feeds every message they produce back into the
// model, following the commands those messages start, until none is left.
func settle(t *testing.T, m Model, cmds ...tea.Cmd) Model {
	t.Helper()

	for len(cmds) > 0 {
		cmd := cmds[0]
		cmds = cmds[1:]

		for _, msg := range runCmd(cmd) {
			next, nextCmd := m.Update(msg)

			got, ok := next.(Model)
			if !ok {
				t.Fatalf("Update returned %T, want ui.Model", next)
			}

			m = got

			cmds = append(cmds, nextCmd)
		}
	}

	return m
}

// pressSettle presses each key and settles the commands it starts.
func pressSettle(t *testing.T, m Model, keys ...string) Model {
	t.Helper()

	for _, k := range keys {
		var cmd tea.Cmd

		m, cmd = pressCmd(t, m, k)
		m = settle(t, m, cmd)
	}

	return m
}

// paymentsModel is a loaded width x height model over the fixture with the
// Repos cursor on payments and its detail panes settled.
func paymentsModel(t *testing.T, width, height int) (Model, *fakeBackend) {
	t.Helper()

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), width, height)
	fake.setDetail(paymentsDetail(fixedNow))
	m = pressSettle(t, loadAll(t, m, fake), keyDown)

	if got := cursorName(m); got != namePayments {
		t.Fatalf("cursor = %q, want payments", got)
	}

	return m, fake
}

func assertCalls(t *testing.T, fake *fakeBackend, prefix string, want []string) {
	t.Helper()

	if diff := cmp.Diff(want, fake.Calls(prefix)); diff != "" {
		t.Errorf("%s calls mismatch (-want +got):\n%s", prefix, diff)
	}
}

func assertViewContains(t *testing.T, m Model, wants ...string) {
	t.Helper()

	got := viewText(m)
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("view does not contain %q:\n%s", want, got)
		}
	}
}

func TestTabTitle(t *testing.T) {
	t.Parallel()

	payments := fixturePayments(fixedNow)

	tests := []struct {
		name   string
		snap   drift.Snapshot
		active Tab
		width  int
		want   string
	}{
		{"incoming active", payments, TabIncoming, 80, "[Incoming 12]  Base 27  Local 2  Branches ✂2 ⌂6"},
		{"branches active", payments, TabBranches, 80, "Incoming 12  Base 27  Local 2  [Branches ✂2 ⌂6]"},
		{"narrow shrinks the inactive tabs", payments, TabIncoming, 36, "[Incoming 12]  Base  Loc  Br"},
		{"narrow keeps the active tab whole", payments, TabBranches, 36, "Inc  Base  Loc  [Branches ✂2 ⌂6]"},
		{"zero counts are omitted", fixtureInfra(fixedNow), TabIncoming, 80, "[Incoming]  Base  Local  Branches ✂3"},
		{"only here without gone", fixtureAuth(fixedNow), TabLocal, 80, "Incoming  Base  [Local]  Branches ⌂1"},
		{"not read yet", drift.Snapshot{}, TabBase, 80, "Incoming  [Base]  Local  Branches"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tabTitle(tt.snap, tt.active, tt.width); got != tt.want {
				t.Errorf("tabTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTabKeysCycle(t *testing.T) {
	t.Parallel()

	m, _ := paymentsModel(t, 120, 40)

	steps := []struct {
		key  string
		want Tab
	}{
		{"]", TabBase},
		{keyRight, TabLocal},
		{"l", TabBranches},
		{"]", TabIncoming},
		{keyLeft, TabBranches},
		{"h", TabLocal},
		{"[", TabBase},
	}

	for i, s := range steps {
		m = press(t, m, s.key)
		if m.detail.tab != s.want {
			t.Errorf("step %d %q: tab %d, want %d", i, s.key, m.detail.tab, s.want)
		}
	}
}

func TestDetailIgnoresStaleToken(t *testing.T) {
	t.Parallel()

	m, _ := paymentsModel(t, 120, 40)
	stale := m.detail.token - 1
	staleErr := errors.New("stale response")

	tests := []struct {
		name string
		msg  tea.Msg
	}{
		{"tick", detailTickMsg{token: stale}},
		{"commits", commitsLoadedMsg{token: stale, commits: incomingCommits(fixedNow)[:1], total: 99}},
		{"files", filesLoadedMsg{token: stale, files: []repo.FileChange{{Path: "stale.txt", Status: "M"}}}},
		{"diff", diffLoadedMsg{token: stale, text: "+stale\n", truncated: true}},
		{"only here", onlyHereLoadedMsg{token: stale, err: staleErr}},
		{"error", commitsLoadedMsg{token: stale, err: staleErr}},
	}

	everything := cmp.Exporter(func(reflect.Type) bool { return true })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			next, cmd := m.Update(tt.msg)

			got, ok := next.(Model)
			if !ok {
				t.Fatalf("Update returned %T, want ui.Model", next)
			}

			if cmd != nil {
				t.Error("a stale message started a command")
			}

			if diff := cmp.Diff(m.detail, got.detail, everything); diff != "" {
				t.Errorf("detail state changed (-before +after):\n%s", diff)
			}
		})
	}
}

func TestDetailIncomingFlow(t *testing.T) {
	t.Parallel()

	m, fake := paymentsModel(t, 120, 40)

	assertCalls(t, fake, "RangeCommits", []string{"RangeCommits payments HEAD.." + shaUpstream + " 500"})
	assertCalls(t, fake, "Files", []string{fmt.Sprintf("Files payments %+v", incomingSpec)})
	assertCalls(t, fake, "FileDiff", []string{
		fmt.Sprintf("FileDiff payments %+v %s %d", incomingSpec, pathTotal, repo.MaxDiffBytes),
	})
	assertViewContains(t, m, "2 Commits  [Incoming 12]", "All incoming (12)", "3 Files (5)", "4 Diff · "+pathTotal, totalDiffHead)

	m = press(t, m, "2")
	assertViewContains(t, m,
		"▸ All incoming (12)",
		"  a1b2c3d Merge branch 'feature/refunds'",
		"  0badc0d fix: round invoice totals",
		"  … 9 more",
		"←→ tabs · enter files · z zoom · esc back · ? help",
	)
}

func TestDetailCommitCursorUsesCommitSpec(t *testing.T) {
	t.Parallel()

	m, fake := paymentsModel(t, 120, 40)
	m = press(t, m, "2")
	m, first := pressCmd(t, m, keyDown)
	m, second := pressCmd(t, m, keyDown)
	m = settle(t, m, first, second)

	spec := commitSpecOf(shaRefunds)

	assertCalls(t, fake, "CommitSpec", []string{"CommitSpec payments " + shaRefunds})
	assertCalls(t, fake, "Files", []string{
		fmt.Sprintf("Files payments %+v", incomingSpec),
		fmt.Sprintf("Files payments %+v", spec),
	})
	assertCalls(t, fake, "FileDiff", []string{
		fmt.Sprintf("FileDiff payments %+v %s %d", incomingSpec, pathTotal, repo.MaxDiffBytes),
		fmt.Sprintf("FileDiff payments %+v %s %d", spec, pathTotal, repo.MaxDiffBytes),
	})
	assertViewContains(t, m, "▸ f00dfee feat: add refund endpoint")

	m = pressSettle(t, m, keyHome)
	assertViewContains(t, m, "▸ All incoming (12)")

	if got := len(fake.Calls("Files")); got != 3 {
		t.Errorf("Files called %d times after going back to the All entry, want 3", got)
	}
}

func TestDetailDebounce(t *testing.T) {
	t.Parallel()

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 120, 40)
	fake.setDetail(paymentsDetail(fixedNow))
	m = loadAll(t, m, fake)

	var cmds []tea.Cmd

	for range 3 {
		var cmd tea.Cmd

		m, cmd = pressCmd(t, m, keyDown)
		cmds = append(cmds, cmd)
	}

	if got := cursorName(m); got != "notifications" {
		t.Fatalf("cursor = %q, want notifications", got)
	}

	if calls := fake.Calls("RangeCommits"); len(calls) != 0 {
		t.Fatalf("RangeCommits called before the debounce delay: %v", calls)
	}

	_ = settle(t, m, cmds...)

	assertCalls(t, fake, "RangeCommits", []string{"RangeCommits notifications HEAD.." + shaUpstream + " 500"})
}

func TestDetailBaseTab(t *testing.T) {
	t.Parallel()

	m, fake := paymentsModel(t, 120, 40)
	m = pressSettle(t, m, "]")

	spec := repo.DiffSpec{Kind: repo.DiffMergeBase, From: "HEAD", To: "refs/remotes/" + refOriginMain}

	assertCalls(t, fake, "RangeCommits", []string{
		"RangeCommits payments HEAD.." + shaUpstream + " 500",
		"RangeCommits payments HEAD..refs/remotes/origin/main 500",
	})
	assertCalls(t, fake, "Files", []string{
		fmt.Sprintf("Files payments %+v", incomingSpec),
		fmt.Sprintf("Files payments %+v", spec),
	})
	assertViewContains(t, m, "2 Commits  Inc  [Base 27]  Loc  Br", "All behind base (12)")
}

func TestDetailEmptyTabs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keys []string
		want string
	}{
		{"nothing incoming", nil, "no incoming commits"},
		{"on the base branch", []string{"]"}, "on the base branch"},
		{"no local changes", []string{"]", "]"}, "no local changes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, fake := newTestModel(t, []drift.Snapshot{fixtureInfra(fixedNow)}, 120, 40)
			m = pressSettle(t, loadAll(t, m, fake), tt.keys...)

			assertViewContains(t, m, tt.want)

			if calls := append(fake.Calls("RangeCommits"), fake.Calls("Files")...); len(calls) != 0 {
				t.Errorf("an empty tab queried the backend: %v", calls)
			}
		})
	}
}

func TestLocalTab(t *testing.T) {
	t.Parallel()

	m, fake := paymentsModel(t, 120, 40)
	m = pressSettle(t, press(t, m, "]"), "]")

	worktree := repo.DiffSpec{Kind: repo.DiffWorktree}

	assertCalls(t, fake, "Files", []string{
		fmt.Sprintf("Files payments %+v", incomingSpec),
		fmt.Sprintf("Files payments %+v", worktree),
	})
	assertViewContains(t, m, "Uncommitted changes (2 files)", "3 Files (5)", "4 Diff · "+pathTotal)

	if got := fake.Calls("RangeCommits"); len(got) != 1 {
		t.Errorf("the Local tab listed commits: %v", got)
	}
}

func TestBranchesTabLoadsOnlyHere(t *testing.T) {
	t.Parallel()

	m, fake := paymentsModel(t, 120, 40)
	m = pressSettle(t, m, "[", "2")

	spec := commitSpecOf(shaGone)

	assertCalls(t, fake, "OnlyHereCommits", []string{"OnlyHereCommits payments feature/cash-rounding 500"})
	assertCalls(t, fake, "CommitSpec", []string{"CommitSpec payments " + shaGone})
	assertCalls(t, fake, "FileDiff", []string{
		fmt.Sprintf("FileDiff payments %+v %s %d", incomingSpec, pathTotal, repo.MaxDiffBytes),
		fmt.Sprintf("FileDiff payments %+v  %d", spec, repo.MaxDiffBytes),
	})
	assertViewContains(t, m,
		"3 Only here · feature/cash-rounding",
		"▸ e4f5a6b fix: cash rounding",
		"  5a6b7c8 wip: cash test",
		"4 Diff · e4f5a6b",
		"commit e4f5a6b fix: cash rounding",
	)
}

func TestBranchesTabStashRow(t *testing.T) {
	t.Parallel()

	m, fake := paymentsModel(t, 120, 40)
	m = pressSettle(t, m, "[", "2", keyEnd)

	spec := repo.StashSpec(fixturePayments(fixedNow).Work.Stashes[0])

	assertCalls(t, fake, "OnlyHereCommits", []string{"OnlyHereCommits payments feature/cash-rounding 500"})
	assertCalls(t, fake, "FileDiff", []string{
		fmt.Sprintf("FileDiff payments %+v %s %d", incomingSpec, pathTotal, repo.MaxDiffBytes),
		fmt.Sprintf("FileDiff payments %+v  %d", commitSpecOf(shaGone), repo.MaxDiffBytes),
		fmt.Sprintf("FileDiff payments %+v  %d", spec, repo.MaxDiffBytes),
	})
	assertViewContains(t, m, "3 Only here · stash@{0}", "▸ c0ffee1 WIP on main", "4 Diff · c0ffee1")
}

func TestBranchesTabSkipsHeaders(t *testing.T) {
	t.Parallel()

	m, _ := paymentsModel(t, 120, 40)
	m = pressSettle(t, m, "[", "2")

	steps := []struct{ key, want string }{
		{"", branchCashRounding},
		{keyUp, branchCashRounding},
		{keyDown, branchQRLogin},
		{keyDown, branchCheckout},
		{keyUp, branchQRLogin},
		{keyEnd, refStash},
		{keyDown, refStash},
		{keyHome, branchCashRounding},
		{keyPgDown, refStash},
		{keyPgUp, branchCashRounding},
	}

	for i, s := range steps {
		if s.key != "" {
			m = press(t, m, s.key)
		}

		item, ok := m.cursorBranchItem()
		if !ok || item.ref != s.want {
			t.Errorf("step %d %q: cursor on %q, want %q", i, s.key, item.ref, s.want)
		}
	}
}

func TestBranchesTabGoneWithoutCommits(t *testing.T) {
	t.Parallel()

	m, fake := paymentsModel(t, 120, 40)
	m = pressSettle(t, m, "[", "2", keyDown)

	if item, ok := m.cursorBranchItem(); !ok || item.ref != branchQRLogin {
		t.Fatalf("cursor on %q, want %s", item.ref, branchQRLogin)
	}

	assertCalls(t, fake, "OnlyHereCommits", []string{"OnlyHereCommits payments feature/cash-rounding 500"})
	assertViewContains(t, m, "3 Only here · feature/qr-login", " no commits only on this machine")

	if title := m.diffTitle(); title != titleDiff {
		t.Errorf("Diff title = %q, want the empty %q", title, titleDiff)
	}
}

func TestDetailResortDropsInFlight(t *testing.T) {
	t.Parallel()

	m, fake := newTestModel(t, navigationSnapshots(), 120, 40)
	fake.setDetail(paymentsDetail(fixedNow))
	m = loadAll(t, m, fake)

	if got := cursorName(m); got != namePayments || m.cursorMoved {
		t.Fatalf("cursor = %q (moved %t), want an unmoved cursor on payments", got, m.cursorMoved)
	}

	// The tick of payments starts its queries; their answers are still in
	// flight when another group's refresh sorts auth above payments.
	next, inFlight := m.Update(detailTickMsg{token: m.detail.token})

	m, ok := next.(Model)
	if !ok || inFlight == nil {
		t.Fatalf("the tick started no queries (%T)", next)
	}

	token := m.detail.token

	diverged := fixtureAuth(fixedNow)
	diverged.Status.Ahead, diverged.Status.Behind, diverged.Status.UpstreamSHA = 1, 1, shaUpstream
	m = step(t, m, groupRefreshedMsg{commonDir: diverged.Loc.CommonDir, snaps: []drift.Snapshot{diverged}})

	if got := cursorName(m); got != nameAuth {
		t.Fatalf("cursor = %q after the re-sort, want auth", got)
	}

	if m.detail.token == token {
		t.Fatal("the re-sort did not start a new load generation")
	}

	for _, msg := range runCmd(inFlight) {
		m = step(t, m, msg)
	}

	assertStillLoading(t, m)
}

// assertStillLoading checks that the Commits and Files panes still wait for
// their own answers and show none of the fixture's payments data.
func assertStillLoading(t *testing.T, m Model) {
	t.Helper()

	if c := m.detail.commits; !c.loading || len(c.commits) != 0 {
		t.Errorf("commits = %+v, want still loading and no payments commits", c)
	}

	if f := m.detail.files; !f.loading || len(f.files) != 0 {
		t.Errorf("files = %+v, want still loading and no payments files", f)
	}

	if got := viewText(m); strings.Contains(got, "All incoming (12)") || strings.Contains(got, pathTotal) {
		t.Errorf("payments answers reached the view:\n%s", got)
	}
}

func TestGoneStatusLine(t *testing.T) {
	t.Parallel()

	m, _ := paymentsModel(t, 120, 40)
	m = pressSettle(t, m, "[", "2")

	status := func(m Model) string {
		lines := strings.Split(viewText(m), "\n")

		return lines[len(lines)-2]
	}

	steps := []struct{ key, want string }{
		{"", "feature/cash-rounding · deleted on remote · 2 commits not on any remote"},
		{keyDown, "feature/qr-login · deleted on remote · 0 commits not on any remote"},
		{keyDown, "payments · base origin/main (auto) · diverged · local changes · 27 behind origin/main"},
	}

	for i, s := range steps {
		if s.key != "" {
			m = press(t, m, s.key)
		}

		if got := status(m); got != s.want {
			t.Errorf("step %d %q: status = %q, want %q", i, s.key, got, s.want)
		}
	}

	one := repo.GoneBranch{Name: "fix/\x1b[2Jtls", OnlyHere: 1}

	want := "fix/tls · deleted on remote · 1 commit not on any remote"
	if got := ansi.Strip(goneStatusLine(one, 120, newStyles(true))); got != want {
		t.Errorf("goneStatusLine() = %q, want %q", got, want)
	}
}

func TestDetailFollowsRefresh(t *testing.T) {
	t.Parallel()

	m, fake := paymentsModel(t, 120, 40)

	refreshed := fixturePayments(fixedNow)
	refreshed.Status.Behind = 2

	next, cmd := m.Update(groupRefreshedMsg{commonDir: refreshed.Loc.CommonDir, snaps: []drift.Snapshot{refreshed}})

	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want ui.Model", next)
	}

	_ = settle(t, got, cmd)

	if calls := fake.Calls("RangeCommits"); len(calls) != 2 {
		t.Errorf("RangeCommits calls after a refresh of the cursor repo = %v, want a second one", calls)
	}
}

// TestCommitsTitleKeepsActiveTab checks that the Commits title shows its
// active tab whole in every pane from the narrowest bottom column up, the
// pane name giving way first, and names the pane where there is room.
func TestCommitsTitleKeepsActiveTab(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := paymentsModel(t, 120, 40)

	for _, keys := range [][]string{nil, {"]"}, {"]", "]"}, {"]", "]", "]"}} {
		m := pressSettle(t, m, keys...)
		s, _ := m.detailSnapshot()
		full := joinTabs(s, m.detail.tab, false)
		active := full[strings.Index(full, "[") : strings.Index(full, "]")+1]

		for w := minLeftWidth; w <= 60; w++ {
			title := m.commitsTitle(w)
			if ansi.StringWidth(title) > w-boxTitleChrome || !strings.Contains(title, active) {
				t.Errorf("commitsTitle(%d) = %q, want at most %d cells with %s whole", w, title, w-boxTitleChrome, active)
			}
		}

		if title := m.commitsTitle(80); !strings.HasPrefix(title, titleCommits) {
			t.Errorf("commitsTitle(80) = %q, want it to start with %q", title, titleCommits)
		}
	}
}
