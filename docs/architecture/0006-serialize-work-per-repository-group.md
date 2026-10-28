# 6. Serialize work per repository group

- Status: Accepted
- Date: 2026-10-28

## Context

Linked worktrees share refs, `packed-refs`, config and remotes. A
`fetch --prune` racing a `branch -D` in the same common git dir fails on ref
locks; a refresh racing a merge reads a half-moved state. At the same time a
folder of 30 or more repositories must refresh in parallel, and pressing
`r`/`R` repeatedly must not pile up processes.

## Decision

- A group is every worktree with the same common git dir. Fetches run once
  per remote per group, and group-wide reads (work only on this machine,
  badge rules) use the first member that can be read.
- Refreshes take a slot of a semaphore (8 by default), then the group lock.
- Every write (pull, gone deletion, base set and reset) takes the same group
  lock and no semaphore slot.
- The group lock is a one-slot channel; waiting honors the context, and the
  context is checked before waiting so a cancelled call never runs git.
- The UI keeps at most one refresh per group in flight. A refresh requested
  meanwhile is queued once, keeping the stronger `fetch` flag.

## Consequences

- gitdrift's own operations never fail on each other's ref locks.
- The number of concurrent git processes stays bounded.
- A write can wait behind a running refresh of its group, bounded by the
  fetch timeouts.
- `P` (pull all safe) starts every pull at once; each still runs alone in
  its group, and merges are local operations.
- Guarded by `internal/workspace/workspace_test.go`
  (`TestRefreshConcurrencyLimit`, `TestWritesWaitForRefresh`) and the UI
  refresh-queue tests.
