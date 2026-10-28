package repo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/epicentre/gitdrift/internal/git"
)

const (
	// MaxCommits is the most commits a detail listing shows.
	MaxCommits = 500
	// MaxDiffBytes is the most diff text the detail pane loads.
	MaxDiffBytes = 1 << 20
)

const (
	untrackedStatus = "?"
	devNull         = "/dev/null"
	// noIndexDiffers is the exit code of `git diff --no-index` when the two
	// sides differ.
	noIndexDiffers = 1
	// numstatFields is the number of tab-separated fields before the path.
	numstatFields = 3
	renameMarker  = "R"
	copyMarker    = "C"
)

// DiffKind selects how a DiffSpec is compared.
type DiffKind int

// Kinds of diff.
const (
	// DiffUnknown is the zero value and is rejected.
	DiffUnknown DiffKind = iota
	// DiffRange compares From with To.
	DiffRange
	// DiffMergeBase compares the merge base of From and To with To.
	DiffMergeBase
	// DiffWorktree compares HEAD with the working tree and adds untracked files.
	DiffWorktree
)

// DiffSpec describes one comparison.
type DiffSpec struct {
	Kind     DiffKind
	From, To string
}

// FileChange is one changed file of a diff.
type FileChange struct {
	Path, OldPath string
	// Status is the git status letter (A, M, D, R, C, T) or "?" for untracked.
	Status         string
	Added, Deleted int
	Binary         bool
}

// diffBase holds the flags every diff carries so that user configuration
// (color, external diff, textconv, prefixes) cannot change the output.
var diffBase = []string{
	"diff", "--no-color", "--no-ext-diff", "--no-textconv",
	"--src-prefix=a/", "--dst-prefix=b/", "-M",
}

// RangeCommits lists up to limit commits reachable from include but not from
// exclude, newest first, plus how many there are in total. Callers pass
// commit ids or HEAD.
func RangeCommits(
	ctx context.Context, g *git.Runner, loc Location, exclude, include string, limit int,
) ([]Commit, int, error) {
	rng := exclude + ".." + include

	args := []string{"log", "-z", logFormat}
	if limit > 0 {
		args = append(args, "--max-count="+strconv.Itoa(limit))
	}

	out, err := g.Run(ctx, loc.Path, append(args, "--end-of-options", rng, "--")...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: list commits %s in %s: %w", rng, loc.Name, err)
	}

	commits, err := parseLog(out)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: parse log %s in %s: %w", rng, loc.Name, err)
	}

	out, err = g.Run(ctx, loc.Path, "rev-list", "--count", "--end-of-options", rng, "--")
	if err != nil {
		return nil, 0, fmt.Errorf("repo: count commits %s in %s: %w", rng, loc.Name, err)
	}

	total, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return nil, 0, fmt.Errorf("repo: parse commit count %q: %w", out, err)
	}

	return commits, total, nil
}

// CommitSpec diffs a commit against its first parent, or against the empty
// tree for a root commit.
func CommitSpec(ctx context.Context, g *git.Runner, loc Location, sha string) (DiffSpec, error) {
	out, err := g.Run(ctx, loc.Path, "rev-list", "--parents", "-n", "1", "--end-of-options", sha)
	if err != nil {
		return DiffSpec{}, fmt.Errorf("repo: read parents of %s in %s: %w", sha, loc.Name, err)
	}

	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return DiffSpec{}, fmt.Errorf("repo: unknown commit %q in %s", sha, loc.Name)
	}

	if len(fields) > 1 {
		return DiffSpec{Kind: DiffRange, From: fields[1], To: sha}, nil
	}

	empty, err := emptyTree(ctx, g, loc)
	if err != nil {
		return DiffSpec{}, err
	}

	return DiffSpec{Kind: DiffRange, From: empty, To: sha}, nil
}

// StashSpec diffs a stash entry against the commit it was made on.
func StashSpec(s Stash) DiffSpec {
	return DiffSpec{Kind: DiffRange, From: s.SHA + "^1", To: s.SHA}
}

func emptyTree(ctx context.Context, g *git.Runner, loc Location) (string, error) {
	out, err := g.Run(ctx, loc.Path, "hash-object", "-t", "tree", devNull)
	if err != nil {
		return "", fmt.Errorf("repo: empty tree id in %s: %w", loc.Name, err)
	}

	return strings.TrimSpace(string(out)), nil
}

