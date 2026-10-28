// Package drift classifies repository snapshots: whether a repository is safe
// to pull, why it needs attention, and which category it belongs to. It is
// pure logic with no git calls and no I/O.
package drift

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/epicentre/gitdrift/internal/badge"
	"github.com/epicentre/gitdrift/internal/repo"
)

// localRemote is the remote name git records for a branch tracking a local
// branch.
const localRemote = "."

// Reason texts shared by PullBlockers and AttentionReasons.
const (
	reasonDetached     = "detached HEAD"
	reasonNoUpstream   = "no upstream"
	reasonUpstreamGone = "upstream gone"
	reasonDiverged     = "diverged"
	reasonLocalChanges = "local changes"
	reasonInProgress   = " in progress"
	reasonFetchFailed  = "fetch failed: "
	reasonNotFetched   = "remote not fetched yet: "
)

// FetchState records the outcome of fetching a repository's remotes.
type FetchState struct {
	// Done is true once the fetch finished.
	Done bool
	// Errors maps a remote name to one line saying why its fetch failed.
	Errors map[string]string
}

// OK reports whether remote's refs are current. The local remote "." is
// always current; any other remote needs a finished fetch without an error.
func (f FetchState) OK(remote string) bool {
	if remote == localRemote {
		return true
	}

	if _, failed := f.Errors[remote]; failed {
		return false
	}

	return f.Done
}

// Snapshot is everything known about one repository at one moment.
type Snapshot struct {
	Loc    repo.Location
	Status repo.Status
	Base   repo.Base
	Work   repo.Work
	Badges []string
	// BadgeRules are the repository's badge rules: the built-in ones and
	// its gitdrift.badge values.
	BadgeRules []badge.Rule
	Fetch      FetchState
	// Err is set when inspecting the repository failed.
	Err error
}

// Category groups snapshots by what the user should do about them.
type Category int

// Categories, in display order.
const (
	CategoryUnknown Category = iota
	CategoryNeedsAttention
	CategoryBehind
	CategoryUpToDate
)

// PullBlockers lists the reasons a fast-forward pull is not safe, in order of
// precedence. An empty result means the repository can be pulled.
func PullBlockers(s Snapshot) []string {
	var out []string

	st := s.Status

	if s.Err != nil {
		out = append(out, "inspection failed")
	}

	out = append(out, upstreamBlockers(s)...)

	if st.Operation != repo.OpNone {
		out = append(out, string(st.Operation)+reasonInProgress)
	}

	switch {
	case st.Ahead > 0 && st.Behind > 0:
		out = append(out, reasonDiverged)
	case st.Ahead > 0:
		out = append(out, "ahead of upstream")
	}

	if st.Behind == 0 {
		out = append(out, "nothing to pull")
	}

	if st.Tracked > 0 {
		out = append(out, reasonLocalChanges)
	}

	return out
}

// upstreamBlockers covers HEAD and upstream state: detached, missing, gone,
// or its remote failed to fetch or is not fetched yet.
func upstreamBlockers(s Snapshot) []string {
	var out []string

	st := s.Status

	if st.Detached {
		out = append(out, reasonDetached)
	} else if st.Upstream == "" {
		out = append(out, reasonNoUpstream)
	}

	if st.UpstreamGone {
		out = append(out, reasonUpstreamGone)
	}

	if remote := st.UpstreamRemote; st.Upstream != "" && remote != "" && !s.Fetch.OK(remote) {
		if _, failed := s.Fetch.Errors[remote]; failed {
			out = append(out, reasonFetchFailed+remote)
		} else {
			out = append(out, reasonNotFetched+remote)
		}
	}

	return out
}

// AttentionReasons lists why the user should look at the repository, in
// display order. An empty result means nothing needs attention.
func AttentionReasons(s Snapshot) []string {
	var out []string

	st := s.Status

	if s.Err != nil {
		out = append(out, "error: "+s.Err.Error())
	}

	if st.Detached {
		out = append(out, reasonDetached)
	}

	if st.Operation != repo.OpNone {
		out = append(out, string(st.Operation)+reasonInProgress)
	}

	for _, remote := range slices.Sorted(maps.Keys(s.Fetch.Errors)) {
		out = append(out, reasonFetchFailed+remote)
	}

	out = append(out, branchReasons(st)...)

	return append(out, baseReasons(s.Base)...)
}

// branchReasons covers upstream divergence and local changes that matter.
func branchReasons(st repo.Status) []string {
	var out []string

	if !st.Detached && st.Upstream == "" {
		out = append(out, reasonNoUpstream)
	}

	if st.UpstreamGone {
		out = append(out, reasonUpstreamGone)
	}

	if st.Ahead > 0 && st.Behind > 0 {
		out = append(out, reasonDiverged)
	}

	if st.Tracked > 0 && st.Behind > 0 {
		out = append(out, reasonLocalChanges)
	}

	return out
}

func baseReasons(b repo.Base) []string {
	switch {
	case b.NotFound:
		return []string{"base not found: " + b.Configured}
	case !b.OnBase && b.Behind > 0:
		return []string{fmt.Sprintf("%d behind %s", b.Behind, b.Ref)}
	default:
		return nil
	}
}

// CategoryOf places the snapshot in exactly one category.
func CategoryOf(s Snapshot) Category {
	switch {
	case len(AttentionReasons(s)) > 0:
		return CategoryNeedsAttention
	case s.Status.Behind > 0:
		return CategoryBehind
	default:
		return CategoryUpToDate
	}
}

// Sort orders snapshots by category, then by folder, name and path (see
// CompareLocations). It is stable.
func Sort(snaps []Snapshot) {
	slices.SortStableFunc(snaps, func(a, b Snapshot) int {
		return cmp.Or(cmp.Compare(CategoryOf(a), CategoryOf(b)), CompareLocations(a.Loc, b.Loc))
	})
}

// CompareLocations orders repositories by folder, then name, then path.
// Folders compare one slash-separated name at a time, so that the root ("")
// comes first and a folder's subfolders follow it before a sibling whose
// name extends it: services, services/platform, services-x.
func CompareLocations(a, b repo.Location) int {
	return cmp.Or(
		compareFolders(a.Folder, b.Folder),
		cmp.Compare(a.Name, b.Name),
		cmp.Compare(a.Path, b.Path),
	)
}

// compareFolders orders slash-separated folders one name at a time.
func compareFolders(a, b string) int {
	for a != "" && b != "" {
		nameA, restA, _ := strings.Cut(a, "/")
		nameB, restB, _ := strings.Cut(b, "/")

		if c := cmp.Compare(nameA, nameB); c != 0 {
			return c
		}

		a, b = restA, restB
	}

	return cmp.Compare(a, b)
}
