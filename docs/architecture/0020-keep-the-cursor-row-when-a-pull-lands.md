# 20. Keep the cursor's row when a pull lands

- Status: Accepted. While other refreshes run the rows hold their places
  and do not re-sort; see [22](0022-read-first-and-hold-rows-while-fetching.md).
- Date: 2026-11-04

## Context

The Repos pane sorts repositories by category: needing attention, behind,
up to date. Once the user moves the cursor, it stays on its repository when
the rows re-sort, so that a refresh never swaps the repository under it.

A successful pull broke the flow this was meant to protect. The pulled
repository becomes up to date and drops into the up-to-date rows, often to
the bottom of the list, and the cursor went with it. The usual next step is
to review and pull the next behind repository, which by then sits where the
pulled one was, so the user had to walk back up to it after every pull.

## Decision

- When a refresh shows the cursor's repository up to date for the first
  time after a successful pull of it, the cursor keeps its row instead of
  following the repository. The repository below moves up under the cursor,
  and the Commits pane shows its incoming commits.
- The cursor follows the repository as before when the pull failed or was
  skipped, when the repository needs attention after the pull, and when the
  user moved the cursor to another repository while the pull ran.
- How the pull started does not matter: `p` on one repository, a selection,
  or `P`.
- Refreshes with `r` and `R` keep the cursor on its repository, as before.

## Consequences

- Pulling behind repositories one after another is `p`, `y`, then `p` again
  on the next one, without moving the cursor.
- The pulled repository leaves the cursor; its result stays on its row until
  `r` or `R` clears it.
- The rule reads the kept pull result: a successful result and a category
  that changes to up to date. Refreshes that find the repository up to date
  again change nothing.
- Guarded by `TestPullLandedKeepsCursorRow` in `internal/ui`.
