package ui

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/ops"
	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	// shaMoved is a tip that changed after a dialog opened.
	shaMoved = "0123456789abcdef0123456789abcdef01234567"
	keyX     = "x"
)

// quits reports whether cmd quits the program.
func quits(cmd tea.Cmd) bool {
	for _, msg := range runCmd(cmd) {
		if _, ok := msg.(tea.QuitMsg); ok {
			return true
		}
	}

	return false
}

// resize delivers a new terminal size.
func resize(t *testing.T, m Model, width, height int) Model {
	t.Helper()

	return step(t, m, tea.WindowSizeMsg{Width: width, Height: height})
}

// cashRoundingDeleted is a deleteDoneMsg for payments: feature/cash-rounding
// deleted.
func cashRoundingDeleted() deleteDoneMsg {
	return deleteDoneMsg{
		loc:      fixtureLoc(namePayments),
		outcomes: []ops.DeleteOutcome{{Branch: branchCashRounding, SHA: shaGone, Deleted: true}},
	}
}

// deleteResultModel is the payments Branches model, width x height, showing
// the result window of a deleted feature/cash-rounding.
func deleteResultModel(t *testing.T, width, height int) (Model, *fakeBackend) {
	t.Helper()

	m, fake := branchesModel(t, fixtureSnapshots(fixedNow), width, height)
	m, cmd := pressCmd(t, openDeleteDialog(t, m), "y")
	m = feed(t, m, runCmd(cmd))

	if _, ok := m.dialog.(deleteResult); !ok {
		t.Fatalf("dialog = %T, want the delete result", m.dialog)
	}

	return m, fake
}

func TestCtrlCQuitsFromDialogs(t *testing.T) {
	t.Parallel()

	m, _ := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)

	tests := []struct {
		name string
		m    Model
	}{
		{"pull confirm", press(t, m, "P")},
		{"help", press(t, m, "?")},
		{"base picker", press(t, m, "b")},
		{"delete result", step(t, m, cashRoundingDeleted())},
		{"hidden pull confirm", resize(t, press(t, m, "P"), 50, 10)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.m.dialog == nil {
				t.Fatal("no dialog is open")
			}

			if _, cmd := pressCmd(t, tt.m, keyCtrlC); !quits(cmd) {
				t.Error("ctrl+c did not quit")
			}

			if _, cmd := pressCmd(t, tt.m, "q"); quits(cmd) {
				t.Error("q quit with a dialog open")
			}
		})
	}
}

func TestDialogsKeepWhatTheyShow(t *testing.T) {
	t.Parallel()

	t.Run("pull", func(t *testing.T) {
		t.Parallel()

		m, fake := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)
		m = press(t, m, "P")

		dir := fixtureLoc(nameAPIGateway).CommonDir
		moved := withSnap(fake.snaps[dir], nameAPIGateway, func(s *drift.Snapshot) {
			s.Status.Behind, s.Status.UpstreamSHA = 13, shaMoved
		})
		m = step(t, m, groupRefreshedMsg{commonDir: dir, snaps: moved})
		assertViewHasRow(t, m, nameAPIGateway, branchMain, apiPullTail)

		_, cmd := pressCmd(t, m, "y")
		runCmd(cmd)

		assertCalls(t, fake, "Pull", []string{
			"Pull api-gateway main " + shaHead + ".." + shaUpstream + " fetched=true",
			"Pull notifications main " + shaHead + ".." + shaUpstream + " fetched=true",
		})
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()

		m, fake := branchesModel(t, fixtureSnapshots(fixedNow), 120, 40)
		m = openDeleteDialog(t, m)

		dir := fixtureLoc(namePayments).CommonDir
		moved := withSnap(fake.snaps[dir], namePayments, func(s *drift.Snapshot) {
			s.Work.Gone = slices.Clone(s.Work.Gone)
			s.Work.Gone[0].SHA = shaMoved
		})
		m = step(t, m, groupRefreshedMsg{commonDir: dir, snaps: moved})
		assertViewContains(t, m, cashRoundingDeleteLine)

		_, cmd := pressCmd(t, m, "y")
		runCmd(cmd)

		assertCalls(t, fake, "DeleteGone", []string{
			"DeleteGone payments feature/cash-rounding " + shaGone + " origin refs/heads/feature/cash-rounding",
		})
	})
}

