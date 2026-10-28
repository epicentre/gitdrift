package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/epicentre/gitdrift/internal/badge"
	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/ops"
	"github.com/epicentre/gitdrift/internal/repo"
	"github.com/epicentre/gitdrift/internal/workspace"
)

// Fixture constants shared by the UI tests and golden files.
const (
	fixtureHome = "/home/dev"
	fixtureRoot = "/home/dev/code/work"

	namePayments   = "payments"
	nameAuth       = "auth"
	nameAPIGateway = "api-gateway"
	badgeDeps      = "deps"

	remoteOrigin  = "origin"
	branchMain    = "main"
	refOriginMain = "origin/main"

	shaHead     = "3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d"
	shaUpstream = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0"
	shaBase     = "9f8e7d6c5b4a39281706f5e4d3c2b1a098765432"
	shaGone     = "e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3"
	shaGoneOld  = "b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6"
	shaStash    = "c0ffee1234567890abcdef1234567890abcdef12"

	branchCheckout     = "feature/checkout"
	branchCashRounding = "feature/cash-rounding"
	branchQRLogin      = "feature/qr-login"
	refStash           = "stash@{0}"
)

// fixedNow is the clock of every UI test.
var fixedNow = time.Date(2026, time.October, 27, 12, 0, 0, 0, time.UTC)

// fakeBackend is an in-memory Backend that records every call.
type fakeBackend struct {
	mu     sync.Mutex
	groups []workspace.Group
	snaps  map[string][]drift.Snapshot
	detail fakeDetail
	calls  []string
	// pulls and deletes are the outcomes of Pull by repository name and of
	// DeleteGone by branch; a branch without one is deleted.
	pulls   map[string]ops.PullOutcome
	deletes map[string]ops.DeleteOutcome
	// remoteBranches is what RemoteBranches lists.
	remoteBranches []string
}

// fakeDetail is what the fake backend serves to the detail panes, whatever
// the repository and range asked for.
type fakeDetail struct {
	commits []repo.Commit
	total   int
	files   []repo.FileChange
	// onlyHere maps a branch name to its commits that no remote has.
	onlyHere map[string][]repo.Commit
	// diffs maps a file path, or the To side of the spec for a whole diff,
	// to its text; truncated marks the keys whose text was cut.
	diffs     map[string]string
	truncated map[string]bool
}

// setDetail replaces what the detail queries return.
func (f *fakeBackend) setDetail(d fakeDetail) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.detail = d
}

// setPullOutcome makes Pull of the repository name return o.
func (f *fakeBackend) setPullOutcome(name string, o ops.PullOutcome) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.pulls == nil {
		f.pulls = make(map[string]ops.PullOutcome)
	}

	f.pulls[name] = o
}

// setDeleteOutcome makes DeleteGone of branch return o.
func (f *fakeBackend) setDeleteOutcome(branch string, o ops.DeleteOutcome) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.deletes == nil {
		f.deletes = make(map[string]ops.DeleteOutcome)
	}

	f.deletes[branch] = o
}

// setRemoteBranches replaces what RemoteBranches lists.
func (f *fakeBackend) setRemoteBranches(refs []string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.remoteBranches = refs
}

func (f *fakeBackend) detailData() fakeDetail {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.detail
}

var _ Backend = (*fakeBackend)(nil)

// newFakeBackend groups snaps by common directory, in order of first
// appearance, and serves them from Refresh.
func newFakeBackend(snaps []drift.Snapshot) *fakeBackend {
	f := &fakeBackend{snaps: make(map[string][]drift.Snapshot)}

	for _, s := range snaps {
		dir := s.Loc.CommonDir

		i := slices.IndexFunc(f.groups, func(g workspace.Group) bool { return g.CommonDir == dir })
		if i < 0 {
			f.groups = append(f.groups, workspace.Group{CommonDir: dir})
			i = len(f.groups) - 1
		}

		f.groups[i].Members = append(f.groups[i].Members, s.Loc)
		f.snaps[dir] = append(f.snaps[dir], s)
	}

	return f
}

