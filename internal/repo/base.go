package repo

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/epicentre/gitdrift/internal/git"
)

// BaseSource tells where the base setting came from.
type BaseSource string

// Sources reported in Base.Source.
const (
	// SourceAuto means no setting was found and the base was detected.
	SourceAuto BaseSource = "auto"
	// SourceRepo means gitdrift.base is set in the repository or worktree config.
	SourceRepo BaseSource = "repo"
	// SourceGlobal means gitdrift.base is set in a global or system config.
	SourceGlobal BaseSource = "global"
)

const (
	baseConfigKey = "gitdrift.base"
	remoteRefs    = "refs/remotes/"
)

// Base is the branch a repository is compared against.
type Base struct {
	// Configured is the raw gitdrift.base value; empty when unset.
	Configured string
	Source     BaseSource
	// Ref is the short remote-tracking name, for example origin/main; empty
	// when there is no base or it was not found.
	Ref string
	// SHA is the commit Ref resolves to.
	SHA string
	// NotFound is true when a configured base does not exist.
	NotFound bool
	// OnBase is true when the checked-out branch is the base.
	OnBase bool
	// Behind counts the base commits HEAD does not contain; 0 when OnBase.
	Behind int
}

// ResolveBase determines the base branch of the repository described by st.
func ResolveBase(ctx context.Context, g *git.Runner, loc Location, st Status) (Base, error) {
	configured, source, err := readBaseConfig(ctx, g, loc)
	if err != nil {
		return Base{}, err
	}

	b := Base{Configured: configured, Source: source}
	remote := ReferenceRemote(st)

	var ref string

	if configured != "" {
		ref, err = configuredRef(ctx, g, loc, st, configured, remote)
		b.NotFound = err == nil && ref == ""
	} else if remote != "" {
		ref, err = autoRef(ctx, g, loc, remote)
	}

	if err != nil {
		return Base{}, err
	}

	if ref == "" {
		return b, nil
	}

	b.Ref = strings.TrimPrefix(ref, remoteRefs)

	b.SHA, err = verifyCommit(ctx, g, loc, ref)
	if err != nil {
		return Base{}, err
	}

	b.OnBase = onBase(st, ref)
	if !b.OnBase && !st.Unborn {
		b.Behind, err = countBehind(ctx, g, loc, ref)
		if err != nil {
			return Base{}, err
		}
	}

	return b, nil
}

// ReferenceRemote returns the remote base branches are looked up on: the
// upstream's remote (a local upstream, ".", does not count), else origin,
// else the only remote, else "".
func ReferenceRemote(st Status) string {
	if st.UpstreamRemote != "" && st.UpstreamRemote != "." {
		return st.UpstreamRemote
	}

	if slices.Contains(st.Remotes, "origin") {
		return "origin"
	}

	if len(st.Remotes) == 1 {
		return st.Remotes[0]
	}

	return ""
}

// RemoteBranches lists remote-tracking branches as short names such as
// origin/develop, sorted, without <remote>/HEAD entries.
func RemoteBranches(ctx context.Context, g *git.Runner, loc Location) ([]string, error) {
	out, err := g.Run(ctx, loc.Path, "for-each-ref", "--format=%(refname:short)", "--sort=refname", remoteRefs)
	if err != nil {
		return nil, fmt.Errorf("repo: list remote branches of %s: %w", loc.Name, err)
	}

	var names []string

	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		// Depending on the git version, a remote HEAD may print as the bare
		// remote name; both forms are excluded.
		if line == "" || line == "HEAD" || strings.HasSuffix(line, "/HEAD") {
			continue
		}

		if !strings.Contains(line, "/") {
			continue
		}

		names = append(names, line)
	}

	slices.Sort(names)

	return names, nil
}

func readBaseConfig(ctx context.Context, g *git.Runner, loc Location) (string, BaseSource, error) {
	out, err := g.Run(ctx, loc.Path, "config", "--show-scope", "--get", baseConfigKey)
	if err != nil {
		if git.ExitCode(err) == 1 {
			return "", SourceAuto, nil
		}

		return "", "", fmt.Errorf("repo: read %s: %w", baseConfigKey, err)
	}

	scope, value, _ := strings.Cut(strings.TrimSpace(string(out)), "\t")

	value = strings.TrimSpace(value)
	if value == "" {
		return "", SourceAuto, nil
	}

	if scope == "local" || scope == "worktree" {
		return value, SourceRepo, nil
	}

	return value, SourceGlobal, nil
}

// configuredRef returns the full ref of a configured base, or "" when it does
// not exist.
func configuredRef(ctx context.Context, g *git.Runner, loc Location, st Status, value, remote string) (string, error) {
	first, _, _ := strings.Cut(value, "/")

	var ref string

	switch {
	case slices.Contains(st.Remotes, first):
		ref = remoteRefs + value
	case remote != "":
		ref = remoteRefs + remote + "/" + value
	default:
		return "", nil
	}

	return existingRef(ctx, g, loc, ref)
}

// autoRef tries the remote's HEAD, then main, then master.
func autoRef(ctx context.Context, g *git.Runner, loc Location, remote string) (string, error) {
	out, err := g.Run(ctx, loc.Path, "symbolic-ref", "-q", remoteRefs+remote+"/HEAD")

	switch {
	case err == nil:
		if ref, err := existingRef(ctx, g, loc, strings.TrimSpace(string(out))); err != nil || ref != "" {
			return ref, err
		}
	case git.ExitCode(err) != 1:
		return "", fmt.Errorf("repo: read %s/HEAD: %w", remote, err)
	}

	for _, name := range []string{"main", "master"} {
		if ref, err := existingRef(ctx, g, loc, remoteRefs+remote+"/"+name); err != nil || ref != "" {
			return ref, err
		}
	}

	return "", nil
}

// existingRef returns ref when it resolves to a commit, else "".
func existingRef(ctx context.Context, g *git.Runner, loc Location, ref string) (string, error) {
	sha, err := verifyCommit(ctx, g, loc, ref)
	if err != nil || sha == "" {
		return "", err
	}

	return ref, nil
}

// verifyCommit resolves ref to a commit sha; "" when it does not exist.
func verifyCommit(ctx context.Context, g *git.Runner, loc Location, ref string) (string, error) {
	out, err := g.Run(ctx, loc.Path, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		if git.ExitCode(err) == 1 {
			return "", nil
		}

		return "", fmt.Errorf("repo: resolve %s: %w", ref, err)
	}

	return strings.TrimSpace(string(out)), nil
}

func onBase(st Status, ref string) bool {
	if st.UpstreamRef != "" {
		return st.UpstreamRef == ref
	}

	if st.Branch == "" {
		return false
	}

	_, branch, _ := strings.Cut(strings.TrimPrefix(ref, remoteRefs), "/")

	return st.Branch == branch
}

func countBehind(ctx context.Context, g *git.Runner, loc Location, ref string) (int, error) {
	out, err := g.Run(ctx, loc.Path, "rev-list", "--count", "HEAD.."+ref)
	if err != nil {
		return 0, fmt.Errorf("repo: count commits behind %s: %w", ref, err)
	}

	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("repo: parse commit count %q: %w", out, err)
	}

	return n, nil
}
