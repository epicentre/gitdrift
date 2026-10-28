package repo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/epicentre/gitdrift/internal/git"
)

// Operation is a multi-step git operation that is in progress.
type Operation string

// Operations reported in Status.Operation.
const (
	OpNone       Operation = ""
	OpMerge      Operation = "merge"
	OpRebase     Operation = "rebase"
	OpCherryPick Operation = "cherry-pick"
	OpRevert     Operation = "revert"
	OpBisect     Operation = "bisect"
)

// Status is the state of one worktree: its branch, upstream, local changes
// and any operation in progress.
type Status struct {
	// Branch is the checked-out branch; empty when Detached.
	Branch string
	// Detached is true when HEAD is not on a branch.
	Detached bool
	// Unborn is true when the branch has no commits yet.
	Unborn bool
	// HeadSHA is the full HEAD commit; empty when Unborn.
	HeadSHA string

	// Upstream is the short upstream name, for example origin/main.
	Upstream string
	// UpstreamRef is the full upstream ref, for example refs/remotes/origin/main.
	UpstreamRef string
	// UpstreamRemote is the remote name; "." for a local upstream branch.
	UpstreamRemote string
	// UpstreamRemoteRef is the branch on the remote, for example refs/heads/main.
	UpstreamRemoteRef string
	// UpstreamGone is true when the configured upstream no longer exists.
	UpstreamGone bool
	// UpstreamSHA is the commit the upstream ref points to; empty when gone.
	UpstreamSHA string

	// Ahead and Behind count commits relative to the upstream.
	Ahead, Behind int
	// Tracked counts changed tracked entries; Untracked counts untracked paths.
	Tracked, Untracked int

	// Operation is the operation in progress, if any.
	Operation Operation
	// Remotes lists the configured remote names.
	Remotes []string
	// LastUpdate is the commit time of the upstream, or of HEAD without one.
	LastUpdate time.Time
}

// operationMarkers are checked in order inside the worktree's git directory.
var operationMarkers = []struct {
	name string
	op   Operation
}{
	{"MERGE_HEAD", OpMerge},
	{"rebase-merge", OpRebase},
	{"rebase-apply", OpRebase},
	{"CHERRY_PICK_HEAD", OpCherryPick},
	{"REVERT_HEAD", OpRevert},
	{"BISECT_LOG", OpBisect},
}

// ReadStatus inspects the worktree at loc. It only reads.
func ReadStatus(ctx context.Context, g *git.Runner, loc Location) (Status, error) {
	out, err := g.Run(ctx, loc.Path, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=normal")
	if err != nil {
		return Status{}, fmt.Errorf("repo: read status of %s: %w", loc.Name, err)
	}

	parsed, err := parseStatusV2(out)
	if err != nil {
		return Status{}, fmt.Errorf("repo: parse status of %s: %w", loc.Name, err)
	}

	st := Status{
		Branch:    parsed.branch,
		Detached:  parsed.detached,
		Unborn:    parsed.unborn,
		HeadSHA:   parsed.oid,
		Upstream:  parsed.upstream,
		Ahead:     parsed.ahead,
		Behind:    parsed.behind,
		Tracked:   parsed.tracked,
		Untracked: parsed.untracked,
		Operation: readOperation(loc.GitDir),
	}

	if st.Upstream != "" {
		if err := readUpstream(ctx, g, loc, &st); err != nil {
			return Status{}, err
		}
	}

	if st.Remotes, err = readRemotes(ctx, g, loc); err != nil {
		return Status{}, err
	}

	if !st.Unborn {
		if st.LastUpdate, err = readLastUpdate(ctx, g, loc, st); err != nil {
			return Status{}, err
		}
	}

	return st, nil
}

// readUpstream fills the upstream ref fields of st from the branch config and
// decides authoritatively whether the upstream is gone: a remote-tracking ref
// that no longer resolves. A local upstream is never gone.
func readUpstream(ctx context.Context, g *git.Runner, loc Location, st *Status) error {
	var err error

	if st.Unborn {
		err = readUnbornUpstream(ctx, g, loc, st)
	} else {
		err = readBranchUpstream(ctx, g, loc, st)
	}

	if err != nil {
		return err
	}

	if st.UpstreamRef == "" {
		return nil
	}

	sha, err := g.Run(ctx, loc.Path, "rev-parse", "--verify", "--quiet", st.UpstreamRef+"^{commit}")
	if err != nil {
		if git.ExitCode(err) != 1 {
			return fmt.Errorf("repo: resolve %s: %w", st.UpstreamRef, err)
		}

		st.UpstreamGone = strings.HasPrefix(st.UpstreamRef, "refs/remotes/")

		return nil
	}

	st.UpstreamSHA = strings.TrimSpace(string(sha))

	return nil
}

func readBranchUpstream(ctx context.Context, g *git.Runner, loc Location, st *Status) error {
	out, err := g.Run(ctx, loc.Path, "for-each-ref",
		"--format=%(upstream)%00%(upstream:remotename)%00%(upstream:remoteref)", "refs/heads/"+st.Branch)
	if err != nil {
		return fmt.Errorf("repo: read upstream of %s: %w", st.Branch, err)
	}

	fields := strings.Split(strings.TrimSuffix(string(out), "\n"), "\x00")
	if len(fields) != 3 {
		return fmt.Errorf("repo: unexpected for-each-ref output %q", out)
	}

	st.UpstreamRef, st.UpstreamRemote, st.UpstreamRemoteRef = fields[0], fields[1], fields[2]

	return nil
}

// readUnbornUpstream reads the upstream of a branch that has no ref yet, where
// for-each-ref prints nothing, from the branch configuration.
func readUnbornUpstream(ctx context.Context, g *git.Runner, loc Location, st *Status) error {
	remote, err := readBranchConfig(ctx, g, loc, "branch."+st.Branch+".remote")
	if err != nil {
		return err
	}

	merge, err := readBranchConfig(ctx, g, loc, "branch."+st.Branch+".merge")
	if err != nil {
		return err
	}

	st.UpstreamRemote, st.UpstreamRemoteRef = remote, merge

	if remote == "." {
		st.UpstreamRef = merge
	} else {
		st.UpstreamRef = "refs/remotes/" + st.Upstream
	}

	return nil
}

func readBranchConfig(ctx context.Context, g *git.Runner, loc Location, key string) (string, error) {
	out, err := g.Run(ctx, loc.Path, "config", "--get", key)
	if err != nil {
		if git.ExitCode(err) == 1 {
			return "", nil // not set
		}

		return "", fmt.Errorf("repo: read %s: %w", key, err)
	}

	return strings.TrimSpace(string(out)), nil
}

// Remotes lists the names of the remotes configured for loc.
func Remotes(ctx context.Context, g *git.Runner, loc Location) ([]string, error) {
	return readRemotes(ctx, g, loc)
}

func readRemotes(ctx context.Context, g *git.Runner, loc Location) ([]string, error) {
	out, err := g.Run(ctx, loc.Path, "remote")
	if err != nil {
		return nil, fmt.Errorf("repo: list remotes of %s: %w", loc.Name, err)
	}

	var remotes []string

	for line := range strings.SplitSeq(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			remotes = append(remotes, line)
		}
	}

	return remotes, nil
}

