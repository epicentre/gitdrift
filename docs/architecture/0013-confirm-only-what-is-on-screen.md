# 13. Confirm only what is on screen

- Status: Accepted
- Date: 2026-10-28

## Context

The confirm dialogs are the user's last check before a write. A review found
that below the minimum size, where the screen shows only "terminal too
small", keys still reached a dialog that was not drawn: `y` confirmed a
pull, and any key dismissed a deletion's result window together with its
restore commands. Refreshes can also arrive while a dialog is open.

## Decision

- A dialog keeps the snapshots it renders, and its requests are built from
  them; a refresh never changes an open dialog. The writes re-check the
  repository anyway (decisions 3 and 5).
- While a window cannot be drawn: action keys are ignored except quit; a
  hidden confirm dialog accepts only cancel; the hidden base picker accepts
  only `esc`; a deletion's result window stays open until it can be shown.
- `ctrl+c` always quits, through the normal quit path (decision 8); `q` is
  ignored while a dialog is open.
- The base picker starts on the current base, ignores `enter` while its list
  loads, and ignores `enter` when a typed filter matches no branch, so it
  never resets the base by accident.
- While a pull runs, its row shows `pulling…` and cannot be selected or
  pulled again.

## Consequences

- No write is confirmed without the user seeing what it does.
- A terminal below 60×15 cannot act until it is resized.
- Guarded by `internal/ui/dialogs_test.go`, `actions_test.go` and
  `picker_test.go`.