func (f *fakeBackend) record(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

// Calls returns the recorded calls whose text starts with prefix.
func (f *fakeBackend) Calls(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []string

	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}

	return out
}

func (f *fakeBackend) Groups() []workspace.Group {
	f.record("Groups")

	return f.groups
}

func (f *fakeBackend) Refresh(_ context.Context, commonDir string, fetch bool) []drift.Snapshot {
	f.record("Refresh %s fetch=%t", commonDir, fetch)

	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.snaps[commonDir])
}

func (f *fakeBackend) RangeCommits(
	_ context.Context, loc repo.Location, exclude, include string, limit int,
) ([]repo.Commit, int, error) {
	f.record("RangeCommits %s %s..%s %d", loc.Name, exclude, include, limit)

	d := f.detailData()

	return slices.Clone(d.commits), d.total, nil
}

func (f *fakeBackend) OnlyHereCommits(
	_ context.Context, loc repo.Location, ref string, limit int,
) ([]repo.Commit, error) {
	f.record("OnlyHereCommits %s %s %d", loc.Name, ref, limit)

	return slices.Clone(f.detailData().onlyHere[ref]), nil
}

func (f *fakeBackend) CommitSpec(_ context.Context, loc repo.Location, sha string) (repo.DiffSpec, error) {
	f.record("CommitSpec %s %s", loc.Name, sha)

	return commitSpecOf(sha), nil
}

// commitSpecOf is the spec the fake returns for a commit: the commit
// against its first parent.
func commitSpecOf(sha string) repo.DiffSpec {
	return repo.DiffSpec{Kind: repo.DiffRange, From: sha + "^", To: sha}
}

func (f *fakeBackend) Files(_ context.Context, loc repo.Location, spec repo.DiffSpec) ([]repo.FileChange, error) {
	f.record("Files %s %+v", loc.Name, spec)

	return slices.Clone(f.detailData().files), nil
}

func (f *fakeBackend) FileDiff(
	_ context.Context, loc repo.Location, spec repo.DiffSpec, path, oldPath string, maxBytes int,
) (string, bool, error) {
	if oldPath != "" {
		f.record("FileDiff %s %+v %s from %s %d", loc.Name, spec, path, oldPath, maxBytes)
	} else {
		f.record("FileDiff %s %+v %s %d", loc.Name, spec, path, maxBytes)
	}

	key := path
	if key == "" {
		key = spec.To
	}

	d := f.detailData()

	return d.diffs[key], d.truncated[key], nil
}

func (f *fakeBackend) RemoteBranches(_ context.Context, loc repo.Location) ([]string, error) {
	f.record("RemoteBranches %s", loc.Name)

	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.remoteBranches), nil
}

func (f *fakeBackend) Pull(_ context.Context, req ops.PullRequest) ops.PullOutcome {
	f.record("Pull %s %s %s..%s fetched=%t", req.Loc.Name, req.Branch, req.HeadSHA, req.UpstreamSHA, req.Fetch.Done)

	f.mu.Lock()
	defer f.mu.Unlock()

	return f.pulls[req.Loc.Name]
}

func (f *fakeBackend) DeleteGone(_ context.Context, req ops.DeleteRequest) ops.DeleteOutcome {
	f.record("DeleteGone %s %s %s %s %s", req.Loc.Name, req.Branch, req.SHA, req.Remote, req.RemoteRef)

	f.mu.Lock()
	defer f.mu.Unlock()

	if o, ok := f.deletes[req.Branch]; ok {
		return o
	}

	return ops.DeleteOutcome{Branch: req.Branch, SHA: req.SHA, Deleted: true}
}

func (f *fakeBackend) SetBase(_ context.Context, loc repo.Location, value string) error {
	f.record("SetBase %s %s", loc.Name, value)

	return nil
}

func (f *fakeBackend) ResetBase(_ context.Context, loc repo.Location) error {
	f.record("ResetBase %s", loc.Name)

	return nil
}

