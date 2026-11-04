package repo

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/epicentre/gitdrift/internal/git"
)

// logFormat is the --format used with -z for every commit listing; parseLog
// reads it back. The author name (%aN) follows the repository's .mailmap.
const logFormat = "--format=%H%x1f%h%x1f%P%x1f%ct%x1f%aN%x1f%s"

const (
	logFieldSep    = "\x1f"
	logFieldCount  = 6
	logParentsIdx  = 2
	logCommitTime  = 3
	mergeParentMin = 2
	sha1Len        = 40
	sha256Len      = 64
)

// Commit is one commit from a log listing.
type Commit struct {
	SHA, Short, Subject, Author string
	When                        time.Time
	// Merge is true when the commit has more than one parent.
	Merge bool
}

// parseLog parses the output of `git log -z` with logFormat.
func parseLog(out []byte) ([]Commit, error) {
	var commits []Commit

	for rec := range bytes.SplitSeq(out, []byte{0}) {
		if len(rec) == 0 {
			continue
		}

		line := string(rec)

		// The subject is last, so it absorbs any separator byte it contains.
		fields := strings.SplitN(line, logFieldSep, logFieldCount)
		if len(fields) != logFieldCount {
			return nil, fmt.Errorf("malformed log record %q", line)
		}

		secs, err := strconv.ParseInt(fields[logCommitTime], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("malformed commit time %q: %w", fields[logCommitTime], err)
		}

		commits = append(commits, Commit{
			SHA:     fields[0],
			Short:   fields[1],
			Merge:   len(strings.Fields(fields[logParentsIdx])) >= mergeParentMin,
			When:    time.Unix(secs, 0).UTC(),
			Author:  fields[4],
			Subject: fields[5],
		})
	}

	return commits, nil
}

// OnlyHereCommits lists the commits reachable from ref that no remote-tracking
// ref contains, newest first. A limit of zero or less lists all of them.
func OnlyHereCommits(ctx context.Context, g *git.Runner, loc Location, ref string, limit int) ([]Commit, error) {
	args := []string{"log", "-z", logFormat}
	if limit > 0 {
		args = append(args, "--max-count="+strconv.Itoa(limit))
	}

	args = append(args, qualifyRef(ref), "--not", "--remotes", "--")

	out, err := g.Run(ctx, loc.Path, args...)
	if err != nil {
		return nil, fmt.Errorf("repo: list commits only on this machine in %s: %w", loc.Name, err)
	}

	commits, err := parseLog(out)
	if err != nil {
		return nil, fmt.Errorf("repo: parse log of %s: %w", loc.Name, err)
	}

	return commits, nil
}

// qualifyRef makes a short branch name unambiguous: a tag or other ref with
// the same name must never be read instead of the branch. Full refs and
// commit ids pass through unchanged.
func qualifyRef(ref string) string {
	if strings.HasPrefix(ref, "refs/") || isHexSHA(ref) {
		return ref
	}

	return "refs/heads/" + ref
}

func isHexSHA(s string) bool {
	if len(s) != sha1Len && len(s) != sha256Len {
		return false
	}

	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}

	return true
}
