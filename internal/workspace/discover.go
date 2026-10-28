// Package workspace discovers the repositories in a folder, refreshes them in
// parallel and exposes the operations the UI needs.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/repo"
)

// ErrNoRepos means the folder holds no git repository.
var ErrNoRepos = errors.New("workspace: no git repositories found")

// Group is a set of worktrees that share one common git directory: a
// repository and its linked worktrees.
type Group struct {
	CommonDir string
	// Members are sorted by path.
	Members []repo.Location
}

// DefaultDepth is how many directory levels below the root Discover searches
// when DiscoverOptions.Depth is not set.
const DefaultDepth = 5

// DiscoverOptions tunes Discover.
type DiscoverOptions struct {
	// Depth is how many directory levels below the root to search; the root
	// is level 0 and its direct children are level 1. Values below 1 mean
	// DefaultDepth.
	Depth int
}

// locateFunc resolves a directory to a worktree root; repo.Locate in
// production, a counting wrapper in tests.
type locateFunc func(ctx context.Context, g *git.Runner, dir string) (repo.Location, error)

// walkDir is one directory reached by the walk.
type walkDir struct {
	path     string // as walked, not symlink-resolved
	resolved string // symlink-resolved path, the visited-set key
	rel      string // slash-separated, relative to the root
}

// Discover finds the repositories in root and in the directories up to
// opts.Depth levels below it. The walk is breadth-first with each level in
// name order. A directory below the root that holds a .git entry is located
// with git and, when it is a work tree, never entered; one that is not, such
// as the parent of a bare clone's worktrees, is walked like any other.
// Directories without a .git entry cost no git process.
// Below the root, directories that are not repositories and are hidden or
// named node_modules or vendor are not entered; a repository with such a
// name is still listed.
// Symlinks are followed, each resolved directory is visited once, and a
// repository reached through several routes is reported once, with the
// folder of the first route. Groups are sorted by their first member's path.
func Discover(ctx context.Context, g *git.Runner, root string, opts DiscoverOptions) ([]Group, error) {
	return discover(ctx, g, root, opts, repo.Locate)
}

// collector gathers located repositories into groups, once per repo.ID.
type collector struct {
	seen     map[repo.ID]bool
	byCommon map[string]*Group
}

func (c *collector) add(loc repo.Location, rel string) {
	if c.seen[loc.ID] {
		return
	}

	c.seen[loc.ID] = true

	if dir := path.Dir(rel); dir != "." {
		loc.Folder = dir
	}

	grp := c.byCommon[loc.CommonDir]
	if grp == nil {
		grp = &Group{CommonDir: loc.CommonDir}
		c.byCommon[loc.CommonDir] = grp
	}

	grp.Members = append(grp.Members, loc)
}

func (c *collector) groups() []Group {
	groups := make([]Group, 0, len(c.byCommon))

	for _, grp := range c.byCommon {
		slices.SortFunc(grp.Members, func(a, b repo.Location) int { return strings.Compare(a.Path, b.Path) })
		groups = append(groups, *grp)
	}

	slices.SortFunc(groups, func(a, b Group) int {
		return strings.Compare(a.Members[0].Path, b.Members[0].Path)
	})

	return groups
}

func discover(ctx context.Context, g *git.Runner, root string, opts DiscoverOptions, locate locateFunc) ([]Group, error) {
	depth := opts.Depth
	if depth < 1 {
		depth = DefaultDepth
	}

	col := &collector{seen: make(map[repo.ID]bool), byCommon: make(map[string]*Group)}

	loc, err := locate(ctx, g, root)

	switch {
	case err == nil:
		col.add(loc, ".")
	case errors.Is(err, repo.ErrNotRepo), errors.Is(err, repo.ErrNotToplevel):
	default:
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("workspace: discover: %w", ctxErr)
		}

		return nil, fmt.Errorf("workspace: discover %s: %w", root, err)
	}

	if err := walk(ctx, g, root, depth, locate, col); err != nil {
		return nil, err
	}

	if len(col.byCommon) == 0 {
		return nil, ErrNoRepos
	}

	return col.groups(), nil
}

// walk visits the levels below root breadth-first and feeds every repository
// it locates to col. Each level is processed in name order: parents keep the
// order of the previous level and os.ReadDir sorts their children by name.
func walk(ctx context.Context, g *git.Runner, root string, depth int, locate locateFunc, col *collector) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("workspace: resolve %s: %w", root, err)
	}

	visited := map[string]bool{resolvedRoot: true}

	// parents are the directories whose children form the next level.
	parents := []walkDir{{path: root, resolved: resolvedRoot, rel: "."}}

	for level := 1; level <= depth && len(parents) > 0; level++ {
		var next []walkDir

		for _, parent := range parents {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("workspace: discover: %w", err)
			}

			children, err := childDirs(parent)
			if err != nil {
				if parent.rel == "." {
					return err // the root's own error is fatal
				}

				continue // an unreadable directory is skipped
			}

			for _, dir := range children {
				if visited[dir.resolved] {
					continue
				}

				visited[dir.resolved] = true

				enter, err := visit(ctx, g, dir, locate, col)
				if err != nil {
					return err
				}

				if enter {
					next = append(next, dir)
				}
			}
		}

		parents = next
	}

	return nil
}

// visit checks one directory: a repository is located and listed, anything
// else is reported as enterable unless its name marks it as skipped. A .git
// entry that git finds no work tree for, such as the .git file of a bare
// clone whose worktrees sit beside it (proj/.git pointing at proj/.bare, with
// proj/main and proj/feature), makes a plain directory.
func visit(ctx context.Context, g *git.Runner, dir walkDir, locate locateFunc, col *collector) (bool, error) {
	enterable := !skippedName(filepath.Base(dir.path))
	if !hasGitEntry(dir.path) {
		return enterable, nil
	}

	loc, err := locate(ctx, g, dir.path)

	switch {
	case err == nil:
		col.add(loc, dir.rel)

		return false, nil
	case ctx.Err() != nil:
		return false, fmt.Errorf("workspace: discover: %w", ctx.Err())
	case errors.Is(err, repo.ErrNotRepo):
		return enterable, nil
	default:
		return false, nil // one unusable repository must not hide the others
	}
}

// hasGitEntry reports whether dir holds a .git directory or file.
func hasGitEntry(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))

	return err == nil
}

// skippedName reports whether a directory that is not a repository is left
// unexplored: hidden directories, node_modules and vendor.
func skippedName(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor"
}

// childDirs lists the child directories of dir in name order, following
// symlinks. A symlink that does not lead to a directory is dropped.
func childDirs(dir walkDir) ([]walkDir, error) {
	entries, err := os.ReadDir(dir.path)
	if err != nil {
		return nil, fmt.Errorf("workspace: read %s: %w", dir.path, err)
	}

	var out []walkDir

	for _, e := range entries {
		name := e.Name()
		p := filepath.Join(dir.path, name)
		resolved := filepath.Join(dir.resolved, name)

		switch {
		case e.IsDir():
		case e.Type()&fs.ModeSymlink != 0:
			if info, statErr := os.Stat(p); statErr != nil || !info.IsDir() {
				continue
			}

			if resolved, err = filepath.EvalSymlinks(p); err != nil {
				continue
			}
		default:
			continue
		}

		out = append(out, walkDir{path: p, resolved: resolved, rel: path.Join(dir.rel, name)})
	}

	return out, nil
}
