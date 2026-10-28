# 7. Fetch state fails closed

- Status: Accepted
- Date: 2026-10-28

## Context

Pull selectability and gone branch deletion depend on remote-tracking refs
being current. A review found that a fetch skipped because the first
worktree of a group could not be read was stored as "done, no errors", so
stale refs looked fresh. Separately, users could not tell an authentication
failure from a timeout or from gitdrift's own refusal (decision 4), because
only `fetch failed: <remote>` was shown.

## Decision

- `drift.FetchState` holds `Done` and a map of per-remote error messages.
  `OK(remote)` is true only when the fetch is done and that remote has no
  error; the local remote `.` is always current.
- When no member of a group can list its remotes, a requested fetch yields
  `Done: false`; `Done: true` is set only after every listed remote was
  attempted.
- Each error is stored as one short line chosen by `git.Message`: git's
  `fatal:`/`error:` line (else the last non-empty stderr line), `timed out`
  for a deadline, or the refusal's own text.
- `drift.PullBlockers` reports `fetch failed: <remote>` for an error and
  `remote not fetched yet: <remote>` when the fetch did not run; the gone
  branch refusals use the same copy. The cursor repository's status line
  appends the message, sanitized.

## Consequences

- An unknown remote state blocks pulls and deletions instead of allowing
  them.
- The user sees why a fetch failed.
- A group that cannot be read shows blocked pulls until it can be read.
- Guarded by `internal/workspace/workspace_test.go`
  (`TestRefreshNoMemberListsRemotes`, `TestRefreshSurvivesRemovedFirstMember`,
  `TestRefreshKeepsFetchErrors`) and `internal/drift` tests.
