package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/ops"
	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	nameNotifications = "notifications"
	badgeMig          = "mig"
	keySpace          = "space"
	keyCtrlC          = "ctrl+c"

	// apiPullLine is the pull dialog line of api-gateway alone: 12 commits
	// behind origin/main at a1b2c3d. Among other lines its columns widen.
	apiPullLine = "api-gateway  main  ↓12 → a1b2c3d"
	// apiPullTail is the end of that line, after the name and the branch.
	apiPullTail = "↓12 → a1b2c3d"
	// cashRoundingDeleteLine introduces the commits of feature/cash-rounding
	// in the delete dialog.
	cashRoundingDeleteLine = "feature/cash-rounding (e4f5a6b), 2 commits only on this machine:"

	// The commits of notifications in TestPullConfirmAndRun, unlike those
	// of the other fixture repositories.
	shaNotifyHead = "11aa22bb33cc44dd55ee66ff77aa88bb99cc00dd"
	shaNotifyUp   = "deadbeef00112233445566778899aabbccddeeff"
)

// loadedModel is a width x height model over snaps with every group
// refreshed.
func loadedModel(t *testing.T, snaps []drift.Snapshot, width, height int) (Model, *fakeBackend) {
	t.Helper()

	m, fake := newTestModel(t, snaps, width, height)

	return loadAll(t, m, fake), fake
}

// moveTo puts the Repos cursor on the row of the repository name.
func moveTo(t *testing.T, m Model, name string) Model {
	t.Helper()

	m = press(t, m, keyHome)

	for range len(m.rows) {
		if cursorName(m) == name {
			return m
		}

		m = press(t, m, keyDown)
	}

	if got := cursorName(m); got != name {
		t.Fatalf("cursor = %q, want %q", got, name)
	}

	return m
}

// updateCmd applies msg and returns the model with the command it started.
func updateCmd(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(msg)

	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want ui.Model", next)
	}

	return got, cmd
}

// feed applies every message in order, dropping the commands they start.
func feed(t *testing.T, m Model, msgs []tea.Msg) Model {
	t.Helper()

	for _, msg := range msgs {
		m = step(t, m, msg)
	}

	return m
}

// messagesOf keeps the messages of type T.
func messagesOf[T any](msgs []tea.Msg) []T {
	var out []T

	for _, msg := range msgs {
		if v, ok := msg.(T); ok {
			out = append(out, v)
		}
	}

	return out
}

// statusText is the status line of the view: the line above the footer.
func statusText(m Model) string {
	lines := strings.Split(viewText(m), "\n")
	if len(lines) < 2 {
		return ""
	}

	return lines[len(lines)-2]
}

// callsSince returns the calls with prefix recorded after the first n.
func callsSince(fake *fakeBackend, prefix string, n int) []string {
	calls := fake.Calls(prefix)
	if n >= len(calls) {
		return nil
	}

	return calls[n:]
}

// withSnap returns snaps with mut applied to the snapshot of name.
func withSnap(snaps []drift.Snapshot, name string, mut func(*drift.Snapshot)) []drift.Snapshot {
	out := slices.Clone(snaps)

	for i := range out {
		if out[i].Loc.Name == name {
			mut(&out[i])
		}
	}

	return out
}

func refreshCall(name string, fetch bool) string {
	return fmt.Sprintf("Refresh %s fetch=%t", fixtureLoc(name).CommonDir, fetch)
}

func TestSelectOnlySafe(t *testing.T) {
	t.Parallel()

	dirty := behindMain(nameAPIGateway, 12, nil, fixedNow)
	dirty.Status.Tracked = 2
	m, fake := newTestModel(t, []drift.Snapshot{dirty, behindMain(nameNotifications, 4, nil, fixedNow)}, 120, 40)

	m = press(t, m, keySpace)
	if got := statusText(m); got != "cannot select: still fetching" {
		t.Errorf("status before the first refresh = %q, want the refusal", got)
	}

	m = loadAll(t, m, fake)

	m = press(t, moveTo(t, m, nameAPIGateway), keySpace)
	if got := statusText(m); got != "cannot select: local changes" {
		t.Errorf("status = %q, want %q", got, "cannot select: local changes")
	}

	if len(m.selected) != 0 {
		t.Errorf("selection = %v, want none", m.selected)
	}

	m = press(t, moveTo(t, m, nameNotifications), keySpace)
	if !m.selected[fixtureLoc(nameNotifications).ID] {
		t.Fatalf("notifications is not selected: %v", m.selected)
	}

	assertViewContains(t, m, "↓●notifications")

	if got := statusText(m); strings.HasPrefix(got, "cannot select") {
		t.Errorf("status = %q, want the refusal cleared by the next key", got)
	}

	m = press(t, m, keySpace)
	if len(m.selected) != 0 {
		t.Errorf("selection after a second space = %v, want none", m.selected)
	}
}

