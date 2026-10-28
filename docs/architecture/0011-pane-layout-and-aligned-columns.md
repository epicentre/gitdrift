# 11. One screen of panes with aligned, full-width columns

- Status: Accepted. The compact drop order is superseded by
  [17](0017-folders-in-the-repos-pane.md).
- Date: 2026-10-28

## Context

The user chose, from three mockups, a full-screen layout with panes that
feed each other, sized for a folder of about 30 repositories. Trying an
early build on a real workspace showed two problems: long repository and
branch names were cut although the pane had room, and the tokens after the
branch were joined with spaces, so the same kind of value started at a
different column on every row.

## Decision

- Repos, Commits and Files are stacked on the left (44% of the width, at
  least 40 columns); Diff is on the right. The focused left pane takes most
  of the height and the others collapse to three rows. `z` zooms the
  focused pane; on Repos it opens a table with every column. Below 90
  columns the Diff pane is hidden; below 60×15 the screen asks to be
  resized.
- Every list is drawn by one column helper (`internal/ui/columns.go`):
  - column widths come from all rows of the list, not the visible window, so
    scrolling never shifts columns;
  - spare width is spread over the gaps, so a list spans its pane;
  - when a list is too wide, shrinkable columns are cut with `…` down to
    their minimums, then whole columns are dropped by priority;
  - all widths are display widths.
- The Repos pane shows a header row naming its columns (REPO, BRANCH,
  LOCAL, UPSTREAM, BASE, INCOMING, HERE, GONE) when it has at least four
  inner rows; titles are never cut. BRANCH shrinks first (to 10), then
  REPO (to 13). Columns are dropped in the order GONE, HERE, INCOMING, BASE,
  LOCAL, UPSTREAM, so the upstream state is the last to go. Columns without
  any value take no room.
- Files cut long paths from the left so the file name stays visible; ages
  and line counts are right-aligned. Per-file cells and badges are computed
  once when the list loads, not on every frame.

## Consequences

- Rows read as a table and stay stable while scrolling and refreshing.
- On a narrow pane whole columns hide for every row; the zoomed table always
  shows them.
- The UI tests cover every size from 60×15 upward with wide (CJK, emoji)
  text and check that each line is exactly its pane's width.
