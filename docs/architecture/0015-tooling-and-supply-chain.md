# 15. Pin tools in a module and actions by commit SHA

- Status: Accepted
- Date: 2026-10-28

## Context

Developers and CI must use the same tool versions. golangci-lint brings a
large dependency tree (over 200 requirements) that has nothing to do with
gitdrift's binary. GitHub Actions referenced by tag run whatever the tag
points to at run time; a compromised action repository can move a tag, and
the release workflow holds write permissions.

## Decision

- Development tools are pinned with `tool` directives in a separate module,
  `tools/go.mod` (golangci-lint, govulncheck), and run as
  `go tool -modfile=tools/go.mod <tool>`. The root `go.mod` lists only what
  gitdrift itself needs.
- Every GitHub Action is pinned by full commit SHA with a `# vX.Y.Z` comment.
  Dependabot updates the root module, the tools module and the actions
  weekly.
- Workflows start with read-only permissions; only the jobs that need more
  get it (CodeQL `security-events: write`; release `contents: write`,
  `id-token: write`, `attestations: write`). Checkouts do not persist
  credentials. The release workflow does not cancel a release in progress.
- Releases are built by GoReleaser on a `v*` tag for macOS and Linux (amd64,
  arm64) with checksums and build provenance attestation.

## Consequences

- A clean, readable root `go.mod`; tool upgrades do not touch it.
- Commands for the tools are longer; the Makefile hides them.
- A moved tag cannot change what CI runs; upgrades arrive as reviewable
  Dependabot pull requests.
