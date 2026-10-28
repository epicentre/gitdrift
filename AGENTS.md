# AGENTS.md

Guidance for coding agents working on this repository.

## Project

gitdrift (`gdrift`) is a terminal dashboard for drift across the git
repositories in one folder: which repos are behind their upstream, which
feature branches fell behind their base branch, what a pull would bring in,
what work exists only on this machine, and which local branches were deleted
on the remote. It is not a git UI: it never commits, pushes, checks out,
stashes or resets.

- Design spec (source of truth for behavior, written in Turkish):
  `docs/superpowers/specs/2026-10-27-gitdrift-design.md`
- Implementation plan: `docs/superpowers/plans/2026-10-27-gitdrift.md`

## Hard rules

1. **Never lose user code.** Every git process goes through `internal/git`,
   whose allowlist permits exactly these writing commands and nothing else:
   - `git fetch --prune --no-prune-tags --quiet --end-of-options <remote>`
     (refused for a remote with no url in git config or whose fetch refspec
     writes outside `refs/remotes/`, since pruning would delete local refs)
   - `git merge --ff-only --no-autostash --no-overwrite-ignore --end-of-options <sha>`
   - `git branch -D --end-of-options <name>` (gone branches only, after the
     pre-checks in `internal/ops`)
   - `git config --local gitdrift.base <value>` and
     `git config --local --unset gitdrift.base`

   Do not add writing commands, do not weaken the allowlist, never use
   `sh -c`, and never override the user's `GIT_SSH_COMMAND`, credential
   helper or git config.
2. **English only in code.** Identifiers, comments, UI text, test data and
   commit messages are English. No Turkish words or characters anywhere in the
   codebase (the specs and plans under `docs/superpowers/` are the only
   exception).
3. **No commits.** Do not run `git commit`, `git push`, `git tag` or create
   remotes. When a change is done, propose a Conventional Commits message for
   the human to use.
4. **Latest stable versions.** Before adding or upgrading a dependency or
   tool, check the latest version with
   `curl -s https://proxy.golang.org/<module>/@latest`. Ask before adding a
   dependency the plan does not list.
5. **README rules.** The README states that gitdrift is not a git UI and was
   built for the drift problem. It never names other tools.

## Go skills

For any Go task, load the `golang-how-to` skill first; it routes to the
skills in `.agents/skills/` (also linked from `.claude/skills/`). The ones this
project leans on: `golang-testing`, `golang-error-handling`,
`golang-concurrency` with `golang-context`, `golang-lint`, `golang-naming`,
`golang-code-style`, `golang-security`, `golang-documentation`,
`golang-continuous-integration`, `golang-modernize`.

## Layout

```
cmd/gdrift/          flags, version, exit codes, wiring (main.go stays small)
internal/git/        Runner (exec), allowlist policy, git version check
internal/gittest/    throwaway repos and bare remotes for tests
internal/repo/       read-only inspection: status, base, work, details
internal/badge/      "what to do after pull" badge rules
internal/drift/      Snapshot and pure classification (safe, attention)
internal/ops/        the only writing operations, with pre-checks
internal/workspace/  repo discovery, bounded parallel refresh, UI backend
internal/ui/         Bubble Tea v2 model, panes, dialogs
tools/               separate module pinning dev tools (go tool)
scripts/, assets/    demo workspace, screenshot tapes (assets/vhs) and images
```

## Commands

```
make build          # bin/gdrift with version ldflags
make install        # go install ./cmd/gdrift
make test           # go test -race -shuffle=on ./...
make cover          # coverage.out and coverage.html
make fuzz           # fuzz the porcelain parsers (FUZZTIME=30s)
make lint           # golangci-lint run ./...
make fmt            # golangci-lint fmt ./...
make vuln           # govulncheck ./...
make check          # fmt-check, lint, test, vuln, english check
make release-check  # goreleaser check
make snapshot       # goreleaser release --snapshot --clean
make demo           # build the demo workspace used for screenshots
make screenshots    # render assets/images with vhs
```

## Conventions

- Go 1.27 (`go 1.27.0`, `toolchain go1.27.3`); stdlib `flag` for the CLI.
- `ctx context.Context` is the first parameter; errors are wrapped with `%w`,
  lowercase, no trailing punctuation; error types are named `XxxError`.
- Tests use stdlib `testing` and `go-cmp`, table tests with `t.Parallel()`,
  and real git in `t.TempDir()` through `internal/gittest`. Git config is
  isolated per command through environment variables, never `t.Setenv` in
  parallel tests. TUI golden files live in `internal/ui/testdata` and are
  refreshed with `go test ./internal/ui -update`.
- Lint config is the golang-lint skill's `.golangci.yml`; every `//nolint`
  names the linter and gives a reason.
- UI: Bubble Tea v2 (`charm.land/bubbletea/v2`), `View()` returns `tea.View`
  with `AltScreen` set. Strip ANSI and control characters from every string
  that comes from a repository before rendering it.
