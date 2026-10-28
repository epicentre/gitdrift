package workspace

import (
	"context"
	"sync/atomic"

	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/repo"
)

// SetOnAcquire installs the test hook that runs once a refresh holds both the
// semaphore slot and the group lock.
func SetOnAcquire(w *Workspace, fn func(active int)) { w.onAcquire = fn }

// DiscoverCounting runs Discover with a locate function that counts its
// calls, and returns the result together with the call count.
func DiscoverCounting(ctx context.Context, g *git.Runner, root string, opts DiscoverOptions) ([]Group, int, error) {
	var calls atomic.Int32

	groups, err := discover(ctx, g, root, opts, func(ctx context.Context, g *git.Runner, dir string) (repo.Location, error) {
		calls.Add(1)

		return repo.Locate(ctx, g, dir)
	})

	return groups, int(calls.Load()), err
}