// fixtureLoc is the location of a fixture repository under fixtureRoot.
func fixtureLoc(name string) repo.Location {
	path := fixtureRoot + "/" + name

	return repo.Location{
		ID:        repo.ID(path),
		Path:      path,
		Name:      name,
		GitDir:    path + "/.git",
		CommonDir: path + "/.git",
	}
}

// tracking is a clean, fetched snapshot of name on branch, tracking
// origin/<branch>, up to date, with base origin/main and the built-in badge
// rules.
func tracking(name, branch string, lastUpdate time.Time) drift.Snapshot {
	return drift.Snapshot{
		Loc: fixtureLoc(name),
		Status: repo.Status{
			Branch:            branch,
			HeadSHA:           shaHead,
			Upstream:          remoteOrigin + "/" + branch,
			UpstreamRef:       "refs/remotes/" + remoteOrigin + "/" + branch,
			UpstreamRemote:    remoteOrigin,
			UpstreamRemoteRef: "refs/heads/" + branch,
			UpstreamSHA:       shaHead,
			Remotes:           []string{remoteOrigin},
			LastUpdate:        lastUpdate,
		},
		Base: repo.Base{
			Source: repo.SourceAuto,
			Ref:    refOriginMain,
			SHA:    shaBase,
			OnBase: branch == branchMain,
		},
		Fetch:      drift.FetchState{Done: true},
		BadgeRules: badge.Defaults(),
	}
}

// behindMain is a safe snapshot on main that is behind its upstream.
func behindMain(name string, behind int, badges []string, lastUpdate time.Time) drift.Snapshot {
	s := tracking(name, branchMain, lastUpdate)
	s.Status.Behind = behind
	s.Status.UpstreamSHA = shaUpstream
	s.Badges = badges

	return s
}

func fixturePayments(now time.Time) drift.Snapshot {
	s := tracking(namePayments, branchCheckout, now.Add(-5*time.Hour))
	s.Status.Ahead = 3
	s.Status.Behind = 12
	s.Status.Tracked = 2
	s.Status.UpstreamSHA = shaUpstream
	s.Base.Behind = 27
	s.Badges = []string{badgeDeps, "mig"}
	s.Work = repo.Work{
		Gone: []repo.GoneBranch{
			{
				Name: branchCashRounding, SHA: shaGone, Remote: remoteOrigin,
				RemoteRef: "refs/heads/" + branchCashRounding, OnlyHere: 2, LastCommit: now.Add(-21 * day),
			},
			{
				Name: branchQRLogin, SHA: shaGoneOld, Remote: remoteOrigin,
				RemoteRef: "refs/heads/" + branchQRLogin, OnlyHere: 0, LastCommit: now.Add(-40 * day),
			},
		},
		LocalOnly:       []repo.LocalOnlyBranch{{Name: branchCheckout, Commits: 3}},
		Stashes:         []repo.Stash{{Ref: refStash, SHA: shaStash, Subject: "WIP on main", When: now.Add(-5 * day)}},
		OnlyHereCommits: 5,
	}

	return s
}

func fixtureBilling(now time.Time) drift.Snapshot {
	s := tracking("billing", "feature/vat", now.Add(-30*time.Hour))
	s.Base.Behind = 27

	return s
}

func fixtureAuth(now time.Time) drift.Snapshot {
	s := tracking(nameAuth, branchMain, now.Add(-15*day))
	s.Work = repo.Work{
		LocalOnly:       []repo.LocalOnlyBranch{{Name: "spike/token-cache", Commits: 1, NoUpstream: true}},
		OnlyHereCommits: 1,
	}

	return s
}

func fixtureInfra(now time.Time) drift.Snapshot {
	s := tracking("infra", branchMain, now.Add(-16*day))

	for i, name := range []string{"chore/bump-charts", "feature/old-dns", "fix/tls-renewal"} {
		s.Work.Gone = append(s.Work.Gone, repo.GoneBranch{
			Name: name, SHA: shaGoneOld, Remote: remoteOrigin, RemoteRef: "refs/heads/" + name,
			LastCommit: now.Add(-time.Duration(30+i) * day),
		})
	}

	return s
}

