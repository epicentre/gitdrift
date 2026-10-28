package ops

import (
	"context"
	"fmt"
	"time"

	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	baseKey = "gitdrift.base"
	// configUnsetMissing is git config's exit code for a key that is not set.
	configUnsetMissing = 5
)

// configTimeout bounds a base write. The write runs detached from the
// caller's cancellation, so a quit never kills git config while it holds
// config.lock.
const configTimeout = 10 * time.Second

// SetBase stores the base branch for loc in the repository's local config.
// Once git has started, cancelling ctx no longer stops it.
func SetBase(ctx context.Context, g *git.Runner, loc repo.Location, value string) error {
	// Never start a write after the caller has quit.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("ops: set %s in %s: %w", baseKey, loc.Name, err)
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), configTimeout)
	defer cancel()

	if _, err := g.Run(ctx, loc.Path, "config", "--local", baseKey, value); err != nil {
		return fmt.Errorf("ops: set %s in %s: %w", baseKey, loc.Name, err)
	}

	return nil
}

// ResetBase removes the stored base branch. A base that was never set is not
// an error. Once git has started, cancelling ctx no longer stops it.
func ResetBase(ctx context.Context, g *git.Runner, loc repo.Location) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("ops: unset %s in %s: %w", baseKey, loc.Name, err)
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), configTimeout)
	defer cancel()

	_, err := g.Run(ctx, loc.Path, "config", "--local", "--unset", baseKey)
	if err == nil {
		return nil
	}

	if git.ExitCode(err) != configUnsetMissing {
		return fmt.Errorf("ops: unset %s in %s: %w", baseKey, loc.Name, err)
	}

	// Exit 5 also means "multiple values": git then removes nothing.
	out, getErr := g.Run(ctx, loc.Path, "config", "--local", "--get-all", baseKey)
	if getErr != nil && git.ExitCode(getErr) != 1 {
		return fmt.Errorf("ops: read %s in %s: %w", baseKey, loc.Name, getErr)
	}

	if len(out) > 0 {
		return fmt.Errorf("ops: reset base in %s: %s has multiple values", loc.Name, baseKey)
	}

	return nil
}
