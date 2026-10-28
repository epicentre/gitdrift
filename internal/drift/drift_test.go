package drift_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/repo"
)

// Reason texts asserted by the tests.
const (
	reasonDetached     = "detached HEAD"
	reasonNoUpstream   = "no upstream"
	reasonUpstreamGone = "upstream gone"
	reasonDiverged     = "diverged"
	reasonLocalChanges = "local changes"
	reasonNothing      = "nothing to pull"

	remoteOrigin  = "origin"
	refOriginMain = "origin/main"
)

// safeBehind is a snapshot that is safe to pull: on a branch, tracking
// origin/main, fetched, two commits behind, clean.
func safeBehind() drift.Snapshot {
	return drift.Snapshot{
		Loc: repo.Location{Name: "payments", Path: "/src/payments"},
		Status: repo.Status{
			Branch:         "main",
			Upstream:       refOriginMain,
			UpstreamRemote: remoteOrigin,
			Behind:         2,
		},
		Fetch: drift.FetchState{Done: true},
	}
}

func with(mut func(*drift.Snapshot)) drift.Snapshot {
	s := safeBehind()
	mut(&s)

	return s
}

func TestPullBlockers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		snap drift.Snapshot
		want []string
	}{
		{"safe", safeBehind(), nil},
		{"inspection failed", with(func(s *drift.Snapshot) { s.Err = errors.New("boom") }), []string{"inspection failed"}},
		{"detached", with(func(s *drift.Snapshot) {
			s.Status.Detached = true
			s.Status.Upstream = ""
			s.Status.UpstreamRemote = ""
		}), []string{reasonDetached}},
		{reasonNoUpstream, with(func(s *drift.Snapshot) {
			s.Status.Upstream = ""
			s.Status.UpstreamRemote = ""
		}), []string{reasonNoUpstream}},
		{reasonUpstreamGone, with(func(s *drift.Snapshot) { s.Status.UpstreamGone = true }), []string{reasonUpstreamGone}},
		{"fetch failed", with(func(s *drift.Snapshot) {
			s.Fetch.Errors = map[string]string{remoteOrigin: "timeout"}
		}), []string{"fetch failed: origin"}},
		{"fetch not done", with(func(s *drift.Snapshot) { s.Fetch.Done = false }), []string{"remote not fetched yet: origin"}},
		{"fetch not done and failed", with(func(s *drift.Snapshot) {
			s.Fetch = drift.FetchState{Errors: map[string]string{remoteOrigin: "timeout"}}
		}), []string{"fetch failed: origin"}},
		{"other remote failed", with(func(s *drift.Snapshot) {
			s.Fetch.Errors = map[string]string{"backup": "timeout"}
		}), nil},
		{"rebase", with(func(s *drift.Snapshot) { s.Status.Operation = repo.OpRebase }), []string{"rebase in progress"}},
		{reasonDiverged, with(func(s *drift.Snapshot) { s.Status.Ahead = 1 }), []string{reasonDiverged}},
		{"ahead", with(func(s *drift.Snapshot) {
			s.Status.Ahead = 1
			s.Status.Behind = 0
		}), []string{"ahead of upstream", reasonNothing}},
		{reasonNothing, with(func(s *drift.Snapshot) { s.Status.Behind = 0 }), []string{reasonNothing}},
		{reasonLocalChanges, with(func(s *drift.Snapshot) { s.Status.Tracked = 1 }), []string{reasonLocalChanges}},
		{"diverged first", with(func(s *drift.Snapshot) {
			s.Status.Ahead = 1
			s.Status.Behind = 2
		}), []string{reasonDiverged}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(tt.want, drift.PullBlockers(tt.snap)); diff != "" {
				t.Errorf("PullBlockers mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPullBlockersLocalUpstream(t *testing.T) {
	t.Parallel()

	s := drift.Snapshot{
		Loc: repo.Location{Name: "payments"},
		Status: repo.Status{
			Branch:         "feature/checkout",
			Upstream:       "main",
			UpstreamRemote: ".",
			Behind:         1,
		},
		Fetch: drift.FetchState{},
	}

	if got := drift.PullBlockers(s); got != nil {
		t.Errorf("PullBlockers = %v, want nil", got)
	}
}

func TestFetchStateOK(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		fetch  drift.FetchState
		remote string
		want   bool
	}{
		{"local always ok", drift.FetchState{}, ".", true},
		{"not done", drift.FetchState{}, remoteOrigin, false},
		{"done", drift.FetchState{Done: true}, remoteOrigin, true},
		{"error", drift.FetchState{Done: true, Errors: map[string]string{remoteOrigin: "x"}}, remoteOrigin, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.fetch.OK(tt.remote); got != tt.want {
				t.Errorf("OK(%q) = %v, want %v", tt.remote, got, tt.want)
			}
		})
	}
}

func TestAttentionReasons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		snap drift.Snapshot
		want []string
	}{
		{
			name: "diverged feature branch",
			snap: drift.Snapshot{
				Loc: repo.Location{Name: "payments"},
				Status: repo.Status{
					Branch: "feature/checkout", Upstream: "origin/feature/checkout", UpstreamRemote: remoteOrigin,
					Ahead: 3, Behind: 12, Tracked: 2,
				},
				Base:  repo.Base{Configured: "", Ref: refOriginMain, Behind: 27},
				Fetch: drift.FetchState{Done: true},
			},
			want: []string{reasonDiverged, reasonLocalChanges, "27 behind origin/main"},
		},
		{
			name: "failed remotes sorted",
			snap: with(func(s *drift.Snapshot) {
				s.Fetch.Errors = map[string]string{"upstream": "x", "broken": "y"}
			}),
			want: []string{"fetch failed: broken", "fetch failed: upstream"},
		},
		{
			name: "ahead with local changes is quiet",
			snap: with(func(s *drift.Snapshot) {
				s.Status.Ahead, s.Status.Behind, s.Status.Tracked = 1, 0, 3
			}),
			want: nil,
		},
		{
			name: "base not found",
			snap: with(func(s *drift.Snapshot) {
				s.Base = repo.Base{Configured: "develop", NotFound: true}
			}),
			want: []string{"base not found: develop"},
		},
		{
			name: "on base is quiet",
			snap: with(func(s *drift.Snapshot) { s.Base = repo.Base{Ref: refOriginMain, OnBase: true} }),
			want: nil,
		},
		{
			name: "error",
			snap: with(func(s *drift.Snapshot) { s.Err = errors.New("boom") }),
			want: []string{"error: boom"},
		},
		{
			name: "detached has no no-upstream reason",
			snap: with(func(s *drift.Snapshot) {
				s.Status.Detached = true
				s.Status.Upstream = ""
				s.Status.UpstreamRemote = ""
				s.Status.Behind = 0
			}),
			want: []string{reasonDetached},
		},
		{
			name: "no upstream on branch",
			snap: with(func(s *drift.Snapshot) {
				s.Status.Upstream = ""
				s.Status.UpstreamRemote = ""
				s.Status.Behind = 0
			}),
			want: []string{reasonNoUpstream},
		},
		{
			name: "unborn without upstream",
			snap: drift.Snapshot{Status: repo.Status{Branch: "main", Unborn: true}},
			want: []string{reasonNoUpstream},
		},
		{
			name: "operation",
			snap: with(func(s *drift.Snapshot) { s.Status.Operation = repo.OpMerge }),
			want: []string{"merge in progress"},
		},
		{
			name: reasonUpstreamGone,
			snap: with(func(s *drift.Snapshot) {
				s.Status.UpstreamGone = true
				s.Status.Behind = 0
			}),
			want: []string{reasonUpstreamGone},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(tt.want, drift.AttentionReasons(tt.snap)); diff != "" {
				t.Errorf("AttentionReasons mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCategoryOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		snap drift.Snapshot
		want drift.Category
	}{
		{"needs attention", with(func(s *drift.Snapshot) { s.Status.Ahead = 1 }), drift.CategoryNeedsAttention},
		{"safe and behind", safeBehind(), drift.CategoryBehind},
		{"up to date", with(func(s *drift.Snapshot) { s.Status.Behind = 0 }), drift.CategoryUpToDate},
		{"behind upstream and behind base", with(func(s *drift.Snapshot) {
			s.Base = repo.Base{Ref: refOriginMain, Behind: 4}
		}), drift.CategoryNeedsAttention},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := drift.CategoryOf(tt.snap); got != tt.want {
				t.Errorf("CategoryOf = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSort(t *testing.T) {
	t.Parallel()

	upToDate := func(name string) drift.Snapshot {
		return with(func(s *drift.Snapshot) {
			s.Loc.Name, s.Loc.Path = name, ""
			s.Status.Behind = 0
		})
	}
	behind := func(name string) drift.Snapshot {
		return with(func(s *drift.Snapshot) { s.Loc.Name, s.Loc.Path = name, "" })
	}
	attention := func(name string) drift.Snapshot {
		return with(func(s *drift.Snapshot) {
			s.Loc.Name, s.Loc.Path = name, ""
			s.Err = errors.New("boom")
		})
	}

	snaps := []drift.Snapshot{
		upToDate("auth"), behind("notifications"), attention("infra"),
		behind("billing"), attention("api-gateway"), upToDate("zeta"),
	}
	// Same name, different path: path breaks the tie.
	a := behind("billing")
	a.Loc.Path = "/a"
	b := behind("billing")
	b.Loc.Path = "/b"
	snaps = append(snaps, b, a)

	drift.Sort(snaps)

	var got []string
	for _, s := range snaps {
		got = append(got, s.Loc.Name+s.Loc.Path)
	}

	want := []string{
		"api-gateway", "infra",
		"billing", "billing/a", "billing/b", "notifications",
		"auth", "zeta",
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Sort order mismatch (-want +got):\n%s", diff)
	}
}

// TestSortByFolder checks that within a category snapshots go by folder,
// the root first and each folder's subfolders right after it, then by name.
func TestSortByFolder(t *testing.T) {
	t.Parallel()

	in := func(folder, name string, behind int) drift.Snapshot {
		return with(func(s *drift.Snapshot) {
			s.Loc.Folder, s.Loc.Name, s.Loc.Path = folder, name, ""
			s.Status.Behind = behind
		})
	}

	snaps := []drift.Snapshot{
		in("services-x", "alpha", 2), in("services/platform", "beta", 2), in("services", "gamma", 2),
		in("", "delta", 2), in("web", "admin", 0), in("", "docs", 0), in("services", "billing", 2),
	}

	drift.Sort(snaps)

	var got []string
	for _, s := range snaps {
		got = append(got, s.Loc.Folder+":"+s.Loc.Name)
	}

	want := []string{
		":delta", "services:billing", "services:gamma", "services/platform:beta", "services-x:alpha",
		":docs", "web:admin",
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Sort order mismatch (-want +got):\n%s", diff)
	}
}

func TestCompareLocations(t *testing.T) {
	t.Parallel()

	loc := func(folder, name, path string) repo.Location {
		return repo.Location{Folder: folder, Name: name, Path: path}
	}

	tests := []struct {
		name string
		a, b repo.Location
		want int
	}{
		{"the root before a folder", loc("", "zeta", ""), loc("api", "alpha", ""), -1},
		{"folders by name", loc("services", "x", ""), loc("web", "x", ""), -1},
		{"a folder before its subfolder", loc("services", "zeta", ""), loc("services/platform", "alpha", ""), -1},
		{"a subfolder before a sibling that extends the name", loc("services/platform", "x", ""), loc("services-x", "x", ""), -1},
		{"deeper subfolders by name", loc("a/b/d", "x", ""), loc("a/b/c", "x", ""), 1},
		{"same folder by name", loc("web", "admin", ""), loc("web", "storefront", ""), -1},
		{"same folder and name by path", loc("web", "admin", "/b"), loc("web", "admin", "/a"), 1},
		{"equal", loc("web", "admin", "/a"), loc("web", "admin", "/a"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := drift.CompareLocations(tt.a, tt.b); got != tt.want {
				t.Errorf("CompareLocations(%+v, %+v) = %d, want %d", tt.a, tt.b, got, tt.want)
			}

			if got := drift.CompareLocations(tt.b, tt.a); got != -tt.want {
				t.Errorf("CompareLocations(%+v, %+v) = %d, want %d", tt.b, tt.a, got, -tt.want)
			}
		})
	}
}