// Long names for the column tests: a 28-character repository on a
// 30-character branch.
const (
	nameLong   = "settlement-reconciliation-ui"
	branchLong = "feature/merchant-onboarding-v2"
)

// fixtureLong is a repository called name on branch with every status
// token of a compact row: *11 ↓2 base↓3 deps ⌂4 ✂1.
func fixtureLong(name, branch string, now time.Time) drift.Snapshot {
	s := tracking(name, branch, now.Add(-2*time.Hour))
	s.Status.Behind, s.Status.UpstreamSHA, s.Status.Untracked = 2, shaUpstream, 11
	s.Base.Behind = 3
	s.Badges = []string{badgeDeps}
	s.Work = repo.Work{
		Gone: []repo.GoneBranch{{
			Name: "fix/old-rates", SHA: shaGoneOld, Remote: remoteOrigin,
			RemoteRef: "refs/heads/fix/old-rates", LastCommit: now.Add(-40 * day),
		}},
		LocalOnly:       []repo.LocalOnlyBranch{{Name: "spike/batches", Commits: 4}},
		OnlyHereCommits: 4,
	}

	return s
}

// Wide text for the sweep: fullwidth letters, East Asian characters and
// emoji, all two cells wide, in a repository name, branches, paths and
// subjects.
const (
	nameWide   = "ｐａｙｍｅｎｔｓ-漢字"
	branchWide = "feature/漢字-ｃｈｅｃｋｏｕｔ"
	goneWide   = "fix/漢字-ｒｏｕｎｄｉｎｇ-🎉"
)

// fixtureWide is payments with wide text in every string its rows show.
func fixtureWide(now time.Time) drift.Snapshot {
	s := fixturePayments(now)
	s.Loc = fixtureLoc(nameWide)
	s.Status.Branch = branchWide
	s.Work.Gone[0].Name = goneWide
	s.Work.LocalOnly[0].Name = "feature/🚀-ｌａｕｎｃｈ"
	s.Work.Stashes[0].Subject = "WIP on 漢字 🎉"

	return s
}

// wideDetail is paymentsDetail with wide commit subjects and paths.
func wideDetail(now time.Time) fakeDetail {
	d := paymentsDetail(now)
	d.commits = []repo.Commit{
		{SHA: shaUpstream, Short: shaUpstream[:7], Subject: "Merge branch 'feature/漢字' 🎉", When: now.Add(-2 * day), Merge: true},
		{SHA: shaRefunds, Short: shaRefunds[:7], Subject: "feat: ｒｅｆｕｎｄ endpoint 🚀", When: now.Add(-3 * day)},
		{SHA: shaInvoices, Short: shaInvoices[:7], Subject: "fix: 漢字 invoice totals", When: now.Add(-400 * day)},
	}
	d.files = []repo.FileChange{
		{Path: "app/漢字/ｃｈｅｃｋｏｕｔ.php", Status: "M", Added: 3, Deleted: 1},
		{Path: "public/🚀.png", Status: "M", Binary: true},
		{Path: pathLock, Status: "M", Added: 120, Deleted: 80},
		{Path: "database/migrations/漢字_ｉｎｉｔ.php", Status: "A", Added: 9},
	}
	d.onlyHere = map[string][]repo.Commit{
		goneWide: {{SHA: shaGone, Short: shaGone[:7], Subject: "fix: 漢字 rounding 🎉", When: now.Add(-21 * day)}},
	}

	return d
}

// fixtureSnapshots is the English UI fixture used by every UI test and
// golden file, in no particular order.
func fixtureSnapshots(now time.Time) []drift.Snapshot {
	return []drift.Snapshot{
		fixturePayments(now),
		fixtureBilling(now),
		behindMain(nameAPIGateway, 12, []string{badgeDeps}, now.Add(-50*time.Hour)),
		behindMain("notifications", 4, []string{"mig", "env"}, now.Add(-75*time.Hour)),
		fixtureAuth(now),
		fixtureInfra(now),
	}
}