// revArgs returns the revision part of a diff argv for spec.
func revArgs(ctx context.Context, g *git.Runner, loc Location, spec DiffSpec) ([]string, error) {
	switch spec.Kind {
	case DiffRange:
		return []string{"--end-of-options", spec.From, spec.To}, nil
	case DiffMergeBase:
		return []string{"--end-of-options", spec.From + "..." + spec.To}, nil
	case DiffWorktree:
		head := "HEAD"

		if _, err := g.Run(ctx, loc.Path, "rev-parse", "--verify", "--quiet", "HEAD^{commit}"); err != nil {
			if git.ExitCode(err) != 1 {
				return nil, fmt.Errorf("repo: resolve HEAD in %s: %w", loc.Name, err)
			}

			empty, treeErr := emptyTree(ctx, g, loc)
			if treeErr != nil {
				return nil, treeErr
			}

			head = empty
		}

		return []string{"--end-of-options", head}, nil
	case DiffUnknown:
		fallthrough
	default:
		return nil, fmt.Errorf("repo: unsupported diff kind %d", spec.Kind)
	}
}

// Files lists the files that spec changes. For DiffWorktree, untracked files
// follow the tracked changes with status "?".
func Files(ctx context.Context, g *git.Runner, loc Location, spec DiffSpec) ([]FileChange, error) {
	revs, err := revArgs(ctx, g, loc, spec)
	if err != nil {
		return nil, err
	}

	run := func(mode string) ([]byte, error) {
		args := append(cloneArgs(diffBase), mode, "-z")
		args = append(args, revs...)

		return g.Run(ctx, loc.Path, append(args, "--")...)
	}

	status, err := run("--name-status")
	if err != nil {
		return nil, fmt.Errorf("repo: list changed files in %s: %w", loc.Name, err)
	}

	numstat, err := run("--numstat")
	if err != nil {
		return nil, fmt.Errorf("repo: count changed lines in %s: %w", loc.Name, err)
	}

	files, err := parseNameStatus(status)
	if err != nil {
		return nil, err
	}

	counts, err := parseNumstat(numstat)
	if err != nil {
		return nil, err
	}

	for i := range files {
		if c, ok := counts[files[i].Path]; ok {
			files[i].Added, files[i].Deleted, files[i].Binary = c.added, c.deleted, c.binary
		}
	}

	if spec.Kind == DiffWorktree {
		untracked, listErr := untrackedFiles(ctx, g, loc, "")
		if listErr != nil {
			return nil, listErr
		}

		for _, p := range untracked {
			files = append(files, FileChange{Path: p, Status: untrackedStatus})
		}
	}

	return files, nil
}

func cloneArgs(s []string) []string { return append([]string(nil), s...) }

func parseNameStatus(out []byte) ([]FileChange, error) {
	var files []FileChange

	parts := strings.Split(string(out), "\x00")

	for i := 0; i < len(parts); i++ {
		code := parts[i]
		if code == "" {
			continue
		}

		letter := code[:1]
		npaths := 1

		if letter == renameMarker || letter == copyMarker {
			npaths = 2
		}

		if i+npaths >= len(parts) {
			return nil, fmt.Errorf("repo: malformed name-status output near %q", code)
		}

		f := FileChange{Status: letter, Path: parts[i+npaths]}
		if npaths == 2 {
			f.OldPath = parts[i+1]
		}

		files = append(files, f)
		i += npaths
	}

	return files, nil
}

type lineCount struct {
	added, deleted int
	binary         bool
}

// parseNumstat reads `--numstat -z`, keyed by the new path.
func parseNumstat(out []byte) (map[string]lineCount, error) {
	counts := map[string]lineCount{}
	parts := strings.Split(string(out), "\x00")

	for i := 0; i < len(parts); i++ {
		rec := parts[i]
		if rec == "" {
			continue
		}

		fields := strings.SplitN(rec, "\t", numstatFields)
		if len(fields) != numstatFields {
			return nil, fmt.Errorf("repo: malformed numstat record %q", rec)
		}

		path := fields[2]
		if path == "" { // rename or copy: the old and new paths follow
			if i+2 >= len(parts) {
				return nil, fmt.Errorf("repo: malformed numstat rename %q", rec)
			}

			path = parts[i+2]
			i += 2
		}

		var c lineCount

		if fields[0] == "-" && fields[1] == "-" {
			c.binary = true
		} else {
			var err error

			if c.added, err = strconv.Atoi(fields[0]); err != nil {
				return nil, fmt.Errorf("repo: malformed numstat count %q: %w", fields[0], err)
			}

			if c.deleted, err = strconv.Atoi(fields[1]); err != nil {
				return nil, fmt.Errorf("repo: malformed numstat count %q: %w", fields[1], err)
			}
		}

		counts[path] = c
	}

	return counts, nil
}

