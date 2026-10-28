package ops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/repo"
)

// fetchTimeout bounds the fetch of one remote.
const fetchTimeout = 60 * time.Second

// ErrNoRemoteURL is wrapped by the error for a remote without a url in git
// config. Git then reads a legacy remotes file whose refspecs Fetch cannot
// vet, so such a remote is never fetched.
var ErrNoRemoteURL = errors.New("remote has no url in git config")

// UnsafeRefspecError reports a fetch refspec that writes outside
// refs/remotes/. Pruning with such a refspec would delete local branches or
// tags that the remote lacks.
type UnsafeRefspecError struct {
	Remote  string
	Refspec string
}

func (e *UnsafeRefspecError) Error() string {
	return fmt.Sprintf("fetch refused: remote %q refspec %q writes outside refs/remotes/", e.Remote, e.Refspec)
}

// unsafeRefspec reports whether a fetch refspec has a destination outside
// refs/remotes/. Negative refspecs and refspecs without a destination are safe.
func unsafeRefspec(spec string) bool {
	if strings.HasPrefix(spec, "^") {
		return false
	}

	spec = strings.TrimPrefix(spec, "+")

	_, dst, found := strings.Cut(spec, ":")
	if !found || dst == "" {
		return false
	}

	return !strings.HasPrefix(dst, "refs/remotes/")
}

// checkRefspecs refuses remotes whose configured fetch refspecs would let
// prune delete local refs.
func checkRefspecs(ctx context.Context, g *git.Runner, loc repo.Location, remote string) error {
	if _, err := g.Run(ctx, loc.Path, "config", "--get", "remote."+remote+".url"); err != nil {
		if git.ExitCode(err) == 1 {
			return fmt.Errorf("fetch refused: remote %q: %w", remote, ErrNoRemoteURL)
		}

		return fmt.Errorf("ops: read url of %s: %w", remote, err)
	}

	out, err := g.Run(ctx, loc.Path, "config", "--get-all", "remote."+remote+".fetch")
	if err != nil {
		if git.ExitCode(err) == 1 {
			return nil // no refspecs configured
		}

		return fmt.Errorf("ops: read refspecs of %s: %w", remote, err)
	}

	for spec := range strings.SplitSeq(string(out), "\n") {
		if spec = strings.TrimSpace(spec); spec != "" && unsafeRefspec(spec) {
			return &UnsafeRefspecError{Remote: remote, Refspec: spec}
		}
	}

	return nil
}

// Fetch updates the remote-tracking refs of remote and prunes refs the remote
// no longer has. It never touches the worktree or local branches.
func Fetch(ctx context.Context, g *git.Runner, loc repo.Location, remote string) error {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	if err := checkRefspecs(ctx, g, loc, remote); err != nil {
		return err
	}

	if _, err := g.Run(ctx, loc.Path, "fetch", "--prune", "--no-prune-tags", "--quiet", "--end-of-options", remote); err != nil {
		return fmt.Errorf("ops: fetch %s: %w", remote, err)
	}

	return nil
}