// headerSnapshots extends the fixture to the 32 repositories of the spec
// header: 6 behind, 2 needing attention and 3 with work only here.
func headerSnapshots(now time.Time) []drift.Snapshot {
	snaps := fixtureSnapshots(now)

	for i := 1; len(snaps) < 32; i++ {
		name := fmt.Sprintf("service-%02d", i)

		var s drift.Snapshot

		switch {
		case i <= 3:
			s = behindMain(name, 2, nil, now.Add(-time.Duration(i)*day))
		case i == 4:
			s = tracking(name, branchMain, now.Add(-day))
			s.Work.Stashes = []repo.Stash{{Ref: refStash, SHA: shaStash, Subject: "WIP on main", When: now}}
		default:
			s = tracking(name, branchMain, now.Add(-time.Duration(i)*day))
		}

		snaps = append(snaps, s)
	}

	return snaps
}

// Detail fixture values: what a pull of payments brings in and the work on
// its gone branch feature/cash-rounding.
const (
	pathTotal     = "app/Cash/Total.php"
	pathLock      = "composer.lock"
	pathLogo      = "public/logo.png"
	shaRefunds    = "f00dfeed0123456789abcdef0123456789abcdef"
	shaInvoices   = "0badc0de0123456789abcdef0123456789abcdef"
	shaCashTest   = "5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b"
	totalDiffHead = "diff --git a/app/Cash/Total.php b/app/Cash/Total.php"

	// totalDiff changes one line of app/Cash/Total.php; its lines are
	// indented with tabs.
	totalDiff = totalDiffHead + "\n" +
		"index 3f2a1b0..8c9d4e2 100644\n" +
		"--- a/app/Cash/Total.php\n" +
		"+++ b/app/Cash/Total.php\n" +
		"@@ -12,4 +12,4 @@\n" +
		" \tpublic function total(): float\n" +
		" \t{\n" +
		"-\t\treturn round($this->sum, 1);\n" +
		"+\t\treturn round($this->sum, 2);\n" +
		" \t}\n"
)

// incomingCommits are the newest commits a pull of payments brings in.
func incomingCommits(now time.Time) []repo.Commit {
	return []repo.Commit{
		{
			SHA: shaUpstream, Short: shaUpstream[:7], Subject: "Merge branch 'feature/refunds'",
			Author: "Dana", When: now.Add(-2 * day), Merge: true,
		},
		{SHA: shaRefunds, Short: shaRefunds[:7], Subject: "feat: add refund endpoint", Author: "Dana", When: now.Add(-3 * day)},
		{SHA: shaInvoices, Short: shaInvoices[:7], Subject: "fix: round invoice totals", Author: "Lee", When: now.Add(-4 * day)},
	}
}

// paymentsDetail serves 3 of 12 incoming commits, 5 changed files and the
// commits that exist only on feature/cash-rounding.
func paymentsDetail(now time.Time) fakeDetail {
	return fakeDetail{
		commits: incomingCommits(now),
		total:   12,
		files: []repo.FileChange{
			{Path: pathTotal, Status: "M", Added: 1, Deleted: 1},
			{Path: pathLock, Status: "M", Added: 120, Deleted: 80},
			{Path: "database/migrations/2026_10_01_add_refunds.php", Status: "A", Added: 40},
			{Path: ".env.example", Status: "M", Added: 2},
			{Path: pathLogo, Status: "M", Binary: true},
		},
		onlyHere: map[string][]repo.Commit{
			branchCashRounding: {
				{SHA: shaGone, Short: shaGone[:7], Subject: "fix: cash rounding", Author: "Sam", When: now.Add(-21 * day)},
				{SHA: shaCashTest, Short: shaCashTest[:7], Subject: "wip: cash test", Author: "Sam", When: now.Add(-22 * day)},
			},
		},
		diffs: map[string]string{
			pathTotal: totalDiff,
			shaGone:   totalDiff,
			shaStash:  totalDiff,
		},
	}
}
