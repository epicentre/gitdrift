# 9. Resolve the base branch from repo, global, then auto

- Status: Accepted
- Date: 2026-10-27

## Context

A feature branch drifts not only from its own upstream but from the branch
it will be merged into. Teams use different bases (`main`, `master`,
`develop`), and a single repository can differ from the rest.

## Decision

- The base is the git config key `gitdrift.base`, read with
  `config --show-scope`: a value in the repository or worktree config wins
  over a global or system value.
- A value whose first segment is a remote name is a remote branch
  (`upstream/main`); any other value (`develop`) is a branch on the
  reference remote: the branch's upstream remote, else `origin`, else the
  only remote.
- Without a setting the base is detected: `<remote>/HEAD`, then `main`, then
  `master` on the reference remote.
- The base is always a remote-tracking branch, compared as
  `refs/remotes/<ref>`. A configured base that does not exist shows `base?`
  and marks the repository as needing attention.
- Being behind the base by one commit is enough to count; there is no
  threshold.
- The in-app picker (`b`) writes `config --local gitdrift.base <ref>` or
  `--unset`; the value stays in git config, visible to git and to the user.

## Consequences

- Works without any configuration in most repositories.
- Per-repository overrides persist outside gitdrift.
- A local-only branch cannot be a base.
- Guarded by `internal/repo/base_test.go` and `internal/ops/base_test.go`.