func TestPullConfirmAndRun(t *testing.T) {
	t.Parallel()

	snaps := withSnap(fixtureSnapshots(fixedNow), nameAPIGateway, func(s *drift.Snapshot) {
		s.Badges = []string{badgeDeps, badgeMig}
	})
	snaps = withSnap(snaps, nameNotifications, func(s *drift.Snapshot) {
		s.Status.HeadSHA, s.Status.UpstreamSHA = shaNotifyHead, shaNotifyUp
	})
	m, fake := loadedModel(t, snaps, 120, 40)
	fake.setPullOutcome(nameAPIGateway, ops.PullOutcome{Pulled: 12})
	fake.setPullOutcome(nameNotifications, ops.PullOutcome{Pulled: 4})

	m = press(t, moveTo(t, m, nameAPIGateway), keySpace)
	m = press(t, moveTo(t, m, nameNotifications), keySpace, "p")
	assertViewContains(t, m, "Pull 2 repositories?", "y pull · n cancel")
	assertViewHasRow(t, m, nameAPIGateway, branchMain, apiPullTail)
	assertViewHasRow(t, m, nameNotifications, branchMain, "↓ 4 → deadbee")

	m, cmd := pressCmd(t, m, "n")
	if m.dialog != nil || len(runCmd(cmd)) != 0 || len(fake.Calls("Pull")) != 0 {
		t.Fatalf("n left dialog %T or pulled: %v", m.dialog, fake.Calls("Pull"))
	}

	m, cmd = pressCmd(t, press(t, m, "p"), "y")
	done := messagesOf[pullDoneMsg](runCmd(cmd))

	assertCalls(t, fake, "Pull", []string{
		"Pull api-gateway main " + shaHead + ".." + shaUpstream + " fetched=true",
		"Pull notifications main " + shaNotifyHead + ".." + shaNotifyUp + " fetched=true",
	})

	if len(done) != 2 || len(m.selected) != 0 {
		t.Fatalf("pulls answered %d times with selection %v, want 2 answers and no selection", len(done), m.selected)
	}

	var refreshed []tea.Msg

	for _, msg := range done {
		var next tea.Cmd

		m, next = updateCmd(t, m, msg)
		refreshed = append(refreshed, runCmd(next)...)
	}

	assertCalls(t, fake, "Refresh", []string{refreshCall(nameAPIGateway, false), refreshCall(nameNotifications, false)})

	m = moveTo(t, m, nameAPIGateway)
	if got, want := statusText(m), "api-gateway · pulled 12 · deps mig"; !strings.HasPrefix(got, want) {
		t.Errorf("status = %q, want prefix %q", got, want)
	}

	// The row shows the result across its status columns, cut to them.
	assertViewHasRow(t, m, "↓ "+nameAPIGateway, branchMain, "pulled 12 · ")

	m = feed(t, m, refreshed)
	if got := statusText(m); !strings.Contains(got, "pulled 12 · deps mig") {
		t.Errorf("status after the automatic refresh = %q, want the result kept", got)
	}

	m = press(t, m, "r")
	if got := statusText(m); strings.Contains(got, "pulled") {
		t.Errorf("status after r = %q, want the result cleared", got)
	}

	if _, ok := m.results[fixtureLoc(nameNotifications).ID]; !ok {
		t.Error("r cleared the result of another group")
	}
}

func TestPullInFlight(t *testing.T) {
	t.Parallel()

	m, fake := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)
	m = press(t, moveTo(t, m, nameAPIGateway), "p")

	m, cmd := pressCmd(t, m, "y")
	assertViewHasRow(t, m, "↓ "+nameAPIGateway, branchMain, "pulling…")

	if got := statusText(m); !strings.HasPrefix(got, "api-gateway · pulling…") {
		t.Errorf("status = %q, want the pull in progress", got)
	}

	if got := statusText(press(t, m, keySpace)); got != "cannot select: pull in progress" {
		t.Errorf("space on a row being pulled: status = %q", got)
	}

	again, again2 := pressCmd(t, m, "p")
	if again.dialog != nil || again2 != nil || statusText(again) != "nothing safe to pull" {
		t.Errorf("p on a row being pulled opened %T, status %q", again.dialog, statusText(again))
	}

	m = feed(t, m, runCmd(cmd))
	if got := statusText(m); !strings.HasPrefix(got, "api-gateway · pulled 0 · deps") {
		t.Errorf("status after the pull = %q, want its result", got)
	}

	if n := len(fake.Calls("Pull")); n != 1 {
		t.Errorf("Pull called %d times, want 1", n)
	}
}

func TestPullAllSafe(t *testing.T) {
	t.Parallel()

	m, _ := loadedModel(t, []drift.Snapshot{
		fixturePayments(fixedNow),
		behindMain(nameAPIGateway, 12, []string{badgeDeps}, fixedNow),
	}, 120, 40)

	m = press(t, m, "P")
	assertViewContains(t, m, "Pull 1 repository?", apiPullLine)

	if got := viewText(m); strings.Contains(got, "payments  feature/checkout") {
		t.Errorf("dialog lists the unsafe payments:\n%s", got)
	}
}

func TestPullNothingSafe(t *testing.T) {
	t.Parallel()

	m, _ := loadedModel(t, []drift.Snapshot{fixturePayments(fixedNow), fixtureBilling(fixedNow)}, 120, 40)

	for _, k := range []string{"P", "p"} {
		next, cmd := pressCmd(t, m, k)
		if next.dialog != nil || cmd != nil {
			t.Errorf("%s opened %T with command %v, want nothing", k, next.dialog, cmd)
		}

		if got := statusText(next); got != "nothing safe to pull" {
			t.Errorf("%s: status = %q, want %q", k, got, "nothing safe to pull")
		}
	}
}