func readLastUpdate(ctx context.Context, g *git.Runner, loc Location, st Status) (time.Time, error) {
	rev := "HEAD"
	if st.UpstreamSHA != "" {
		rev = st.UpstreamRef
	}

	out, err := g.Run(ctx, loc.Path, "log", "-1", "--format=%ct", rev, "--")
	if err != nil {
		return time.Time{}, fmt.Errorf("repo: read commit time of %s: %w", loc.Name, err)
	}

	secs, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("repo: parse commit time %q: %w", out, err)
	}

	return time.Unix(secs, 0).UTC(), nil
}

func readOperation(gitDir string) Operation {
	for _, m := range operationMarkers {
		if _, err := os.Stat(filepath.Join(gitDir, m.name)); err == nil {
			return m.op
		}
	}

	return OpNone
}

// statusV2 is the parsed content of `git status --porcelain=v2 -z --branch`.
type statusV2 struct {
	oid       string
	branch    string
	detached  bool
	unborn    bool
	upstream  string
	hasAB     bool
	ahead     int
	behind    int
	tracked   int
	untracked int
}

const (
	oidInitial     = "(initial)"
	headDetached   = "(detached)"
	headerPrefix   = "# "
	renameEntryTag = '2'
)

// parseStatusV2 parses NUL-separated porcelain v2 output. A rename or copy
// entry is followed by one extra NUL-terminated field with the original path.
func parseStatusV2(out []byte) (statusV2, error) {
	var st statusV2

	entries := bytes.Split(out, []byte{0})

	for i := 0; i < len(entries); i++ {
		entry := string(entries[i])
		if entry == "" {
			continue
		}

		switch {
		case strings.HasPrefix(entry, headerPrefix):
			if err := st.parseHeader(entry[len(headerPrefix):]); err != nil {
				return statusV2{}, err
			}
		case entry[0] == '1' || entry[0] == 'u':
			st.tracked++
		case entry[0] == renameEntryTag:
			st.tracked++
			i++ // skip the original path
		case entry[0] == '?':
			st.untracked++
		case entry[0] == '!':
		default:
			return statusV2{}, fmt.Errorf("unexpected entry %q", entry)
		}
	}

	return st, nil
}

func (st *statusV2) parseHeader(h string) error {
	key, value, _ := strings.Cut(h, " ")

	switch key {
	case "branch.oid":
		if value == oidInitial {
			st.unborn = true
		} else {
			st.oid = value
		}
	case "branch.head":
		if value == headDetached {
			st.detached = true
		} else {
			st.branch = value
		}
	case "branch.upstream":
		st.upstream = value
	case "branch.ab":
		ahead, behind, err := parseAB(value)
		if err != nil {
			return err
		}

		st.hasAB, st.ahead, st.behind = true, ahead, behind
	}

	return nil
}

func parseAB(v string) (int, int, error) {
	a, b, ok := strings.Cut(v, " ")
	if !ok || !strings.HasPrefix(a, "+") || !strings.HasPrefix(b, "-") {
		return 0, 0, errors.New("malformed branch.ab " + strconv.Quote(v))
	}

	ahead, err := strconv.Atoi(a[1:])
	if err != nil {
		return 0, 0, fmt.Errorf("malformed branch.ab %q: %w", v, err)
	}

	behind, err := strconv.Atoi(b[1:])
	if err != nil {
		return 0, 0, fmt.Errorf("malformed branch.ab %q: %w", v, err)
	}

	if ahead < 0 || behind < 0 {
		return 0, 0, errors.New("negative branch.ab " + strconv.Quote(v))
	}

	return ahead, behind, nil
}