func TestDialogQueue(t *testing.T) {
	t.Parallel()

	t.Run("results wait behind another window", func(t *testing.T) {
		t.Parallel()

		m, fake := loadedModel(t, fixtureSnapshots(fixedNow), 140, 30)
		infra := deleteDoneMsg{
			loc:      fixtureLoc("infra"),
			outcomes: []ops.DeleteOutcome{{Branch: "chore/bump-charts", SHA: shaGoneOld, Deleted: true}},
		}
		m = step(t, step(t, press(t, m, "?"), cashRoundingDeleted()), infra)

		if _, ok := m.dialog.(helpOverlay); !ok {
			t.Fatalf("a result replaced the help: %T", m.dialog)
		}

		var cmds []tea.Cmd

		for _, want := range []string{"deleted feature/cash-rounding", "deleted chore/bump-charts", ""} {
			var cmd tea.Cmd

			m, cmd = pressCmd(t, m, keyX)
			cmds = append(cmds, cmd)

			if want == "" {
				if m.dialog != nil {
					t.Errorf("dialog %T left open", m.dialog)
				}

				continue
			}

			assertViewContains(t, m, want)
		}

		for _, cmd := range cmds {
			runCmd(cmd)
		}

		assertCalls(t, fake, "Refresh", []string{refreshCall(namePayments, false), refreshCall("infra", false)})
	})

	t.Run("a delete dialog waits behind another window", func(t *testing.T) {
		t.Parallel()

		m, _ := branchesModel(t, fixtureSnapshots(fixedNow), 120, 40)
		m, cmd := pressCmd(t, m, "d")
		m = feed(t, press(t, m, "?"), runCmd(cmd))

		if _, ok := m.dialog.(helpOverlay); !ok {
			t.Fatalf("the delete dialog replaced the help: %T", m.dialog)
		}

		m = press(t, m, keyX)
		assertViewContains(t, m, "Delete 1 gone branch in payments?", cashRoundingDeleteLine)
	})
}

func TestTinyScreenRefusesActions(t *testing.T) {
	t.Parallel()

	m, fake := loadedModel(t, fixtureSnapshots(fixedNow), 50, 10)

	for _, k := range []string{"P", "y", "p", keySpace, "?", "b", "r", "R"} {
		next, cmd := pressCmd(t, m, k)
		if next.dialog != nil || len(next.selected) != 0 {
			t.Errorf("%s acted on a screen too small to show it: dialog %T, selection %v", k, next.dialog, next.selected)
		}

		runCmd(cmd)
	}

	for _, prefix := range []string{"Pull", "Refresh", "RemoteBranches"} {
		if calls := fake.Calls(prefix); len(calls) != 0 {
			t.Errorf("%s calls on a tiny screen: %v", prefix, calls)
		}
	}

	if _, cmd := pressCmd(t, m, "q"); !quits(cmd) {
		t.Error("q does not quit on a tiny screen")
	}
}

func TestTinyScreenConfirmTakesOnlyCancel(t *testing.T) {
	t.Parallel()

	t.Run("pull", func(t *testing.T) {
		t.Parallel()

		m, fake := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)
		m = resize(t, press(t, m, "P"), 50, 10)

		for _, k := range []string{"y", keyX, keyEnter, keyEnd} {
			next, cmd := pressCmd(t, m, k)
			runCmd(cmd)

			if _, ok := next.dialog.(pullConfirm); !ok {
				t.Errorf("%s closed the hidden pull dialog: %T", k, next.dialog)
			}
		}

		if calls := fake.Calls("Pull"); len(calls) != 0 {
			t.Errorf("a hidden pull dialog pulled: %v", calls)
		}

		if next := press(t, m, "n"); next.dialog != nil {
			t.Errorf("n left the hidden dialog %T open", next.dialog)
		}
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()

		m, fake := branchesModel(t, fixtureSnapshots(fixedNow), 120, 40)
		m = resize(t, openDeleteDialog(t, m), 50, 10)

		next, cmd := pressCmd(t, m, "y")
		runCmd(cmd)

		if _, ok := next.dialog.(deleteConfirm); !ok || len(fake.Calls("DeleteGone")) != 0 {
			t.Errorf("y on the hidden delete dialog: dialog %T, calls %v", next.dialog, fake.Calls("DeleteGone"))
		}

		if next = press(t, next, keyEsc); next.dialog != nil {
			t.Errorf("esc left the hidden dialog %T open", next.dialog)
		}
	})
}

func TestTinyScreenDeleteResultStays(t *testing.T) {
	t.Parallel()

	m, fake := deleteResultModel(t, 140, 30)
	m = resize(t, m, 50, 10)

	for _, k := range []string{keyX, "n", keyEsc, "q", keyEnter} {
		next, cmd := pressCmd(t, m, k)
		runCmd(cmd)

		if _, ok := next.dialog.(deleteResult); !ok {
			t.Errorf("%s closed the hidden result window: %T", k, next.dialog)
		}
	}

	refreshes := len(fake.Calls("Refresh"))

	m, cmd := pressCmd(t, resize(t, m, 140, 30), keyX)
	runCmd(cmd)

	if m.dialog != nil {
		t.Errorf("a key after resizing back left %T open", m.dialog)
	}

	if diff := cmp.Diff([]string{refreshCall(namePayments, false)}, callsSince(fake, "Refresh", refreshes)); diff != "" {
		t.Errorf("Refresh calls after closing mismatch (-want +got):\n%s", diff)
	}
}