func TestPullResultCopy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		outcome ops.PullOutcome
		badges  []string
		want    result
	}{
		{"pulled with badges", ops.PullOutcome{Pulled: 12}, []string{badgeDeps, badgeMig}, result{text: "pulled 12 · deps mig"}},
		{"pulled without badges", ops.PullOutcome{Pulled: 1}, nil, result{text: "pulled 1"}},
		{
			"skipped",
			ops.PullOutcome{Skipped: "upstream changed since review, press r"},
			[]string{badgeDeps},
			result{text: "skipped: upstream changed since review, press r", err: true},
		},
		{
			"failed",
			ops.PullOutcome{Err: errors.New("error: Your local changes would be overwritten\n\tapp/Cash/Total.php")},
			nil,
			result{text: "pull failed: error: Your local changes would be overwritten", err: true},
		},
		{
			"reason without escapes",
			ops.PullOutcome{Skipped: "fetch failed: ori\x1b[31mgin\a"},
			nil,
			result{text: "skipped: fetch failed: origin", err: true},
		},
		{"badges without escapes", ops.PullOutcome{Pulled: 2}, []string{"de\x1b]0;x\x07ps"}, result{text: "pulled 2 · deps"}},
		{
			"git's error line with the files it lists",
			ops.PullOutcome{Err: mergeError("hint: x\nerror: Your local changes to the following files would be overwritten by merge:\n" +
				"\tapp/Cash/Total.php\n\tcomposer.\x1b[2Jlock\nPlease commit your changes or stash them before you merge.\nAborting\n")},
			nil,
			result{
				text: "pull failed: error: Your local changes to the following files would be overwritten by merge: " +
					"app/Cash/Total.php, composer.lock",
				err: true,
			},
		},
		{
			"git's file list cut after five",
			ops.PullOutcome{Err: mergeError("error: The following untracked working tree files would be overwritten by merge:\n" +
				"\ta.txt\n\tb.txt\n\tc.txt\n\td.txt\n\te.txt\n\tf.txt\nPlease move or remove them before you merge.\nAborting\n")},
			nil,
			result{
				text: "pull failed: error: The following untracked working tree files would be overwritten by merge: " +
					"a.txt, b.txt, c.txt, d.txt, e.txt +1 more",
				err: true,
			},
		},
		{
			"git's last line when none is fatal or error",
			ops.PullOutcome{Err: fmt.Errorf("ops: pull: %w", mergeError("warning: one\n  last \x1b[31mwords\n\n"))},
			nil,
			result{text: "pull failed: last words", err: true},
		},
		{
			"git error without stderr",
			ops.PullOutcome{Err: &git.CommandError{Args: []string{"merge"}, ExitCode: -1, Err: errors.New("signal: killed")}},
			nil,
			result{text: "pull failed: git merge: signal: killed", err: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := pullResult(tt.outcome, tt.badges)
			if diff := cmp.Diff(tt.want, got, cmp.AllowUnexported(result{})); diff != "" {
				t.Errorf("pullResult() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// branchesModel is the payments model with the Branches tab focused and the
// cursor on feature/cash-rounding. The Commits pane is focused before [ is
// pressed, so that [ switches its tab even when snaps have folders.
func branchesModel(t *testing.T, snaps []drift.Snapshot, width, height int) (Model, *fakeBackend) {
	t.Helper()

	m, fake := newTestModel(t, snaps, width, height)
	fake.setDetail(paymentsDetail(fixedNow))
	m = pressSettle(t, moveTo(t, loadAll(t, m, fake), namePayments), "2", "[")

	if it, ok := m.cursorBranchItem(); !ok || it.ref != branchCashRounding {
		t.Fatalf("Branches cursor is on %+v, want feature/cash-rounding", it)
	}

	return m, fake
}

// openDeleteDialog presses d and delivers the commits it loads.
func openDeleteDialog(t *testing.T, m Model) Model {
	t.Helper()

	m, cmd := pressCmd(t, m, "d")

	return feed(t, m, runCmd(cmd))
}

func TestDeleteGoneFlow(t *testing.T) {
	t.Parallel()

	m, fake := branchesModel(t, fixtureSnapshots(fixedNow), 140, 30)

	m = press(t, m, keySpace)
	assertViewContains(t, m, "[x] feature/cash-rounding")

	loaded := len(fake.Calls("OnlyHereCommits"))
	m = openDeleteDialog(t, m)

	if diff := cmp.Diff([]string{"OnlyHereCommits " + namePayments + " " + branchCashRounding + " 500"},
		callsSince(fake, "OnlyHereCommits", loaded)); diff != "" {
		t.Errorf("OnlyHereCommits calls mismatch (-want +got):\n%s", diff)
	}

	assertViewContains(t, m, "Delete 1 gone branch in payments?", cashRoundingDeleteLine, "y delete · n cancel")
	assertViewHasRow(t, m, "┃", "e4f5a6b fix: cash rounding", "3w", "┃")
	assertViewHasRow(t, m, "┃", "5a6b7c8 wip: cash test", "3w", "┃")

	m, cmd := pressCmd(t, m, "y")
	done := runCmd(cmd)

	assertCalls(t, fake, "DeleteGone", []string{
		"DeleteGone payments feature/cash-rounding " + shaGone + " origin refs/heads/feature/cash-rounding",
	})

	m = feed(t, m, done)
	assertViewContains(t, m,
		"deleted feature/cash-rounding (was e4f5a6b) · restore: git branch feature/cash-rounding "+shaGone,
		"press any key to close")

	payments := fixtureLoc(namePayments).CommonDir
	m = step(t, m, groupRefreshedMsg{commonDir: payments, snaps: fake.snaps[payments]})

	if _, ok := m.dialog.(deleteResult); !ok {
		t.Fatalf("a refresh closed the result window: dialog %T", m.dialog)
	}

	refreshes := len(fake.Calls("Refresh"))
	m, cmd = pressCmd(t, m, "x")

	if m.dialog != nil {
		t.Fatalf("a key left dialog %T open", m.dialog)
	}

	runCmd(cmd)

	if diff := cmp.Diff([]string{refreshCall(namePayments, false)}, callsSince(fake, "Refresh", refreshes)); diff != "" {
		t.Errorf("Refresh calls after closing mismatch (-want +got):\n%s", diff)
	}

	assertViewContains(t, m, "[ ] feature/cash-rounding")
}

func TestDeleteCurrentBranchRefused(t *testing.T) {
	t.Parallel()

	snaps := withSnap(fixtureSnapshots(fixedNow), namePayments, func(s *drift.Snapshot) {
		s.Status.Branch = branchCashRounding
		s.Status.UpstreamGone = true
	})
	m, _ := branchesModel(t, snaps, 120, 40)

	for _, k := range []string{keySpace, "d"} {
		next, cmd := pressCmd(t, m, k)
		if got := statusText(next); got != "current branch cannot be deleted" {
			t.Errorf("%s: status = %q, want the refusal", k, got)
		}

		if next.dialog != nil || cmd != nil || len(next.goneSel) != 0 {
			t.Errorf("%s: dialog %T, command %v, selection %v, want none", k, next.dialog, cmd, next.goneSel)
		}
	}
}

func TestDeleteOutsideBranchesTab(t *testing.T) {
	t.Parallel()

	m, fake := newTestModel(t, fixtureSnapshots(fixedNow), 120, 40)
	fake.setDetail(paymentsDetail(fixedNow))
	m = moveTo(t, loadAll(t, m, fake), namePayments)

	for _, tt := range []struct {
		name string
		keys []string
	}{
		{"repos pane", nil},
		{"incoming tab", []string{"2"}},
		{"files pane", []string{"3"}},
	} {
		next, cmd := pressCmd(t, pressSettle(t, m, tt.keys...), "d")
		if got := statusText(next); got != "open the Branches tab to delete gone branches" {
			t.Errorf("%s: status = %q, want the notice", tt.name, got)
		}

		if next.dialog != nil || cmd != nil {
			t.Errorf("%s: dialog %T, command %v, want none", tt.name, next.dialog, cmd)
		}
	}
}

func TestDeleteRefusedAfterFailedFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		fetch drift.FetchState
		want  string
	}{
		{
			"fetch failed",
			drift.FetchState{Done: true, Errors: map[string]string{remoteOrigin: "fatal: could not read from remote"}},
			"fetch failed: origin",
		},
		{"not fetched yet", drift.FetchState{}, "remote not fetched yet: origin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			snaps := withSnap(fixtureSnapshots(fixedNow), namePayments, func(s *drift.Snapshot) { s.Fetch = tt.fetch })
			m, _ := branchesModel(t, snaps, 120, 40)

			for _, k := range []string{keySpace, "d"} {
				next, cmd := pressCmd(t, m, k)
				if got := statusText(next); got != tt.want {
					t.Errorf("%s: status = %q, want %q", k, got, tt.want)
				}

				if next.dialog != nil || cmd != nil || len(next.goneSel) != 0 {
					t.Errorf("%s: dialog %T, command %v, selection %v, want none", k, next.dialog, cmd, next.goneSel)
				}
			}
		})
	}
}

func TestDeleteDialogTruncatesCommits(t *testing.T) {
	t.Parallel()

	m, fake := branchesModel(t, fixtureSnapshots(fixedNow), 120, 40)

	detail := paymentsDetail(fixedNow)
	commits := make([]repo.Commit, 7)

	for i := range commits {
		short := fmt.Sprintf("c%06d", i+1)
		commits[i] = repo.Commit{SHA: short + "ffff", Short: short, Subject: fmt.Sprintf("change %d", i+1), When: fixedNow.Add(-day)}
	}

	commits[0].Subject = "fix: \x1b[31mred\x1b[0m\ttext"
	detail.onlyHere[branchCashRounding] = commits
	fake.setDetail(detail)

	m = openDeleteDialog(t, m)
	got := viewText(m)

	if strings.Contains(m.View().Content, "\x1b[31mred") {
		t.Error("the commit subject reaches the screen with its escape sequence")
	}

	for _, want := range []string{"feature/cash-rounding (e4f5a6b), 7 commits only on this machine:", "  +2 more"} {
		if !strings.Contains(got, want) {
			t.Errorf("dialog does not contain %q:\n%s", want, got)
		}
	}

	assertViewHasRow(t, m, "┃", "c000001 fix: redtext", "1d", "┃")
	assertViewHasRow(t, m, "┃", "c000005 change 5", "1d", "┃")

	for _, unwanted := range []string{"c000006", "c000007"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("dialog contains %q:\n%s", unwanted, got)
		}
	}
}

func TestDeleteResultCopy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		outcome ops.DeleteOutcome
		want    string
	}{
		{
			"deleted",
			ops.DeleteOutcome{Branch: branchCashRounding, SHA: shaGone, Deleted: true},
			"deleted feature/cash-rounding (was e4f5a6b) · restore: git branch feature/cash-rounding " + shaGone,
		},
		{
			"deleted with a quote in the name",
			ops.DeleteOutcome{Branch: "fix/o'brien", SHA: shaGone, Deleted: true},
			`deleted fix/o'brien (was e4f5a6b) · restore: git branch 'fix/o'\''brien' ` + shaGone,
		},
		{
			"deleted with a hidden rune in the name",
			ops.DeleteOutcome{Branch: "fix/a\u202Eb", SHA: shaGone, Deleted: true},
			`deleted fix/ab (was e4f5a6b) · restore: git branch "$(printf '\146\151\170\057\141\342\200\256\142')" ` + shaGone,
		},
		{
			"skipped",
			ops.DeleteOutcome{Branch: branchQRLogin, SHA: shaGoneOld, Skipped: "branch changed since review"},
			"skipped feature/qr-login: branch changed since review",
		},
		{
			"skipped without upstream",
			ops.DeleteOutcome{Branch: branchQRLogin, SHA: shaGoneOld, Skipped: "branch has no remote-tracking upstream"},
			"skipped feature/qr-login: branch has no remote-tracking upstream",
		},
		{
			"skipped without remote",
			ops.DeleteOutcome{
				Branch: branchQRLogin, SHA: shaGoneOld,
				Skipped: "could not verify remote: branch has no remote or remote ref",
			},
			"skipped feature/qr-login: could not verify remote: branch has no remote or remote ref",
		},
		{
			"skipped with remote stderr",
			ops.DeleteOutcome{Branch: branchQRLogin, SHA: shaGoneOld, Skipped: "could not verify remote: fatal: \x1b[2Jgone\r"},
			"skipped feature/qr-login: could not verify remote: fatal: gone",
		},
		{
			"failed",
			ops.DeleteOutcome{Branch: branchQRLogin, SHA: shaGoneOld, Err: errors.New("git branch: error: cannot lock ref\nhint: x")},
			"failed feature/qr-login: git branch: error: cannot lock ref",
		},
		{
			"failed with git's message",
			ops.DeleteOutcome{Branch: branchQRLogin, SHA: shaGoneOld, Err: &git.CommandError{
				Args: []string{"branch", "-D"}, ExitCode: 1,
				Stderr: "warning: ref busy\nfatal: cannot lock ref 'refs/heads/feature/qr-login'\n",
			}},
			"failed feature/qr-login: fatal: cannot lock ref 'refs/heads/feature/qr-login'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := deleteResultLine(tt.outcome); got != tt.want {
				t.Errorf("deleteResultLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRefreshKeys(t *testing.T) {
	t.Parallel()

	m, fake := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)

	m, cmd := pressCmd(t, m, "r")
	if row := rowOf(t, strings.Split(viewText(m), "\n"), "billing"); !strings.Contains(row, "fetching…") {
		t.Errorf("billing row %q does not show fetching…", row)
	}

	if got := statusText(m); !strings.HasPrefix(got, "billing · fetching…") {
		t.Errorf("status = %q, want the refresh in progress", got)
	}

	m = feed(t, m, runCmd(cmd))
	assertCalls(t, fake, "Refresh", []string{refreshCall("billing", true)})

	_, cmd = pressCmd(t, m, "R")
	runCmd(cmd)

	want := make([]string, 0, len(fake.groups))
	for _, s := range fixtureSnapshots(fixedNow) {
		want = append(want, refreshCall(s.Loc.Name, true))
	}

	got := callsSince(fake, "Refresh", 1)
	slices.Sort(got)
	slices.Sort(want)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("R Refresh calls mismatch (-want +got):\n%s", diff)
	}
}

