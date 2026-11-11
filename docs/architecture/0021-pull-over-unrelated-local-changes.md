# 21. Pull over local changes the pull does not touch

- Status: Accepted. Supersedes the local changes blocker of
  [3](0003-pull-is-a-fast-forward-of-the-reviewed-commit.md).
- Date: 2026-11-11

## Context

Decision 3 skipped a pull whenever a tracked file had changes, staged or
not. A common workspace keeps small local edits for weeks, such as a setting
changed in a config file for this machine. Every repository with such an
edit stayed in Needs attention and was never offered for a pull, even when
the incoming commits changed other files; the user had to pull it in git.

The fast-forward that gitdrift runs already handles this case safely.
`git merge --ff-only` keeps local changes to files the merge does not change,
staged ones included, and refuses before it changes anything when the merge
would change a file with local changes or when the index has unmerged
entries.

## Decision

- Local changes stop a pull only when one of their paths is among the files
  the pull would change. The changed paths come from
  `git status --porcelain=v2` (a rename or copy gives both its paths), the
  incoming files from `git diff --no-renames --name-only HEAD...<upstream>`.
  Without rename detection a file the upstream renames or moves lists its old
  path, which the pull removes.
- Unmerged entries stop a pull whatever their paths, as `local changes`.
- Until the incoming files are read for a snapshot, any local change stops
  the pull, as before (fail closed).
- The reason names the file: `local changes to <path>`, or
  `local changes to <path> and N more`. A repository needs attention for its
  local changes only in that case; otherwise it is behind, and `p`, a
  selection and `P` pull it like a clean one.
- `ops.Pull` re-reads the status and, when there are local changes, the
  incoming files right before the merge, so an edit made after the review is
  caught. git's own refusal stays the last check.
- The merge command and the allowlist do not change. gitdrift still never
  stashes.
- The pull dialog ends with `Local changes are kept.` when a repository in it
  has local changes.

## Consequences

- A repository with a local edit to an unrelated file is pulled; the edit
  stays as it was, and staged changes stay staged.
- The check is per file: a local change and an incoming change to different
  lines of one file still stop the pull. Combining them needs a stash or a
  merge, which stays the user's job in git.
- A pull of a repository with local changes runs one more read,
  `git diff --name-only`.
- Badges also see the old path of a renamed file.
- Guarded by `internal/drift/drift_test.go`, `internal/ops/pull_test.go`
  (unrelated edit kept, edited or renamed incoming file skipped, unmerged
  files skipped, staged change kept), `TestRefreshLocalChanges` in
  `internal/workspace` and the end-to-end test `internal/ui/e2e_test.go`.
