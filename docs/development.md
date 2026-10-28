# Development

This page is for working on gitdrift. Start with
[CONTRIBUTING.md](../CONTRIBUTING.md) for the rules, and with the
[architecture overview](architecture/README.md) for how the code is laid out.

## Setup

- Go: the toolchain pinned in `go.mod` is downloaded automatically.
- git 2.30 or newer.
- For screenshots only: [VHS](https://github.com/charmbracelet/vhs) v0.12.1
  with `ttyd` and `ffmpeg` (`brew install vhs` installs all three).

Development tools (golangci-lint, govulncheck) are pinned in `tools/go.mod`
and run through `go tool`; nothing needs a global install.

## Everyday commands

```
make build        # bin/gdrift with version information
make test         # go test -race -shuffle=on ./...
make lint         # golangci-lint
make fmt          # format; make fmt-check fails on unformatted code
make check        # fmt-check, lint, test, vuln and the English check
make cover        # coverage.out and coverage.html
make fuzz         # fuzz the git output parsers (FUZZTIME=30s by default)
make tidy         # go mod tidy for both modules
```

Run `make check` before sending a change. Never point a development build
at your real repositories while testing a change that writes; use the demo
workspace (`make demo`, then `bin/gdrift demo/work`).

## Tests

Tests run real git; see
[decision 14](architecture/0014-test-against-real-git.md) for why.

- **Fixtures.** `internal/gittest` creates repositories, clones and bare
  remotes under `t.TempDir()` with isolated git config. Use
  `Env.Runner()` for the `git.Runner` under test, so the test never reads
  your own config.
- **Parallel.** Every test calls `t.Parallel()` (the `paralleltest` linter
  enforces it). Never use `t.Setenv`; pass environment through the fixture.
- **Assertions.** stdlib `testing` and `go-cmp`; table tests for variants.
- **Race windows.** To hit a window deterministically, use git's own hooks
  instead of sleeps. Examples in the tree: an `uploadpack` script that moves a
  branch during `ls-remote` (`internal/ops/gone_test.go`), a
  `reference-transaction` hook that holds `branch -D`, a `post-merge` hook
  that keeps a pull running while the workspace closes
  (`internal/workspace/workspace_test.go`). Point `core.hooksPath` at a
  directory in `t.TempDir()`.
- **Processes.** Wait for the condition you need, not for a file to appear:
  a shell creates a file before it writes to it (see `waitForPid` in
  `internal/git/runner_unix_test.go`).
- **Goroutines.** `internal/workspace` runs `goleak.VerifyTestMain`; a test
  that blocks a goroutine must release it in `t.Cleanup`.
- **Fuzzing.** `FuzzParseStatusV2` and `FuzzParseLog` cover the porcelain
  parsers; add seeds when you change the formats.

### UI tests

- Drive `Update` directly with the fake backend in
  `internal/ui/fake_test.go`; `fixtureSnapshots` is the shared English
  fixture.
- Golden files live in `internal/ui/testdata`. After an intended change,
  regenerate them and read every diff before keeping it:

  ```
  go test ./internal/ui -update
  ```

- The resize sweep draws every width from 60 to 200 at sampled heights, with
  wide (CJK, emoji) text, and checks that every line is exactly its pane's
  width. Keep it passing when you touch layout or columns.
- The width checks assume narrow ambiguous characters; tests that compare
  exact widths skip themselves when `RUNEWIDTH_EASTASIAN` is set.
- `internal/ui/e2e_test.go` runs the real workspace through `teatest` and
  pulls in real repositories.

## Common changes

### Adding a read-only git command

1. Add the subcommand (or its exact flags) to the read allowlist in
   `internal/git/policy.go`. Prefer an explicit flag list for commands that
   can run programs or write files.
2. Add accepted and refused rows to `TestClassify`.
3. Put repository-derived operands after `--end-of-options`, and qualify
   branch names as `refs/heads/<name>`.

A command that writes is not a routine change: it needs a discussion first,
a new decision record, and updates to `policy.go`, AGENTS.md,
CONTRIBUTING.md, the README's Safety section and the
[security model](security-model.md).

### Adding a user-facing message

Keep copy short and lowercase, as the existing messages are, and add it to
[troubleshooting](troubleshooting.md) with its cause and fix. Pass any text
that comes from a repository or remote through `sanitize` before rendering.

### Adding a column or list

Use the column helper in `internal/ui/columns.go`: give the column a minimum
width, whether it may shrink, and its drop priority. Measure widths from all
rows, compute per-row cells when the data loads rather than in `View`, and
let the resize sweep check the result.

### Changing a decision

Add a new record in `docs/architecture/` that supersedes the old one, and
change the old record's status to `Superseded by N`. Do not rewrite an
accepted record.

## Demo workspace and screenshots

`scripts/demo-workspace.sh <dir>` builds twelve Go repositories with local
bare remotes under `<dir>`; it never touches the network and refuses a
non-empty directory. `make demo` builds it into `./demo` (git-ignored).

`make screenshots` builds `bin/gdrift` and renders `assets/vhs/*.tape` into
`assets/images/`. Each tape uses a throwaway `HOME`, so your git config does
not leak into the images, and removes it afterwards only if it carries the
tape's marker file. After a `Screenshot` command, keep a `Sleep` before the
next key: VHS captures the frame slightly later, and a key typed at once
ends up in the image. Open every image after rendering.

## Dependencies and versions

- Use the latest stable version of every dependency and tool; check with
  `curl -s https://proxy.golang.org/<module>/@latest`. Ask before adding a
  dependency.
- Tools: `cd tools && go get <module>@<version> && go mod tidy`.
- GitHub Actions are pinned by commit SHA with a `# vX.Y.Z` comment; resolve
  the SHA with `git ls-remote https://github.com/<owner>/<repo>
  refs/tags/<tag>` (for annotated tags, the `^{}` line). Dependabot proposes
  updates weekly.

## Conventions

- English only in code, comments, UI text, test data, docs and commit
  messages; `make english` checks for Turkish characters.
- Errors are wrapped with `%w`, lowercase, without trailing punctuation;
  error types are named `XxxError`.
- Every `//nolint` names the linter and gives a reason.
- Commits follow [Conventional Commits](https://www.conventionalcommits.org/).