func TestRefreshOneAtATimePerGroup(t *testing.T) {
	t.Parallel()

	m, fake := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)
	groups := len(fake.groups)

	m, first := pressCmd(t, m, "R")
	m, second := pressCmd(t, m, "R")
	answers := runCmd(first)
	runCmd(second)

	if n := len(fake.Calls("Refresh")); n != groups {
		t.Fatalf("two R presses made %d Refresh calls, want one per group (%d)", n, groups)
	}

	var queued []tea.Msg

	for _, msg := range messagesOf[groupRefreshedMsg](answers) {
		var cmd tea.Cmd

		m, cmd = updateCmd(t, m, msg)
		queued = append(queued, runCmd(cmd)...)
	}

	if n := len(fake.Calls("Refresh")); n != 2*groups {
		t.Fatalf("after the answers %d Refresh calls, want exactly one more per group (%d)", n, 2*groups)
	}

	for _, msg := range messagesOf[groupRefreshedMsg](queued) {
		_, cmd := updateCmd(t, m, msg)
		runCmd(cmd)
	}

	if n := len(fake.Calls("Refresh")); n != 2*groups {
		t.Errorf("the queued refreshes started %d more Refresh calls, want none", n-2*groups)
	}
}

func TestHelpToggle(t *testing.T) {
	t.Parallel()

	m, _ := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)

	m = press(t, m, "?")
	assertViewContains(t, m, "pull selected or current repo", "delete gone branches", "previous tab", "next tab", "press any key to close")

	for _, k := range []string{"j", "?"} {
		next := press(t, m, k)
		if strings.Contains(viewText(next), "pull selected or current repo") || next.dialog != nil {
			t.Errorf("%s did not close the help", k)
		}

		if next.cursor != m.cursor {
			t.Errorf("%s closed the help and moved the cursor", k)
		}
	}
}

