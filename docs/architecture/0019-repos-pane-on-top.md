# 19. Put the Repos pane on top at full width

- Status: Accepted
- Date: 2026-10-29

## Context

Decision 11 stacked Repos, Commits and Files in a left column of 44% of the
width, with Diff on the right. On a real workspace with long folder names
and nine columns, a 200-column screen gave Repos about 86 columns: BASE and
HERE were dropped and folders were cut to `…o-services`. With folder tabs
the pane title no longer named the pane, and the user found the screen hard
to read. The user chose, from mockups, Repos across the top with the other
panes below it, and asked for a larger Diff while Repos has focus.

## Decision

- **Layout.** Repos spans the whole width at the top. Below it Commits is
  stacked on Files on the left (44% of the width, at least 40 columns) and
  Diff fills the right; below 90 columns Diff is hidden as before.
- **Heights.** With focus on Repos the bottom row takes 40% of the body (at
  least six rows), shared equally by Commits and Files, and Diff is as tall
  as the bottom row. With focus on Commits or Files, Repos shrinks to eight
  rows and the focused pane takes 60% of the rest. With focus on Diff the
  panes keep the sizes of the last bottom pane, and sizes as for Commits when
  focus came from Repos, so that Diff is large when it is read. On a short
  screen Repos gives way first, so the bottom panes keep at least three rows
  each and the focused one four.
- **Prominence.** Every pane title names its pane (`1 Repos (N)` with the
  number of repositories shown, `2 Commits`, `3 Files`, `4 Diff`). The
  focused pane has a thick border; the others keep the thin rounded one. The
  folder tabs leave the title for the first line inside the Repos pane, the
  active tab drawn as a filled box in reverse video, so that it stays
  visible where colors are off. Column titles are bold.
- Zoom is unchanged, and below 90 columns Diff stays hidden; Commits and
  Files then span the width below Repos, with the same heights.

## Consequences

- The compact Repos table has the whole width: every column and whole
  folder names fit on a wide screen, and the drop order of decision 17
  applies only to narrow screens.
- While Repos has focus, a 44-row screen shows about 20 repositories
  instead of 31, and Diff is a 14-line preview; `z` still shows every row.
- Guarded by `TestComputeLayout`, `TestComputeLayoutSmallScreens`,
  `TestBoxBorders`, `TestFolderTabBar`, `TestReposTitle`,
  `TestRepoHeaderIsBold`, the resize sweep and the goldens in
  `internal/ui/testdata`.
