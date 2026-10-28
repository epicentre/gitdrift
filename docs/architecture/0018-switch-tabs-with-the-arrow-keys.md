# 18. Switch tabs with the arrow keys

- Status: Accepted
- Date: 2026-10-29

## Context

Decisions 11 and 17 switch the tabs of the focused pane, the Commits tabs and
the folder tabs of the Repos pane, with `[` and `]`. On keyboard layouts such
as Turkish Q these need AltGr, which is slow for a key that is pressed often.
`←`, `→`, `h` and `l` were not bound to anything.

## Decision

- `←` and `h` switch to the previous tab, `→` and `l` to the next one, in
  the same panes as before: the folder tabs in Repos when there are folders,
  the Commits tabs otherwise and in the other panes.
- `[` and `]` keep working, so nothing a user learned stops working.
- The key hints show `←→ folders` and `←→ tabs`; the help window lists all
  three keys of each direction.
- While a filter is typed, `h` and `l` are typed like any letter, and `←` and
  `→` do nothing, as the other keys without text.

## Consequences

- Tabs switch without a modifier on every layout; the arrow keys match the
  tabs, which sit side by side.
- `h`, `l`, `←` and `→` cannot be given another meaning in these panes.
- Guarded by `TestFolderTabKeys`, `TestTabKeysCycle`, `TestFilterTakesKeys`,
  `TestFilterIgnoresOtherKeys`, `TestKeyMapHelp` and the footer tests and
  goldens in `internal/ui`.