func TestDialogSwallowsKeys(t *testing.T) {
	t.Parallel()

	m, fake := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)
	m = press(t, m, "P")

	for _, k := range []string{"q", "R", keyTab, keySpace, "?", "b", "d"} {
		next, cmd := pressCmd(t, m, k)

		if _, ok := next.dialog.(pullConfirm); !ok {
			t.Errorf("%s closed the pull dialog: %T", k, next.dialog)
		}

		if next.focus != m.focus || len(next.selected) != 0 {
			t.Errorf("%s reached the dashboard", k)
		}

		for _, msg := range runCmd(cmd) {
			if _, ok := msg.(tea.QuitMsg); ok {
				t.Errorf("%s quit with the pull dialog open", k)
			}
		}
	}

	if calls := fake.Calls("Refresh"); len(calls) != 0 {
		t.Errorf("keys in the dialog refreshed: %v", calls)
	}
}

func TestPullDialogScrolls(t *testing.T) {
	t.Parallel()

	snaps := headerSnapshots(fixedNow)
	for i := range 20 {
		snaps = append(snaps, behindMain(fmt.Sprintf("worker-%02d", i), 1, nil, fixedNow))
	}

	m, _ := loadedModel(t, snaps, 100, 24)
	m = press(t, m, "P")
	assertViewContains(t, m, "Pull 25 repositories?", "… 9 more", "y pull · n cancel · ↑↓ scroll")

	m = press(t, m, keyEnd)
	assertViewHasRow(t, m, "worker-19", branchMain, "↓ 1 → a1b2c3d")

	if _, ok := m.dialog.(pullConfirm); !ok {
		t.Errorf("scrolling closed the dialog: %T", m.dialog)
	}
}

func TestActionsLeavePreviousModelUnchanged(t *testing.T) {
	t.Parallel()

	m, _ := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)
	id := fixtureLoc(nameAPIGateway).ID

	selected := press(t, moveTo(t, m, nameAPIGateway), keySpace)
	_ = press(t, selected, keySpace)

	if !selected.selected[id] {
		t.Error("unselecting on the next model changed the previous one")
	}

	pulled := step(t, selected, pullDoneMsg{id: id, outcome: ops.PullOutcome{Pulled: 3}})
	_ = press(t, pulled, "r")

	if _, ok := pulled.results[id]; !ok {
		t.Error("r on the next model cleared the result of the previous one")
	}

	if len(pulled.refreshing) != 1 {
		t.Errorf("refreshes in flight = %v, want the refresh after the pull only", pulled.refreshing)
	}

	if queued := press(t, pulled, "r"); len(queued.queued) != 1 || len(pulled.queued) != 0 {
		t.Errorf("queued refreshes: next %v, previous %v; want one on the next model only", queued.queued, pulled.queued)
	}

	branches, _ := branchesModel(t, fixtureSnapshots(fixedNow), 120, 40)
	marked := press(t, branches, keySpace)
	_ = press(t, marked, keySpace)

	if len(marked.goneSel) != 1 {
		t.Errorf("unselecting a gone branch on the next model changed the previous one: %v", marked.goneSel)
	}

	// Three windows wait behind the help; the third append leaves spare
	// capacity, which two later models must not share.
	waiting := press(t, m, "?")
	for _, name := range []string{namePayments, nameAuth, "infra"} {
		waiting = step(t, waiting, deleteDoneMsg{loc: fixtureLoc(name)})
	}

	left := step(t, waiting, deleteDoneMsg{loc: fixtureLoc(nameAPIGateway)})
	right := step(t, waiting, deleteDoneMsg{loc: fixtureLoc(nameNotifications)})

	if len(waiting.waiting) != 3 || len(left.waiting) != 4 || dialogLoc(left.waiting[3]).Name != nameAPIGateway {
		t.Fatalf("waiting windows: previous %d, next %d; want 3, then 4 ending with api-gateway",
			len(waiting.waiting), len(left.waiting))
	}

	if dialogLoc(right.waiting[3]).Name != nameNotifications {
		t.Errorf("two next models share the waiting windows: %q", dialogLoc(right.waiting[3]).Name)
	}
}

// dialogLoc names the repository of a delete dialog or result window.
func dialogLoc(d any) repo.Location {
	switch d := d.(type) {
	case deleteConfirm:
		return d.loc
	case deleteResult:
		return d.loc
	default:
		return repo.Location{}
	}
}

// mergeError is a failed git merge with stderr.
func mergeError(stderr string) error {
	return &git.CommandError{Args: []string{"merge", "--ff-only"}, ExitCode: 128, Stderr: stderr}
}

