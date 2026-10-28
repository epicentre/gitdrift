# Configuration

gitdrift has two settings, both plain git config keys. There is no config
file of its own, no environment variable, and no flag besides `--version`
and `--help`.

| Key | Values | Default | Read from |
| --- | --- | --- | --- |
| `gitdrift.base` | one branch, `name` or `remote/name` | detected | repository config first, then global or system config |
| `gitdrift.badge` | `<label>=<pattern>`, may be given several times | none (built-in rules always apply) | every config scope, combined |

## `gitdrift.base`

The base branch is what a feature branch is measured against: the `base↓N`
token, the BASE column and the Base tab.

### How the base is resolved

1. A value in the repository's own config wins: the local config, or the
   worktree config when `extensions.worktreeConfig` is on.
2. Otherwise a value from the global or system config applies.
3. Otherwise the base is detected on the reference remote: its `HEAD`
   (`git remote set-head`), then `main`, then `master`.

The **reference remote** is the current branch's upstream remote, else
`origin`, else the only remote. A repository without a remote has no base.

The status line shows the result and where it came from, for example
`base origin/main (auto)`, `base upstream/develop (repo)` or
`base origin/develop (global)`.

### Values

- `remote/name` (`upstream/main`): used when the first part is the name of
  one of the repository's remotes.
- `name` (`develop`, `release/2026`): looked up on the reference remote.

The base is always a remote-tracking branch. A value that does not exist in
the repository shows `base?` on the row and `base not found: <value>` on the
status line, and the repository is marked as needing attention. Being behind
the base by a single commit counts; there is no threshold. When the base
branch itself is checked out, the BASE column stays empty (`–` in the zoomed
table).

### Setting it

```
git config --global gitdrift.base develop        # every repository
git config --local gitdrift.base upstream/main   # this repository only
git config --local --unset gitdrift.base         # back to the default
```

In the dashboard, `b` opens a picker of the repository's remote-tracking
branches with the current base marked `●`. Type to filter, press `enter` to
choose. Choosing writes `gitdrift.base` to the repository's local config;
**Reset to default** removes it. The picker does nothing while the list is
loading or when the filter matches no branch.

When `extensions.worktreeConfig` is on and a worktree-scoped value exists,
it overrides what the picker writes to the local config. Change such a value
with `git config --worktree`.

## `gitdrift.badge`

Badges tell you what to do after a pull. gitdrift lists the files the pull
would change and labels the repository with every rule that matches one of
them. Badges appear on the row, in the zoomed table's INCOMING column, next
to files in the Incoming tab, and in a pull's result line.

### Built-in rules

| Label | Matches |
| --- | --- |
| `deps` | `composer.json`, `composer.lock`, `package.json`, `package-lock.json`, `npm-shrinkwrap.json`, `yarn.lock`, `pnpm-lock.yaml`, `bun.lock`, `go.mod`, `go.sum`, `Cargo.toml`, `Cargo.lock`, `Gemfile`, `Gemfile.lock`, `requirements*.txt`, `Pipfile`, `Pipfile.lock`, `poetry.lock`, `uv.lock`, `pyproject.toml`, `pom.xml`, `build.gradle`, `build.gradle.kts`, `gradle.lockfile` |
| `mig` | files inside a `migrations`, `migrate` or `alembic/versions` directory, at any depth |
| `env` | `.env.example`, `.env.sample`, `.env.dist`, `.env.template` |

### Adding rules

Each value is `<label>=<pattern>`:

- The label has 1 to 12 characters: lowercase letters, digits and hyphens,
  starting with a letter or digit. A new label adds a badge; an existing one
  (`deps`) adds files to it.
- A pattern without a slash matches the file name at any depth
  (`*.proto`). A pattern with a slash matches the whole path from the
  repository root (`deploy/*.yaml`).
- Patterns are shell-style globs; `*` does not cross `/`.

```
git config --global --add gitdrift.badge 'schema=*.sql'
git config --local  --add gitdrift.badge 'infra=deploy/*.yaml'
git config --global --add gitdrift.badge 'deps=tools/go.sum'
```

Values from every scope are combined with the built-in rules. A value that
does not parse is skipped. Rules are read again on every refresh (`r`/`R`).

Badges are a hint taken from file names; they do not read file contents.
