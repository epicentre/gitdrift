# 16. Discover repositories recursively, bounded by a depth

- Status: Accepted
- Date: 2026-10-28

## Context

Workspaces are not always flat: a projects folder often groups repositories
by team or product (`services/platform/billing-api`). Looking only at the
direct children of the folder missed them. A deep walk has a cost, though:
gdrift may be started in a home directory, where an unbounded walk would
touch hundreds of thousands of directories, run git in each, and on macOS
trigger folder permission prompts.

## Decision

`workspace.Discover` walks the tree under these rules:

- The walk is breadth-first, each level in name order, down to `--depth`
  levels below the root (default 5; the root's children are level 1). The CLI
  refuses values below 1.
- The root itself is located first and listed when it is a repository.
- A directory with a `.git` entry (directory or file) is located with git,
  listed and never entered, so repositories inside repositories below the
  root are not listed. The root is the exception: it is always located, and
  when it is a repository its subdirectories are still searched. Linked
  worktrees have a `.git` file and are listed at any level.
- A directory whose `.git` entry git finds no work tree for (`repo.Locate`
  reports `ErrNotRepo`) is walked like a plain directory. This lists the
  worktrees of a bare clone kept beside them: `proj/.git` is a file that
  points at `proj/.bare`, and the worktrees are `proj/main` and
  `proj/feature`. `.bare` is hidden and stays skipped.
- `.git` is checked before the name filter: a repository named like a skipped
  directory is listed. A directory that is not a repository and is hidden
  (starts with `.`), `node_modules` or `vendor` is not entered.
- Below the root, git is started only for directories with a `.git` entry;
  plain directories cost a directory read.
- Symlinks are followed. Each symlink-resolved directory is visited once, so
  loops end and a repository reached through several routes is listed once,
  with the folder of the shallowest route (the first one the walk takes).
- Errors reading the root are fatal. A directory that cannot be read, or a
  repository that cannot be located, is skipped so that it does not hide the
  others.

The repository's folder is its slash-separated path below the root, and the
UI uses it (decision 17).

## Consequences

- Nested independent repositories (a repository inside another one's work
  tree) are not listed, nor are linked worktrees placed inside a work tree;
  start gdrift inside the outer repository to see them.
- A repository deeper than the limit is not listed; `--depth` raises it.
- Starting gdrift in a home directory walks it to the depth limit. On macOS
  this can show permission prompts for Documents, Desktop or Downloads; the
  documentation recommends a projects folder or a lower `--depth`.
- Every listed repository is inspected, so the security note of running
  gdrift only on trusted folders applies to the whole searched tree.
- Guarded by `internal/workspace/discover_test.go`, including a test that
  counts git calls over a tree of many plain directories.