// untrackedFiles lists untracked, non-ignored files, optionally limited to
// one literal path. Nested repositories (reported as directories) are skipped.
func untrackedFiles(ctx context.Context, g *git.Runner, loc Location, path string) ([]string, error) {
	args := []string{"ls-files", "--others", "--exclude-standard", "-z", "--"}
	if path != "" {
		args = append(args, ":(literal)"+path)
	}

	out, err := g.Run(ctx, loc.Path, args...)
	if err != nil {
		return nil, fmt.Errorf("repo: list untracked files in %s: %w", loc.Name, err)
	}

	var files []string

	for p := range bytes.SplitSeq(out, []byte{0}) {
		if len(p) == 0 || p[len(p)-1] == '/' {
			continue
		}

		files = append(files, string(p))
	}

	return files, nil
}

// FileDiff returns the diff text of one file of spec, or of the whole spec
// when path is empty. For a renamed file oldPath is its path before the
// rename, so that git pairs both sides instead of showing the file as added;
// it is empty otherwise. Text beyond maxBytes is cut at a line boundary and
// truncated is true. A maxBytes of zero or less means no limit.
func FileDiff(
	ctx context.Context, g *git.Runner, loc Location, spec DiffSpec, path, oldPath string, maxBytes int,
) (string, bool, error) {
	revs, err := revArgs(ctx, g, loc, spec)
	if err != nil {
		return "", false, err
	}

	var text strings.Builder

	var untracked []string

	if spec.Kind == DiffWorktree {
		if untracked, err = untrackedFiles(ctx, g, loc, path); err != nil {
			return "", false, err
		}
	}

	// An untracked path has no tracked diff; asking for one would match nothing.
	if path == "" || len(untracked) == 0 {
		args := append(cloneArgs(diffBase), revs...)

		args = append(append(args, "--"), pathspecs(path, oldPath)...)

		out, runErr := g.Run(ctx, loc.Path, args...)
		if runErr != nil {
			return "", false, fmt.Errorf("repo: diff in %s: %w", loc.Name, runErr)
		}

		text.Write(out)
	}

	for _, p := range untracked {
		out, runErr := g.Run(ctx, loc.Path, "diff", "--no-color", "--no-ext-diff", "--no-textconv",
			"--no-index", "--src-prefix=a/", "--dst-prefix=b/", "--", devNull, p)
		if runErr != nil {
			// Exit 1 means "the files differ"; the diff is on stdout.
			ce, ok := errors.AsType[*git.CommandError](runErr)
			if !ok || ce.ExitCode != noIndexDiffers {
				return "", false, fmt.Errorf("repo: diff untracked %s in %s: %w", p, loc.Name, runErr)
			}

			out = ce.Stdout
		}

		text.Write(out)
	}

	s, truncated := truncateLines(text.String(), maxBytes)

	return s, truncated, nil
}

// pathspecs limits a diff to path and, for a renamed file, its old path, so
// that git pairs both sides. An empty path selects the whole diff.
func pathspecs(path, oldPath string) []string {
	if path == "" {
		return nil
	}

	specs := []string{":(literal)" + path}
	if oldPath != "" && oldPath != path {
		specs = append(specs, ":(literal)"+oldPath)
	}

	return specs
}

// truncateLines cuts s to at most maxBytes, preferring a line boundary and
// never splitting a UTF-8 sequence.
func truncateLines(s string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s, false
	}

	cut := s[:maxBytes]
	if i := strings.LastIndexByte(cut, '\n'); i >= 0 {
		return cut[:i+1], true
	}

	for len(cut) > 0 && !utf8.RuneStart(s[len(cut)]) {
		cut = cut[:len(cut)-1]
	}

	return cut, true
}
