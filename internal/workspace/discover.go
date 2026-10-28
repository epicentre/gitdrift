// Package workspace discovers the repositories in a folder, refreshes them in
// parallel and exposes the operations the UI needs.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
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

// Discover finds the repositories in root itself and in each of its direct
// child directories. Symlinks are followed and duplicates, reached through
// different paths, are reported once. Groups are sorted by their first
// member's path.
func Discover(ctx context.Context, g *git.Runner, root string) ([]Group, error) {
	candidates, err := candidateDirs(root)
	if err != nil {
		return nil, err
	}

	seen := make(map[repo.ID]bool)
	byCommon := make(map[string]*Group)

	for _, dir := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("workspace: discover: %w", err)
		}

		loc, err := repo.Locate(ctx, g, dir)
		if errors.Is(err, repo.ErrNotRepo) || errors.Is(err, repo.ErrNotToplevel) {
			continue
		}

		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, fmt.Errorf("workspace: discover: %w", ctxErr)
			}

			if dir == root {
				return nil, fmt.Errorf("workspace: discover %s: %w", dir, err)
			}

			continue // one unreadable child must not hide the others
		}

		if seen[loc.ID] {
			continue
		}

		seen[loc.ID] = true

		grp := byCommon[loc.CommonDir]
		if grp == nil {
			grp = &Group{CommonDir: loc.CommonDir}
			byCommon[loc.CommonDir] = grp
		}

		grp.Members = append(grp.Members, loc)
	}

	if len(byCommon) == 0 {
		return nil, ErrNoRepos
	}

	groups := make([]Group, 0, len(byCommon))

	for _, grp := range byCommon {
		slices.SortFunc(grp.Members, func(a, b repo.Location) int { return strings.Compare(a.Path, b.Path) })
		groups = append(groups, *grp)
	}

	slices.SortFunc(groups, func(a, b Group) int {
		return strings.Compare(a.Members[0].Path, b.Members[0].Path)
	})

	return groups, nil
}

// candidateDirs lists root and its direct child directories, following
// symlinks and skipping .git.
func candidateDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("workspace: read %s: %w", root, err)
	}

	dirs := []string{root}

	for _, e := range entries {
		if e.Name() == ".git" {
			continue
		}

		path := filepath.Join(root, e.Name())
		if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
			dirs = append(dirs, path)
		}
	}

	return dirs, nil
}
