# 10. Post-pull badges from incoming file names

- Status: Accepted
- Date: 2026-10-27

## Context

After pulling, the work is often not done: dependencies need installing,
migrations need running, and new environment variables need values. The
incoming commits already say whether that is likely, before the pull.

## Decision

- For a repository that is behind, gitdrift lists the files the pull would
  change (the diff from the merge base to the upstream) and matches them
  against rules:
  - `deps`: dependency manifests and lockfiles (`go.mod`, `go.sum`,
    `package.json`, lockfiles of npm, yarn, pnpm and bun, `composer.*`,
    `Cargo.*`, `Gemfile*`, Python requirement and lock files, Maven and
    Gradle files);
  - `mig`: anything under a `migrations`, `migrate` or `alembic/versions`
    directory;
  - `env`: `.env.example`, `.env.sample`, `.env.dist`, `.env.template`.
- Users add rules with the multi-valued git config key `gitdrift.badge`
  (`<label>=<pattern>`). A pattern without a slash matches base names; one
  with a slash matches the full path. Labels are 1 to 12 lowercase letters,
  digits or dashes.
- Badges show on the row, in the zoomed table's INCOMING column, on files in
  the Incoming tab, and in a pull's result, which stays until the row is
  refreshed.

## Consequences

- Cheap: name matching only, no file contents are read.
- A heuristic: unusual layouts need a `gitdrift.badge` rule; a matching
  name does not prove that action is needed.
- Guarded by `internal/badge/badge_test.go` and `internal/repo` incoming
  tests.
