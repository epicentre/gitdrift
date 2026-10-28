package repo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/epicentre/gitdrift/internal/git"
)

// gitUsageExit is the exit code git uses when a directory is not a usable
// repository (including a bare repository for --show-toplevel).
const gitUsageExit = 128

var (
	// ErrNotRepo means the directory is not inside a git worktree.
	ErrNotRepo = errors.New("repo: not a git repository")
	// ErrNotToplevel means the directory is inside a worktree but is not its root.
	ErrNotToplevel = errors.New("repo: not a worktree root")
)

// ID identifies a repository in a workspace: its resolved worktree path.
type ID string

// Location describes where a repository lives on disk. All paths are
// absolute and symlink-resolved.
type Location struct {
	ID ID
	// Path is the worktree root.
	Path string
	// Name is the last element of Path.
	Name string
	// GitDir is the per-worktree git directory.
	GitDir string
	// CommonDir is the git directory shared by all linked worktrees.
	CommonDir string
}

// Locate resolves dir to a worktree root. It returns ErrNotRepo when dir is
// not in a worktree and ErrNotToplevel when it is a subdirectory of one.
func Locate(ctx context.Context, g *git.Runner, dir string) (Location, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Location{}, fmt.Errorf("repo: resolve %q: %w", dir, err)
	}

	if info, statErr := os.Stat(abs); statErr != nil || !info.IsDir() {
		return Location{}, fmt.Errorf("%w: %s is not a directory", ErrNotRepo, abs)
	}

	out, err := g.Run(ctx, abs, "rev-parse", "--show-toplevel", "--absolute-git-dir", "--git-common-dir")
	if err != nil {
		if git.ExitCode(err) == gitUsageExit {
			return Location{}, fmt.Errorf("%w: %w", ErrNotRepo, err)
		}

		return Location{}, fmt.Errorf("repo: locate %q: %w", dir, err)
	}

	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != 3 {
		return Location{}, fmt.Errorf("repo: unexpected rev-parse output for %q: %q", dir, out)
	}

	top, gitDir, common := lines[0], lines[1], lines[2]
	if !filepath.IsAbs(common) {
		common = filepath.Join(abs, common)
	}

	var loc Location

	for _, p := range []struct {
		dst *string
		src string
	}{{&loc.Path, top}, {&loc.GitDir, gitDir}, {&loc.CommonDir, common}} {
		if *p.dst, err = filepath.EvalSymlinks(p.src); err != nil {
			return Location{}, fmt.Errorf("repo: resolve %q: %w", p.src, err)
		}
	}

	here, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Location{}, fmt.Errorf("repo: resolve %q: %w", abs, err)
	}

	if here != loc.Path {
		return Location{}, fmt.Errorf("%w: %s is inside %s", ErrNotToplevel, here, loc.Path)
	}

	loc.ID = ID(loc.Path)
	loc.Name = filepath.Base(loc.Path)

	return loc, nil
}