func TestPruneSelections(t *testing.T) {
	t.Parallel()

	t.Run("one repository", func(t *testing.T) {
		t.Parallel()

		m, fake := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)
		m = press(t, moveTo(t, m, nameAPIGateway), keySpace)
		m = press(t, moveTo(t, m, nameNotifications), keySpace)

		dir := fixtureLoc(nameAPIGateway).CommonDir
		dirty := withSnap(fake.snaps[dir], nameAPIGateway, func(s *drift.Snapshot) { s.Status.Tracked = 2 })
		m = step(t, m, groupRefreshedMsg{commonDir: dir, snaps: dirty})

		if got, want := statusText(m), "api-gateway unselected: local changes"; got != want {
			t.Errorf("status = %q, want %q", got, want)
		}

		if want := map[repo.ID]bool{fixtureLoc(nameNotifications).ID: true}; !cmp.Equal(want, m.selected) {
			t.Errorf("selection = %v, want only notifications", m.selected)
		}
	})

	t.Run("several repositories", func(t *testing.T) {
		t.Parallel()

		main := behindMain(nameAPIGateway, 3, nil, fixedNow)
		hotfix := behindMain("api-gateway-hotfix", 3, nil, fixedNow)
		hotfix.Loc.CommonDir = main.Loc.CommonDir

		m, _ := loadedModel(t, []drift.Snapshot{main, hotfix}, 120, 40)
		m = press(t, m, keySpace, keyDown, keySpace)

		main.Status.Behind, hotfix.Status.Behind = 0, 0
		m = step(t, m, groupRefreshedMsg{commonDir: main.Loc.CommonDir, snaps: []drift.Snapshot{main, hotfix}})

		if got := statusText(m); got != "2 repositories unselected" {
			t.Errorf("status = %q, want %q", got, "2 repositories unselected")
		}

		if len(m.selected) != 0 {
			t.Errorf("selection = %v, want none", m.selected)
		}
	})

	t.Run("gone branches", func(t *testing.T) {
		t.Parallel()

		m, fake := branchesModel(t, fixtureSnapshots(fixedNow), 120, 40)
		m = press(t, m, keySpace)

		dir := fixtureLoc(namePayments).CommonDir
		if m = step(t, m, groupRefreshedMsg{commonDir: dir, snaps: fake.snaps[dir]}); len(m.goneSel) != 1 {
			t.Fatalf("a refresh that keeps the branch dropped it: %v", m.goneSel)
		}

		merged := withSnap(fake.snaps[dir], namePayments, func(s *drift.Snapshot) { s.Work.Gone = s.Work.Gone[1:] })
		if m = step(t, m, groupRefreshedMsg{commonDir: dir, snaps: merged}); len(m.goneSel) != 0 {
			t.Errorf("selection = %v, want the branch that is gone no more dropped", m.goneSel)
		}
	})
}

func TestDeleteRefusedCheckedOutElsewhere(t *testing.T) {
	t.Parallel()

	main := fixturePayments(fixedNow)
	hotfix := fixturePayments(fixedNow)
	hotfix.Loc = fixtureLoc("payments-hotfix")
	hotfix.Loc.CommonDir = main.Loc.CommonDir
	hotfix.Status.Branch = branchCashRounding

	m, _ := branchesModel(t, []drift.Snapshot{main, hotfix}, 120, 40)

	for _, k := range []string{keySpace, "d"} {
		next, cmd := pressCmd(t, m, k)
		if got, want := statusText(next), "branch is checked out in payments-hotfix"; got != want {
			t.Errorf("%s: status = %q, want %q", k, got, want)
		}

		if next.dialog != nil || cmd != nil || len(next.goneSel) != 0 {
			t.Errorf("%s: dialog %T, command %v, selection %v, want none", k, next.dialog, cmd, next.goneSel)
		}
	}

	if m = press(t, m, keyDown, keySpace); len(m.goneSel) != 1 {
		t.Errorf("feature/qr-login, checked out nowhere, is not selectable: %v", m.goneSel)
	}
}

func TestViewGoldenPullDialog(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := loadedModel(t, fixtureSnapshots(fixedNow), 100, 24)
	m = press(t, m, "P")

	assertGolden(t, "view_pull_dialog.golden", viewText(m)+"\n")
}

func TestViewGoldenDeleteDialog(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, _ := branchesModel(t, fixtureSnapshots(fixedNow), 100, 24)
	m = openDeleteDialog(t, press(t, m, keySpace, keyDown, keySpace))

	assertGolden(t, "view_delete_dialog.golden", viewText(m)+"\n")
}

func TestViewGoldenDeleteResult(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, fake := branchesModel(t, fixtureSnapshots(fixedNow), 140, 24)
	fake.setDeleteOutcome(branchQRLogin, ops.DeleteOutcome{
		Branch: branchQRLogin, SHA: shaGoneOld, Skipped: "branch changed since review",
	})

	m = openDeleteDialog(t, press(t, m, keySpace, keyDown, keySpace))
	m, cmd := pressCmd(t, m, "y")
	m = feed(t, m, runCmd(cmd))

	assertGolden(t, "view_delete_result.golden", viewText(m)+"\n")
}

// nestedID is the ID of the repository name in folder of the nested
// fixture.
func nestedID(folder, name string) repo.ID { return fixtureLocIn(folder, name).ID }

// pullNames names the repositories the open pull dialog lists, in its
// order; none when no pull dialog is open.
func pullNames(m Model) []string {
	d, ok := m.dialog.(pullConfirm)
	if !ok {
		return nil
	}

	names := make([]string, len(d.snaps))
	for i, s := range d.snaps {
		names[i] = s.Loc.Name
	}

	return names
}

// TestPullAllOnlyVisible checks that P offers only the safe repositories the
// Repos pane shows: those of the folder tab that pass the filter.
func TestPullAllOnlyVisible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keys []string
		want []string
	}{
		{"All tab", nil, []string{nameNotifications, nameStorefront}},
		{"services tab", []string{"]"}, []string{nameNotifications}},
		{"web tab", []string{"]", "]"}, []string{nameStorefront}},
		{"filter", []string{"/", "s", "t", "o", "r", keyEnter}, []string{nameStorefront}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)
			m = press(t, press(t, m, tt.keys...), "P")

			if diff := cmp.Diff(tt.want, pullNames(m)); diff != "" {
				t.Errorf("P after %q: pull dialog mismatch (-want +got):\n%s", tt.keys, diff)
			}
		})
	}
}

// TestPullAllRespectsOnlyHere checks that with f on P offers only the
// repositories with work only here: in the nested fixture those are payments
// and search, neither safe to pull, until storefront holds a stash.
func TestPullAllRespectsOnlyHere(t *testing.T) {
	t.Parallel()

	stashed := withSnap(fixtureNested(fixedNow), nameStorefront, func(s *drift.Snapshot) {
		s.Work.Stashes = fixturePayments(fixedNow).Work.Stashes
	})

	tests := []struct {
		name   string
		snaps  []drift.Snapshot
		want   []string
		status string
	}{
		{"none safe", fixtureNested(fixedNow), nil, nothingSafeToPull},
		{"storefront with a stash", stashed, []string{nameStorefront}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, _ := loadedModel(t, tt.snaps, 150, 40)
			m = press(t, m, "f", "P")

			if diff := cmp.Diff(tt.want, pullNames(m)); diff != "" {
				t.Errorf("f, P: pull dialog mismatch (-want +got):\n%s", diff)
			}

			if m.notice != tt.status {
				t.Errorf("f, P: notice = %q, want %q", m.notice, tt.status)
			}
		})
	}
}

