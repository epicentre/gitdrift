# Contributing

Thanks for helping with gitdrift (`gdrift`). It is a terminal dashboard for
drift across the git repositories in one folder. It is not a git UI.

## Prerequisites

- Go, using the toolchain pinned in `go.mod` (it is downloaded automatically).
- git 2.30 or newer.
- Optional: [`vhs`](https://github.com/charmbracelet/vhs) to render the
  screenshots (`make screenshots`).

## Workflow

```
make check    # fmt-check, lint, test, vuln, english check
make test     # go test -race -shuffle=on ./...
make lint
```

Run `make check` before sending a change.

## Architecture

Read [docs/architecture](docs/architecture/README.md) before changing
behavior: it maps the packages, the main flows and the invariants, and links
the decision records. A change that departs from a recorded decision adds a
new record that supersedes it, in the same pull request.

## Safety rules

gitdrift must never lose user code. Every git process goes through
`internal/git`, whose allowlist permits exactly these writing commands:

- `git fetch --prune --no-prune-tags --quiet --end-of-options <remote>`
  (refused for a remote with no url in git config or whose fetch refspec
  writes outside `refs/remotes/`, since pruning would delete local refs)
- `git merge --ff-only --no-autostash --no-overwrite-ignore --end-of-options <sha>`
- `git branch -D --end-of-options <name>` (gone branches only, after the
  pre-checks in `internal/ops`)
- `git config --local gitdrift.base <value>` and
  `git config --local --unset gitdrift.base`

Do not add a new writing command or weaken the allowlist without opening an
issue to discuss it first; such a change needs its own decision record (see
[decision 1](docs/architecture/0001-run-git-through-an-allowlist.md)). Never use `sh -c`, and never override the user's
`GIT_SSH_COMMAND`, credential helper or git config.

## Tests

- Use the stdlib `testing` package and `go-cmp`.
- Write table tests and call `t.Parallel()`.
- Use real git in `t.TempDir()` through `internal/gittest`. Isolate git
  config per command through environment variables; do not use `t.Setenv` in
  parallel tests.
- TUI golden files live in `internal/ui/testdata`; refresh them with
  `go test ./internal/ui -update` and review the diff.

[docs/development.md](docs/development.md) covers the test techniques, the
demo workspace and screenshots, and common changes in more detail.

## Code and commits

- Code, comments, UI text and commit messages are in English.
- Every `//nolint` names the linter and gives a reason.
- Use [Conventional Commits](https://www.conventionalcommits.org/), for
  example `fix: handle detached HEAD in status`.
