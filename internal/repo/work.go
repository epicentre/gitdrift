package repo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/epicentre/gitdrift/internal/git"
)

const (
	branchFieldCount = 6
	stashFieldCount  = 4
	remoteRefsPrefix = "refs/remotes/"
	localUpstream    = "."
)

// GoneBranch is a local branch whose upstream was deleted on the remote.
type GoneBranch struct {
	Name, SHA, Remote, RemoteRef string
	// OnlyHere counts the commits on the branch that no remote-tracking ref has.
	OnlyHere   int
	LastCommit time.Time
}

// LocalOnlyBranch is a branch that has commits no remote has.
type LocalOnlyBranch struct {
	Name    string
	Commits int
	// NoUpstream is true when the branch has no upstream configured.
	NoUpstream bool
}

// Stash is one entry of the stash list.
type Stash struct {
	Ref, SHA, Subject string
	When              time.Time
}

// Work is what exists only on this machine in one repository (shared by all
// of its worktrees).
type Work struct {
	Gone      []GoneBranch
	LocalOnly []LocalOnlyBranch
	Stashes   []Stash
	// OnlyHereCommits counts commits on local branches that no remote has.
	OnlyHereCommits int
	// NoRemote is true when the repository has no remote at all.
	NoRemote bool
}

// OnlyHereTotal is the number of commits plus stashes that exist only here.
func (w Work) OnlyHereTotal() int { return w.OnlyHereCommits + len(w.Stashes) }

type localBranch struct {
	name, sha, upstream, remote, remoteRef string
	last                                   time.Time
}

// ReadWork inspects the repository at loc for work that exists only on this
// machine. It is repo-wide: run it once per CommonDir. It only reads.
func ReadWork(ctx context.Context, g *git.Runner, loc Location) (Work, error) {
	var w Work

	remotes, err := readRemotes(ctx, g, loc)
	if err != nil {
		return Work{}, err
	}

	w.NoRemote = len(remotes) == 0

	branches, err := readLocalBranches(ctx, g, loc)
	if err != nil {
		return Work{}, err
	}

	tracking, err := readTrackingRefs(ctx, g, loc)
	if err != nil {
		return Work{}, err
	}

	if w.OnlyHereCommits, err = countOnlyHere(ctx, g, loc, "--branches"); err != nil {
		return Work{}, err
	}

	for _, b := range branches {
		n, err := countOnlyHere(ctx, g, loc, "refs/heads/"+b.name)
		if err != nil {
			return Work{}, err
		}

		if isGone(b, tracking) {
			w.Gone = append(w.Gone, GoneBranch{
				Name: b.name, SHA: b.sha, Remote: b.remote, RemoteRef: b.remoteRef,
				OnlyHere: n, LastCommit: b.last,
			})

			continue
		}

		if n > 0 {
			w.LocalOnly = append(w.LocalOnly, LocalOnlyBranch{Name: b.name, Commits: n, NoUpstream: b.upstream == ""})
		}
	}

	if w.Stashes, err = readStashes(ctx, g, loc); err != nil {
		return Work{}, err
	}

	return w, nil
}

// isGone applies the same rule as ReadStatus: the upstream is a
// remote-tracking ref that no longer exists. A local upstream is never gone.
func isGone(b localBranch, tracking map[string]bool) bool {
	return strings.HasPrefix(b.upstream, remoteRefsPrefix) && !tracking[b.upstream]
}

func readLocalBranches(ctx context.Context, g *git.Runner, loc Location) ([]localBranch, error) {
	const format = "--format=%(refname)%00%(objectname)%00%(upstream)%00%(upstream:remotename)%00%(upstream:remoteref)%00%(committerdate:unix)"

	out, err := g.Run(ctx, loc.Path, "for-each-ref", format, "refs/heads")
	if err != nil {
		return nil, fmt.Errorf("repo: list branches of %s: %w", loc.Name, err)
	}

	var branches []localBranch

	for line := range strings.SplitSeq(string(out), "\n") {
		if line == "" {
			continue
		}

		f := strings.Split(line, "\x00")
		if len(f) != branchFieldCount {
			return nil, fmt.Errorf("repo: unexpected for-each-ref output %q", line)
		}

		secs, err := strconv.ParseInt(f[5], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("repo: parse commit time %q: %w", f[5], err)
		}

		branches = append(branches, localBranch{
			name:      strings.TrimPrefix(f[0], "refs/heads/"),
			sha:       f[1],
			upstream:  f[2],
			remote:    f[3],
			remoteRef: f[4],
			last:      time.Unix(secs, 0).UTC(),
		})
	}

	return branches, nil
}

func readTrackingRefs(ctx context.Context, g *git.Runner, loc Location) (map[string]bool, error) {
	out, err := g.Run(ctx, loc.Path, "for-each-ref", "--format=%(refname)", "refs/remotes")
	if err != nil {
		return nil, fmt.Errorf("repo: list remote-tracking refs of %s: %w", loc.Name, err)
	}

	refs := map[string]bool{}

	for line := range strings.SplitSeq(string(out), "\n") {
		if line != "" {
			refs[line] = true
		}
	}

	return refs, nil
}

// countOnlyHere counts commits reachable from rev (a branch name or
// --branches) that no remote-tracking ref contains.
func countOnlyHere(ctx context.Context, g *git.Runner, loc Location, rev string) (int, error) {
	out, err := g.Run(ctx, loc.Path, "rev-list", "--count", rev, "--not", "--remotes", "--")
	if err != nil {
		return 0, fmt.Errorf("repo: count commits only on this machine in %s: %w", loc.Name, err)
	}

	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("repo: parse commit count %q: %w", out, err)
	}

	return n, nil
}

func readStashes(ctx context.Context, g *git.Runner, loc Location) ([]Stash, error) {
	out, err := g.Run(ctx, loc.Path, "stash", "list", "--format=%gd%x1f%H%x1f%ct%x1f%s")
	if err != nil {
		return nil, fmt.Errorf("repo: list stashes of %s: %w", loc.Name, err)
	}

	var stashes []Stash

	for line := range strings.SplitSeq(string(out), "\n") {
		if line == "" {
			continue
		}

		// The subject is last, so it absorbs any separator byte it contains.
		f := strings.SplitN(line, logFieldSep, stashFieldCount)
		if len(f) != stashFieldCount {
			continue // skip a malformed entry instead of failing the repository
		}

		secs, err := strconv.ParseInt(f[2], 10, 64)
		if err != nil {
			continue
		}

		stashes = append(stashes, Stash{Ref: f[0], SHA: f[1], When: time.Unix(secs, 0).UTC(), Subject: f[3]})
	}

	return stashes, nil
}