// TestSelectionDroppedWhenHidden checks that a tab switch or a filter that
// hides selected repositories unselects them, saying so, keeps the visible
// selection, and that p then pulls only what stays selected.
func TestSelectionDroppedWhenHidden(t *testing.T) {
	t.Parallel()

	m, fake := loadedModel(t, fixtureNested(fixedNow), 150, 40)
	m = press(t, moveTo(t, m, nameNotifications), keySpace)
	both := press(t, moveTo(t, m, nameStorefront), keySpace)

	m = press(t, both, "]")
	if got, want := statusText(m), "web/storefront unselected"; got != want {
		t.Errorf("status after ] to services = %q, want %q", got, want)
	}

	if want := map[repo.ID]bool{nestedID(folderPlatform, nameNotifications): true}; !cmp.Equal(want, m.selected) {
		t.Errorf("selection in the services tab = %v, want only notifications", m.selected)
	}

	m = press(t, m, "p")
	if diff := cmp.Diff([]string{nameNotifications}, pullNames(m)); diff != "" {
		t.Errorf("p in the services tab: pull dialog mismatch (-want +got):\n%s", diff)
	}

	_, cmd := pressCmd(t, m, "y")
	runCmd(cmd)
	assertCalls(t, fake, "Pull", []string{"Pull notifications main " + shaHead + ".." + shaUpstream + " fetched=true"})

	// z, the first character of filterNothing, already hides every row.
	m = typeFilter(t, both, filterNothing[:1])
	if got, want := statusText(m), "filter: z▏ · 2 repositories unselected"; got != want {
		t.Errorf("status after a filter that hides both = %q, want %q", got, want)
	}

	if len(m.selected) != 0 {
		t.Errorf("selection = %v, want none", m.selected)
	}
}

// TestSelectionDroppedWhileTyping types filters over a selection: a filter
// that keeps the selected repository visible keeps it selected, and the
// keystroke that hides it unselects it, saying so after the typed text. A
// dropped selection does not come back when the filter is cleared.
func TestSelectionDroppedWhileTyping(t *testing.T) {
	t.Parallel()

	id := nestedID(folderPlatform, nameNotifications)

	m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)
	selected := press(t, moveTo(t, m, nameNotifications), keySpace)

	platLine := filterPrefix + filterPlatform + filterCaret
	if m = typeFilter(t, selected, filterPlatform); !m.selected[id] || statusText(m) != platLine {
		t.Errorf("after /plat: selected %t, status %q; want selected, %q", m.selected[id], statusText(m), platLine)
	}

	steps := []struct {
		key, status string
		selected    bool
	}{
		{"/", "filter: ▏", true},
		{"s", "filter: s▏", true},
		{"t", "filter: st▏ · services/platform/notifications unselected", false},
		{"o", "filter: sto▏", false},
		{"r", "filter: stor▏", false},
		{"e", "filter: store▏", false},
		{keyEsc, "web/storefront · base origin/main (auto)", false},
	}

	m = selected
	for _, s := range steps {
		m = press(t, m, s.key)

		got := fmt.Sprintf("selected %t, status %q", m.selected[id], statusText(m))
		if want := fmt.Sprintf("selected %t, status %q", s.selected, s.status); got != want {
			t.Errorf("after %q: %s; want %s", s.key, got, want)
		}
	}
}

