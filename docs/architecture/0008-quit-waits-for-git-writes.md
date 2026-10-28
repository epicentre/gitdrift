# 8. Quitting waits for git writes

- Status: Accepted
- Date: 2026-10-28

## Context

git children run in their own session (decision 2) and write their output
to pipes that gitdrift reads. If gitdrift exits while `git merge` or
`git branch -D` runs, the next write to the closed pipe raises SIGPIPE and
kills git in the middle of the operation. Cancelling those writes with
SIGKILL would leave lock files. Uncancelled fetches would outlive gdrift.
Closing the terminal window sends SIGHUP, whose default action kills the
process at once.

## Decision

- `main` cancels the program context on SIGINT, SIGTERM and SIGHUP, and
  Bubble Tea runs with that context and without its own signal handler, so
  there is one shutdown path. `main` keeps `os.Exit(run(...))` with every
  cancel and wait inside `run`.
- After the program returns for any reason, `start` cancels the context,
  which stops refreshes, fetches and `ls-remote` checks (decision 2), and
  then calls `Workspace.Close`.
- `Close` marks the workspace closed and waits for every backend call in
  flight; calls made after it return `ErrClosed` without running git. The
  count is a mutex-guarded counter with a condition variable, so no
  `WaitGroup.Add` races `Wait`.
- Writes that must finish once started run under `context.WithoutCancel`
  with their own timeout, after a final context check: merge (5 minutes),
  `branch -D` (30 seconds), config writes (10 seconds).
- When waiting takes more than a second, gdrift prints
  `waiting for git to finish…` once on stderr.

## Consequences

- No half-applied merge or deletion, and no lock files left by gitdrift's
  own writes.
- In a rare case quitting waits up to a write's timeout.
- A second `ctrl+c` during that wait is ignored.
- Guarded by `cmd/gdrift/start_test.go` (`TestRunDashboardStopsFetchOnQuit`,
  `TestWaitForGit`), `internal/workspace` (`TestCloseWaitsForPull`) and
  `internal/ops/gone_test.go` (`TestDeleteGoneFinishesOnCancel`).
