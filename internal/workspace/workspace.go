package workspace

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/epicentre/gitdrift/internal/badge"
	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/git"
	"github.com/epicentre/gitdrift/internal/ops"
	"github.com/epicentre/gitdrift/internal/repo"
)

const (
	defaultConcurrency  = 8
	defaultFetchTimeout = 60 * time.Second
)

// ErrClosed means the workspace was closed before the call started; the call
// ran no git.
var ErrClosed = errors.New("workspace: closed")

// Options tune how a Workspace refreshes.
type Options struct {
	// Concurrency bounds the groups refreshed at the same time.
	Concurrency int
	// FetchTimeout bounds the fetch of each remote.
	FetchTimeout time.Duration
}

// DefaultOptions returns the production options.
func DefaultOptions() Options {
	return Options{Concurrency: defaultConcurrency, FetchTimeout: defaultFetchTimeout}
}

// groupState is the mutable state of one group. lock serializes refreshes
// and writes; it is a channel so that waiting honors a context.
type groupState struct {
	lock  chan struct{}
	fetch drift.FetchState // guarded by lock
}

// Workspace refreshes a fixed set of repository groups and runs the UI's
// operations on them.
type Workspace struct {
	g      *git.Runner
	groups []Group
	opts   Options
	sem    chan struct{}
	state  map[string]*groupState
	active atomic.Int32

	// mu guards calls and closed; idle is signalled whenever calls, the
	// number of backend calls in flight, drops to zero.
	mu     sync.Mutex
	idle   *sync.Cond
	calls  int
	closed bool

	// onAcquire is a test hook called once a refresh holds a semaphore slot
	// and its group lock; active counts the refreshes holding a slot.
	onAcquire func(active int)
}

// New creates a Workspace for groups.
func New(g *git.Runner, groups []Group, opts Options) *Workspace {
	if opts.Concurrency < 1 {
		opts.Concurrency = 1
	}

	if opts.FetchTimeout <= 0 {
		opts.FetchTimeout = defaultFetchTimeout
	}

	groups = slices.DeleteFunc(slices.Clone(groups), func(grp Group) bool { return len(grp.Members) == 0 })

	state := make(map[string]*groupState, len(groups))
	for _, grp := range groups {
		state[grp.CommonDir] = &groupState{lock: make(chan struct{}, 1)}
	}

	w := &Workspace{
		g:      g,
		groups: groups,
		opts:   opts,
		sem:    make(chan struct{}, opts.Concurrency),
		state:  state,
	}
	w.idle = sync.NewCond(&w.mu)

	return w
}

// Close stops the workspace. Calls that start afterwards run no git: they
// return ErrClosed, and Refresh returns no snapshots. Close returns once
// every call in flight has returned. Cancel their context first so that
// refreshes stop early; a pull's merge, once started, always runs to its
// end. Close may be called more than once and concurrently.
func (w *Workspace) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.closed = true

	for w.calls > 0 {
		w.idle.Wait()
	}
}

// enter registers a backend call. It reports false once Close has started;
// the call must then return without running git.
func (w *Workspace) enter() bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return false
	}

	w.calls++

	return true
}

// leave ends a call registered by enter.
func (w *Workspace) leave() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.calls--
	if w.calls == 0 {
		w.idle.Broadcast()
	}
}

// Groups returns the discovered groups.
func (w *Workspace) Groups() []Group { return w.groups }

func (w *Workspace) group(commonDir string) (Group, bool) {
	for _, grp := range w.groups {
		if grp.CommonDir == commonDir {
			return grp, true
		}
	}

	return Group{}, false
}

// acquire takes the group's lock, giving up when ctx ends.
func (s *groupState) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("workspace: waiting for repository: %w", err)
	}

	select {
	case s.lock <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("workspace: waiting for repository: %w", ctx.Err())
	}
}

func (s *groupState) release() { <-s.lock }

// Refresh re-reads every member of the group and returns one snapshot per
// member, in member order. With fetch it first fetches every remote once; it
// otherwise keeps the outcome of the last fetch. It returns nil for an
// unknown group and once the workspace is closed.
func (w *Workspace) Refresh(ctx context.Context, commonDir string, fetch bool) []drift.Snapshot {
	if !w.enter() {
		return nil
	}
	defer w.leave()

	grp, ok := w.group(commonDir)
	if !ok {
		return nil
	}

	st := w.state[commonDir]

	if err := w.acquireSlot(ctx); err != nil {
		return failed(grp, err)
	}

	active := int(w.active.Add(1))

	defer func() {
		w.active.Add(-1)
		<-w.sem
	}()

	if err := st.acquire(ctx); err != nil {
		return failed(grp, err)
	}
	defer st.release()

	if w.onAcquire != nil {
		w.onAcquire(active)
	}

	if fetch {
		st.fetch = w.fetchAll(ctx, grp)
	}

	return w.read(ctx, grp, st.fetch)
}

