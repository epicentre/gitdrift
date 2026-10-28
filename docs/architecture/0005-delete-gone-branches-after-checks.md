# 5. Delete gone branches with `-D` after pre-checks

- Status: Accepted
- Date: 2026-10-27

## Context

After a merge request is merged, the remote branch is usually deleted and
the local branch stays behind with an upstream that no longer exists
("gone"). Teams that squash-merge leave the branch's own commits outside the
base, so `git branch -d` refuses to delete them. The user decided that
gitdrift may use `git branch -D` for such branches, provided it shows what
would be lost, re-checks right before deleting, and prints how to undo it.

## Decision

- A branch is gone when its upstream is a remote-tracking ref that no longer
  exists. Branches that never had an upstream are not gone; their commits
  appear as work only on this machine.
- The Branches tab offers deletion only for gone branches, never for the
  current branch or a branch checked out in another worktree of the group,
  and not when the remote's last fetch failed or did not run.
- The confirm dialog lists, per branch, the tip and the commits that exist
  only on this machine (up to five, then `+N more`), loaded before it opens.
- `ops.DeleteGone` then checks, in order, with branch names qualified as
  `refs/heads/<name>`: the branch still exists; its tip is the reviewed SHA;
  it is not checked out in any worktree; its upstream is still gone;
  `git ls-remote --exit-code` confirms the branch is absent on the remote
  (exit 2; any other result skips with `could not verify remote: …`).
- After that network call it checks the tip and the checkouts again, checks
  the context, and runs `git branch -D --end-of-options <name>` detached from
  cancellation with a 30 second timeout (decision 8).
- The result window prints `git branch <name> <sha>` to restore each branch.
  Names with characters a shell treats specially are single-quoted; names
  with control or format characters are written as `"$(printf '…')"` so the
  command restores the exact name.

## Consequences

- A branch that changed, is checked out, or still exists on the remote is
  never deleted.
- After deletion the commits are unreachable and git's garbage collection
  can remove them; the branch's reflog is deleted with it. The README tells
  users to keep the restore command and not to rely on the reflog.
- A window of milliseconds remains between the last re-check and
  `branch -D`, which takes no expected old value.
- Guarded by `internal/ops/gone_test.go`, including
  `TestDeleteGoneRecheckAfterLsRemote`, which moves the branch from an
  `uploadpack` hook during the `ls-remote` call.