func TestTinyScreenPickerTakesOnlyEsc(t *testing.T) {
	t.Parallel()

	m, fake := loadedModel(t, fixtureSnapshots(fixedNow), 120, 40)
	fake.setRemoteBranches([]string{refOriginDevelop, refOriginMain})

	m = resize(t, openPicker(t, m), 50, 10)
	next, cmd := pressCmd(t, press(t, m, "d", "e", "v"), keyEnter)
	runCmd(cmd)

	if _, ok := next.dialog.(basePicker); !ok || len(fake.Calls("SetBase"))+len(fake.Calls("ResetBase")) != 0 {
		t.Errorf("enter on the hidden picker: dialog %T, writes %v %v",
			next.dialog, fake.Calls("SetBase"), fake.Calls("ResetBase"))
	}

	if next = press(t, next, keyEsc); next.dialog != nil {
		t.Errorf("esc left the hidden picker %T open", next.dialog)
	}
}

// TestDeleteDialogAlignsCommits checks the commit lines of the delete
// dialog: subjects in one column and every age right-aligned at the
// window's right edge, however long the subject and the age.
func TestDeleteDialogAlignsCommits(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	m, fake := branchesModel(t, fixtureSnapshots(fixedNow), 120, 40)

	detail := paymentsDetail(fixedNow)
	detail.onlyHere[branchCashRounding] = []repo.Commit{
		{SHA: shaRefunds, Short: shaRefunds[:7], Subject: "fix: cash rounding for every currency", When: fixedNow.Add(-21 * day)},
		{SHA: shaInvoices, Short: shaInvoices[:7], Subject: "wip", When: fixedNow.Add(-40 * day)},
	}
	fake.setDetail(detail)

	m = openDeleteDialog(t, press(t, m, keySpace))
	lines := strings.Split(viewText(m), "\n")

	subjectAt, ageEnd := -1, -1

	for _, c := range detail.onlyHere[branchCashRounding] {
		row := rowOf(t, lines, c.Short)
		age := relAge(c.When, fixedNow)

		end := runeIndex(row, " "+age+" │", 0) + 1 + utf8.RuneCountInString(age)
		if end <= utf8.RuneCountInString(age) {
			t.Fatalf("line %q does not end with its age %s at the window's edge", row, age)
		}

		subject := runeIndex(row, c.Short+" "+c.Subject, 0) + utf8.RuneCountInString(c.Short) + 1

		if subjectAt >= 0 && (subject != subjectAt || end != ageEnd) {
			t.Errorf("line %q: subject at %d and age ending at %d, want %d and %d", row, subject, end, subjectAt, ageEnd)
		}

		subjectAt, ageEnd = subject, end
	}
}

// TestPullConfirmAlignsLines checks the lines of the pull dialog: names and
// branches in columns as wide as the longest, cut with "…" in a narrow
// window, every ↓ and → at one column and the behind counts right-aligned
// between them.
func TestPullConfirmAlignsLines(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	api := behindMain(nameAPIGateway, 12, nil, fixedNow)

	vat := tracking(nameNotifications, "feature/vat", fixedNow)
	vat.Status.Behind, vat.Status.UpstreamSHA = 4, shaUpstream

	long := tracking(nameLong, branchLong, fixedNow)
	long.Status.Behind, long.Status.UpstreamSHA = 2, shaUpstream

	tests := []struct {
		name  string
		snaps []drift.Snapshot
		width int
		want  []string
	}{
		{
			"names and branches in full",
			[]drift.Snapshot{api, vat},
			60,
			[]string{
				pad(nameAPIGateway, 13) + "  " + pad(branchMain, 11) + "  ↓12 → a1b2c3d",
				pad(nameNotifications, 13) + "  " + pad("feature/vat", 11) + "  ↓ 4 → a1b2c3d",
			},
		},
		{
			"a narrow window cuts the branch, then the name",
			[]drift.Snapshot{api, long},
			52,
			[]string{
				pad(nameAPIGateway, 24) + "  " + pad(branchMain, 11) + "  ↓12 → a1b2c3d",
				"settlement-reconciliati…  feature/me…  ↓ 2 → a1b2c3d",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := pullConfirm{snaps: tt.snaps}.view(dialogEnv{st: newStyles(true)}, tt.width, 10).body
			got := make([]string, len(body))

			for i, l := range body {
				got[i] = ansi.Strip(l)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("pull dialog lines mismatch (-want +got):\n%s", diff)
			}

			for _, mark := range []string{"↓", "→"} {
				if a, b := runeIndex(got[0], mark, 0), runeIndex(got[1], mark, 0); a != b {
					t.Errorf("%s starts at column %d and %d:\n%s", mark, a, b, strings.Join(got, "\n"))
				}
			}
		})
	}
}
