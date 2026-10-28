# 1. Run the git CLI through an argv allowlist

- Status: Accepted
- Date: 2026-10-27

## Context

gitdrift reads and changes git repositories that hold the user's work. The
one outcome that counts as failure even when everything else works is lost
code. Two ways to talk to git were possible: a pure-Go library (go-git) or
the git binary.

The user's git setup has to keep working exactly as in their shell:
credential helpers, `ssh` configuration, `includeIf` sections, hooks,
`fsmonitor`, sparse and partial clones. A library re-implements git and
supports only part of that. The git binary already is the user's git.

## Decision

- gitdrift runs the git binary (version 2.30 or newer, checked at start) and
  nothing else. Only `internal/git.Runner` starts processes; no shell is
  involved (`exec.Command` with an argv, never `sh -c`).
- Every argv goes through `git.Classify` before a process starts. It returns
  `KindWrite`, `KindRead` or refuses with an error wrapping `ErrNotAllowed`.
- `KindWrite` is exactly these five argv forms, compared argument by
  argument:
  - `fetch --prune --no-prune-tags --quiet --end-of-options <remote>`
  - `merge --ff-only --no-autostash --no-overwrite-ignore --end-of-options <sha>`
    (`<sha>` must be a full hex object id)
  - `branch -D --end-of-options <name>`
  - `config --local gitdrift.base <value>`
  - `config --local --unset gitdrift.base`
- `KindRead` is an allowlist of subcommands (`status`, `rev-parse`,
  `rev-list`, `log`, `show`, `diff`, `diff-tree`, `for-each-ref`,
  `merge-base`, `cat-file`, `ls-files`, `version`), plus `ls-remote` and
  `symbolic-ref` with explicit flag lists (others can run programs or write
  refs), `config` only in read forms, `hash-object` without `-w`, `remote`
  without arguments, and `worktree list` / `stash list`.
- A deny list applies to every argv: `--force`, `--force=…`, `--hard`,
  `--autostash`, `--output`, `--output=…` and `--ext-diff`.
- Write operands may not start with `-`. Repository-derived operands follow
  `--end-of-options`, and branch names handed to read commands are fully
  qualified (`refs/heads/<name>`) so a tag with the same name is never
  resolved instead.

## Consequences

- gitdrift behaves like the user's own git, including their authentication.
- Adding a writing command is a visible change to `policy.go`, reviewed with
  this record, AGENTS.md, CONTRIBUTING.md and the README's Safety section.
- git 2.30 or newer must be installed.
- Output is parsed from porcelain and `-z` formats; those parsers are fuzzed
  (`FuzzParseStatusV2`, `FuzzParseLog`).
- Each query is a process start; refreshing ~30 repositories costs a few
  hundred short processes, which is acceptable for an interactive tool.
- Guarded by `internal/git/policy_test.go` (`TestClassify`).