func (w *Workspace) acquireSlot(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("workspace: waiting for a refresh slot: %w", err)
	}

	select {
	case w.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("workspace: waiting for a refresh slot: %w", ctx.Err())
	}
}

func failed(grp Group, err error) []drift.Snapshot {
	snaps := make([]drift.Snapshot, len(grp.Members))
	for i, loc := range grp.Members {
		snaps[i] = drift.Snapshot{Loc: loc, Err: err}
	}

	return snaps
}

// fetchAll fetches every remote of the group, each within FetchTimeout.
// Remotes are shared by the members, so the first member that can list them
// speaks for the group. When none can, nothing was attempted and the result
// is not Done. A failed remote keeps one line for the status line: git's own
// message, or gdrift's reason for refusing the fetch.
func (w *Workspace) fetchAll(ctx context.Context, grp Group) drift.FetchState {
	for _, loc := range grp.Members {
		remotes, err := repo.Remotes(ctx, w.g, loc)
		if err != nil {
			continue
		}

		state := drift.FetchState{Done: true}

		for _, remote := range remotes {
			if err := w.fetchOne(ctx, loc, remote); err != nil {
				if state.Errors == nil {
					state.Errors = make(map[string]string)
				}

				state.Errors[remote] = git.Message(err)
			}
		}

		return state
	}

	return drift.FetchState{}
}

func (w *Workspace) fetchOne(ctx context.Context, loc repo.Location, remote string) error {
	ctx, cancel := context.WithTimeout(ctx, w.opts.FetchTimeout)
	defer cancel()

	return ops.Fetch(ctx, w.g, loc, remote)
}

// read builds one snapshot per member. The group's work and badge rules are
// read once.
func (w *Workspace) read(ctx context.Context, grp Group, fetch drift.FetchState) []drift.Snapshot {
	work, workErr := w.readWork(ctx, grp)

	rules, rulesErr := w.readBadgeRules(ctx, grp)
	if workErr == nil {
		workErr = rulesErr
	}

	snaps := make([]drift.Snapshot, len(grp.Members))

	for i, loc := range grp.Members {
		snap := drift.Snapshot{Loc: loc, Work: work, BadgeRules: slices.Clone(rules), Fetch: cloneFetch(fetch)}
		snap.Err = workErr

		if err := w.readMember(ctx, &snap); err != nil {
			snap.Err = err
		}

		snaps[i] = snap
	}

	return snaps
}

func (w *Workspace) readMember(ctx context.Context, snap *drift.Snapshot) error {
	var err error

	if snap.Status, err = repo.ReadStatus(ctx, w.g, snap.Loc); err != nil {
		return err
	}

	if snap.Base, err = repo.ResolveBase(ctx, w.g, snap.Loc, snap.Status); err != nil {
		return err
	}

	if snap.Status.Behind == 0 {
		return nil
	}

	files, err := repo.IncomingFiles(ctx, w.g, snap.Loc, snap.Status)
	if err != nil {
		return err
	}

	snap.Badges = badge.Match(snap.BadgeRules, files)

	return nil
}

func cloneFetch(f drift.FetchState) drift.FetchState {
	f.Errors = maps.Clone(f.Errors)

	return f
}

// readBadgeRules reads the group's badge rules through the first member that
// can be read; the gitdrift.badge values live in the shared config.
func (w *Workspace) readBadgeRules(ctx context.Context, grp Group) ([]badge.Rule, error) {
	var err error

	for _, loc := range grp.Members {
		var rules []badge.Rule
		if rules, err = repo.BadgeRules(ctx, w.g, loc); err == nil {
			return rules, nil
		}
	}

	return nil, err
}

// readWork reads the group's work through the first member that can be read.
func (w *Workspace) readWork(ctx context.Context, grp Group) (repo.Work, error) {
	var (
		work repo.Work
		err  error
	)

	for _, loc := range grp.Members {
		if work, err = repo.ReadWork(ctx, w.g, loc); err == nil {
			return work, nil
		}
	}

	return work, err
}
