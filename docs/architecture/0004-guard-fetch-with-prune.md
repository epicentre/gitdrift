# 4. Fetch with prune, guarded against deleting local refs

- Status: Accepted
- Date: 2026-10-28

## Context

Gone branch detection (decision 5) needs remote-tracking refs of deleted
remote branches to disappear, whatever the user's `fetch.prune` says, so the
fetch passes `--prune`. Pruning deletes every local ref that matches a fetch
refspec's destination and is missing on the remote. That is harmless for the
default `+refs/heads/*:refs/remotes/<remote>/*`, but:

- a mirror-style refspec (`+refs/*:refs/*`, as written by
  `git remote add --mirror=fetch`) or `+refs/heads/*:refs/heads/*` makes
  prune delete local branches with unpushed commits;
- `fetch.pruneTags` makes prune delete local-only tags;
- a legacy `.git/remotes/<name>` file can define refspecs that git uses when
  the config has no `url` for that remote, bypassing a config-only check.

All three were reproduced on git 2.54 during review.

## Decision

Before fetching a remote, `ops.Fetch` refuses, without starting the fetch:

- a remote with no `remote.<name>.url` in git config (`ErrNoRemoteURL`);
- a remote with any `remote.<name>.fetch` refspec whose destination is
  outside `refs/remotes/` (`UnsafeRefspecError`); negative refspecs and
  refspecs without a destination are allowed.

The fetch itself is
`git fetch --prune --no-prune-tags --quiet --end-of-options <remote>`, once
per remote per repository group, with a 60 second timeout. A refusal or
failure is stored per remote in the fetch state (decision 7).

## Consequences

- Local branches and tags are never pruned by gitdrift.
- Remotes with mirror-style refspecs show a fetch error, and pulls of
  branches that track them are blocked; such setups are handled in git.
- Guarded by `internal/ops/fetch_test.go` (unsafe refspec, `fetch.pruneTags`,
  url-less remote, legacy remotes file) and `internal/workspace` tests for
  the stored errors.
