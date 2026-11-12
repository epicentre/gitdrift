# 22. Read every repository first and hold the rows while they fetch

- Status: Accepted. Refines when the rows re-sort for
  [20](0020-keep-the-cursor-row-when-a-pull-lands.md).
- Date: 2026-11-12

## Context

At start the UI refreshed every group with a fetch. Rows showed
`fetching…` until their group answered, and every answer re-sorted the
rows: a repository moved from the unread rows at the bottom to its category
the moment its fetch finished. In a folder with many repositories the
fetches finish in random order over many seconds, so the list kept moving
while the user tried to read it or work in it. Nothing showed how much of
the loading was left.

Most of what a row shows is known without the network: the branch, local
changes, divergence from the last fetched upstream, the base and the work
only here. Only the upstream counts and the incoming files can change with
a fetch.

## Decision

- At start every group is read without a fetch first. Once every group has
  answered, the rows sort, and then every group fetches.
- Rows re-sort only when no refresh is running, and once when the first
  reads are done. An answer that arrives while other refreshes run updates
  its row in place: its marker, tokens and status show the new state, but
  the row keeps its position. This holds for the start, for `R` and for a
  pull that lands while refreshes run; a single `r` re-sorts as soon as it
  answers.
- A row that has not been read yet, or whose group fetches, shows a spinner
  in place of its marker and its cells muted. Its status line says
  `reading…` before the first read and `fetching…` during a fetch.
- The header ends with `reading N/M` until every repository was read, then
  with `fetching N/M` over the repositories whose groups fetch, until the
  last fetch answers.
- The spinner ticks only while something loads.
- Pulls stay blocked until the repository's remote was fetched
  (`remote not fetched yet`, decision 7); a fetched repository can be pulled
  while others still fetch.

## Consequences

- The list is usable within the time of a local `git status` per
  repository, sorted by what is known locally, and holds still while the
  fetches run. It re-sorts once at the end; a cursor the user moved stays
  on its repository.
- Every repository is read twice at start, once without and once after its
  fetch.
- The cursor keeping its row after a pull (decision 20) applies only when the
  rows sort right after that pull, that is when no other refresh runs.
- A row in place may show a marker that its position does not match until
  the last refresh answers.
- Guarded by `TestStartupReadsBeforeFetching`,
  `TestRowsKeepTheirPlaceWhileLoading`, `TestHeaderShowsLoadProgress`,
  `TestSpinnerTurnsOnlyWhileLoading` and
  `TestBusyRowShowsSpinnerAndMutedTokens` in `internal/ui`, and the
  end-to-end test `internal/ui/e2e_test.go`.
