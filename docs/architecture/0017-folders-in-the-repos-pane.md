# 17. Show folders in the Repos pane: column, tabs and a filter

- Status: Accepted. The tab keys are superseded by
  [18](0018-switch-tabs-with-the-arrow-keys.md), the place of the tabs, in
  the pane title, by [19](0019-repos-pane-on-top.md).
- Date: 2026-10-28

## Context

With nested discovery (decision 16) a workspace of 60 repositories in six
folders is one long list. The user chose, from three mockups, a single list
that keeps every repository in one place, over separate panes or a tree, and
asked for a way to narrow it.

## Decision

- **FOLDER column.** Each row shows its folder below the workspace in a
  FOLDER column. A workspace where every repository is directly in the folder
  has no FOLDER column and no tabs, and renders as before.
- **Names.** Where one repository is named alone (the status line, the pull,
  delete and base dialogs, the notices) a repository in a folder is named
  `folder/name`, so that same-named repositories in different folders are
  told apart; one directly in the workspace keeps its name.
- **Sorting.** Rows are sorted by category (needs attention, behind, up to
  date), then folder (one name at a time, so `services` comes before
  `services/platform` and `services-x`), then name, then path.
- **Tabs.** The Repos title shows All and one tab per top-level folder, with
  counts (`shown/total` when filters hide some). A repository at the root
  appears only in All. In a folder tab FOLDER shows the path below the tab.
  `[` and `]` switch tabs.
- **Filter.** `/` types a case-insensitive filter matched against the
  sanitized name or folder. While it is typed, keys with text, including `q`
  and digits, are typed, and a paste types its first line; `enter` keeps it
  and `esc` clears it; `ctrl+c` still quits. A window that would open while
  the filter is typed waits until typing ends. A kept filter starts the
  status line, followed by the status of the repository under the cursor, so
  that its attention reasons stay in sight.
- **Visible-only actions.** `P`, `p`, `R` and `r` act on the rows that pass
  the tab, the filter and `f`. A hidden repository is never pulled. `R`
  refreshes the whole group of each repository shown, so a hidden linked
  worktree of a shown repository is refreshed too. Hidden
  and no-longer-safe selections are dropped on every change of the visible
  set and on every refresh, with a notice.
- **Compact column order.** The compact drop order is now GONE, HERE,
  INCOMING, BASE, FOLDER, LOCAL, UPSTREAM. FOLDER shrinks first, cut at a `/`
  boundary down to 10 cells (`…/platform`). This supersedes the drop order in
  decision 11.

## Consequences

- `[` and `]` act on the focused pane: folder tabs in Repos when there are
  folders, Commits tabs otherwise and in the other panes.
- The zoom table's FOLDER column never shrinks, so a long folder squeezes
  REPO and BRANCH in a narrow zoom pane.
- A selection may disappear when the tab, filter or `f` changes; the notice
  says so.
- Guarded by the UI tests added with this decision (`folders_test.go`,
  `filter_test.go`, `repos_test.go`, `actions_test.go`, the goldens in
  `internal/ui/testdata`) and the sort tests in `internal/drift`.
