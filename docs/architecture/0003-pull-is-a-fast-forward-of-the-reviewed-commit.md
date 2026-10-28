# 3. Pull is a fast-forward of the reviewed commit

- Status: Accepted
- Date: 2026-10-27

## Context

`git pull` fetches commits the user has not seen, then merges or rebases
according to `pull.rebase`, `pull.ff` and similar settings. It can create
merge commits, start a rebase, and with `merge.autoStash` stash local
changes. A plain fast-forward also overwrites ignored files (for example a
local `.env`) when the incoming commit adds a file with the same name.

gitdrift shows what a pull would bring before the user confirms it. What
lands must be exactly that.

## Decision

- Pull never fetches. It merges the upstream commit shown in the confirm
  dialog:
  `git merge --ff-only --no-autostash --no-overwrite-ignore --end-of-options <sha>`.
- Before merging, `ops.Pull` re-reads the status and skips, with a reason,
  when:
  - any `drift.PullBlockers` entry applies: tracked local changes, diverged
    branch, detached HEAD, an operation in progress, no or gone upstream,
    the upstream's remote failed or was not fetched, or the repository could
    not be inspected;
  - HEAD moved or the checked-out branch changed (`repository changed since
    review, press r`);
  - the upstream tip moved (`upstream changed since review, press r`).
- The context is checked once more; then the merge runs detached from
  cancellation with its own 5 minute timeout (decision 8).
- Results stay on the row until the user refreshes it: `pulled N` plus the
  badges, `skipped: <reason>`, or `pull failed: <git's line>`.

## Consequences

- The user's pull settings are irrelevant; no merge commit, rebase or stash
  is ever created.
- Nothing arrives that the dialog did not show; a concurrent fetch in
  another terminal cannot widen the pull.
- git itself refuses to overwrite an untracked or ignored file and to touch
  a locally edited file; the error is shown as git wrote it.
- Diverged branches are never pulled; that stays the user's job in git.
- Guarded by `internal/ops/pull_test.go` (ignored `.env`, user pull config,
  HEAD/branch/upstream moved, staged-only change) and the end-to-end test
  `internal/ui/e2e_test.go`.