// TestRefreshAllVisibleGroups checks that R fetches the groups of the
// visible rows, each once and whole, also when the tab shows only one of its
// worktrees, and clears the pull results of their rows only; in the All tab
// that is every group.
func TestRefreshAllVisibleGroups(t *testing.T) {
	t.Parallel()

	const nameHotfix = "payments-hotfix"

	// hotfix is a worktree of services/payments checked out in web.
	hotfix := relocated(folderWeb, nameHotfix, fixturePayments(fixedNow))
	hotfix.Loc.CommonDir = fixtureLocIn(folderServices, namePayments).CommonDir
	snaps := append(fixtureNested(fixedNow), hotfix)

	dirs := make(map[string]string, len(snaps))
	for _, s := range snaps {
		dirs[s.Loc.Name] = s.Loc.CommonDir
	}

	tests := []struct {
		name string
		keys []string
		// groups names a repository of each group R refreshes; nil for every
		// group of the workspace.
		groups []string
		// kept are the repositories whose pull results stay.
		kept []string
	}{
		{
			"services tab",
			[]string{"]"},
			[]string{nameBilling, namePayments, nameNotifications, nameSearch},
			[]string{nameStorefront},
		},
		{"web tab", []string{"]", "]"}, []string{nameStorefront, "admin", namePayments}, nil},
		{"All tab", nil, nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, fake := loadedModel(t, snaps, 150, 40)

			for _, id := range []repo.ID{hotfix.Loc.ID, nestedID(folderWeb, nameStorefront)} {
				var cmd tea.Cmd

				m, cmd = updateCmd(t, m, pullDoneMsg{id: id, outcome: ops.PullOutcome{Pulled: 1}})
				m = settle(t, m, cmd)
			}

			before := len(fake.Calls("Refresh"))

			m, cmd := pressCmd(t, press(t, m, tt.keys...), "R")
			runCmd(cmd)

			var want []string

			for _, g := range fake.groups {
				if tt.groups == nil || slices.ContainsFunc(tt.groups, func(name string) bool { return dirs[name] == g.CommonDir }) {
					want = append(want, fmt.Sprintf("Refresh %s fetch=true", g.CommonDir))
				}
			}

			got := callsSince(fake, "Refresh", before)
			slices.Sort(got)
			slices.Sort(want)

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("R Refresh calls mismatch (-want +got):\n%s", diff)
			}

			var kept []string

			for _, r := range m.rows {
				if _, ok := m.results[r.snap.Loc.ID]; ok {
					kept = append(kept, r.snap.Loc.Name)
				}
			}

			if diff := cmp.Diff(tt.kept, kept); diff != "" {
				t.Errorf("pull results kept after R mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSelectionDroppedByOnlyHere checks that f, and a refresh under f that
// takes a repository's work only here away, unselect the selected
// repositories they hide, saying so.
func TestSelectionDroppedByOnlyHere(t *testing.T) {
	t.Parallel()

	t.Run("f", func(t *testing.T) {
		t.Parallel()

		m, _ := loadedModel(t, fixtureNested(fixedNow), 150, 40)
		m = press(t, moveTo(t, m, nameNotifications), keySpace)
		m = press(t, moveTo(t, m, nameStorefront), keySpace, "f")

		if got, want := statusText(m), "2 repositories unselected"; got != want {
			t.Errorf("status after f = %q, want %q", got, want)
		}

		if len(m.selected) != 0 {
			t.Errorf("selection after f = %v, want none", m.selected)
		}
	})

	t.Run("a refresh under f", func(t *testing.T) {
		t.Parallel()

		snaps := withSnap(fixtureNested(fixedNow), nameStorefront, func(s *drift.Snapshot) {
			s.Work.Stashes = fixturePayments(fixedNow).Work.Stashes
		})
		id := nestedID(folderWeb, nameStorefront)

		m, fake := loadedModel(t, snaps, 150, 40)
		if m = press(t, moveTo(t, press(t, m, "f"), nameStorefront), keySpace); !m.selected[id] {
			t.Fatalf("storefront with a stash is not selected under f: %v", m.selected)
		}

		dir := fixtureLocIn(folderWeb, nameStorefront).CommonDir
		dropped := withSnap(fake.snaps[dir], nameStorefront, func(s *drift.Snapshot) { s.Work.Stashes = nil })
		m = step(t, m, groupRefreshedMsg{commonDir: dir, snaps: dropped})

		if got, want := statusText(m), "web/storefront unselected"; got != want {
			t.Errorf("status after the refresh = %q, want %q", got, want)
		}

		if len(m.selected) != 0 {
			t.Errorf("selection after the refresh = %v, want none", m.selected)
		}
	})
}

// TestSelectionDropsShareOneNotice checks that one refresh that hides a
// selected repository under f and makes another one unsafe to pull counts
// both in one notice.
func TestSelectionDropsShareOneNotice(t *testing.T) {
	t.Parallel()

	stashes := fixturePayments(fixedNow).Work.Stashes
	main := behindMain(nameStorefront, 3, nil, fixedNow)
	hotfix := behindMain("storefront-hotfix", 3, nil, fixedNow)
	hotfix.Loc.CommonDir = main.Loc.CommonDir
	main.Work.Stashes, hotfix.Work.Stashes = stashes, stashes

	m, _ := loadedModel(t, []drift.Snapshot{main, hotfix}, 120, 40)
	if m = press(t, m, "f", keySpace, keyDown, keySpace); len(m.selected) != 2 {
		t.Fatalf("selection under f = %v, want both worktrees", m.selected)
	}

	// The refresh hides storefront, its stash gone, and finds local changes
	// in storefront-hotfix.
	main.Work.Stashes, hotfix.Status.Tracked = nil, 2
	m = step(t, m, groupRefreshedMsg{commonDir: main.Loc.CommonDir, snaps: []drift.Snapshot{main, hotfix}})

	if got, want := statusText(m), "2 repositories unselected"; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}

	if len(m.selected) != 0 {
		t.Errorf("selection = %v, want none", m.selected)
	}
}

// TestSameNamedReposAreToldApart checks that two repositories with one name
// in different folders are named by their folder too wherever a single one
// is named: the pull dialog, the delete dialog and its result, the base
// picker, the status line and the notices.
func TestSameNamedReposAreToldApart(t *testing.T) {
	t.Parallel()

	const nameAPI = "api"

	apis := []drift.Snapshot{
		relocated(folderServices, nameAPI, behindMain(nameAPI, 3, nil, fixedNow)),
		relocated(folderWeb, nameAPI, behindMain(nameAPI, 5, nil, fixedNow)),
	}

	t.Run("pull dialog and status line", func(t *testing.T) {
		t.Parallel()

		m, _ := loadedModel(t, apis, 120, 40)

		if got, want := statusText(m), "services/api · base origin/main"; !strings.HasPrefix(got, want) {
			t.Errorf("status = %q, want it to start with %q", got, want)
		}

		m = press(t, m, "P")
		assertViewHasRow(t, m, "┃", "services/api", "main", "↓3 → a1b2c3d", "┃")
		assertViewHasRow(t, m, "┃", "web/api", "main", "↓5 → a1b2c3d", "┃")
	})

	t.Run("notice of a hidden selection", func(t *testing.T) {
		t.Parallel()

		m, _ := loadedModel(t, apis, 120, 40)
		m = press(t, m, keySpace, keyDown, keySpace, "]")

		if got, want := statusText(m), "web/api unselected"; got != want {
			t.Errorf("status = %q, want %q", got, want)
		}
	})

	t.Run("base picker", func(t *testing.T) {
		t.Parallel()

		m, _ := loadedModel(t, apis, 120, 40)
		assertViewContains(t, press(t, moveTo(t, m, nameAPI), keyDown, "b"), "Base for web/api")
	})

	payments := []drift.Snapshot{
		relocated(folderServices, namePayments, fixturePayments(fixedNow)),
		relocated(folderWeb, namePayments, fixturePayments(fixedNow)),
	}

	t.Run("delete dialog and result", func(t *testing.T) {
		t.Parallel()

		m, _ := branchesModel(t, payments, 140, 30)
		m = openDeleteDialog(t, press(t, m, keySpace))
		assertViewContains(t, m, "Delete 1 gone branch in services/payments?")

		m, cmd := pressCmd(t, m, "y")
		assertViewContains(t, feed(t, m, runCmd(cmd)), "Gone branches in services/payments")
	})

	t.Run("checked out in another worktree", func(t *testing.T) {
		t.Parallel()

		hotfix := fixturePayments(fixedNow)
		hotfix.Loc = fixtureLocIn(folderServices, "payments-hotfix")
		hotfix.Loc.CommonDir = payments[0].Loc.CommonDir
		hotfix.Status.Branch = branchCashRounding

		m, _ := branchesModel(t, []drift.Snapshot{payments[0], hotfix}, 140, 30)

		if got, want := statusText(press(t, m, keySpace)), "branch is checked out in services/payments-hotfix"; got != want {
			t.Errorf("status = %q, want %q", got, want)
		}
	})
}
