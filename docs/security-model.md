# Security model

gitdrift's promise is that it does not lose your code. This page states what
that covers: what gitdrift can change, what it trusts, what it treats as
hostile, and where its guarantees end. To report a vulnerability, see
[SECURITY.md](../SECURITY.md); a bug that can lose code is treated as one.

## What gitdrift can change

Every git process goes through one allowlist
([decision 1](architecture/0001-run-git-through-an-allowlist.md)). These are
the only commands that write, and when they run:

| Command | Runs | Changes |
| --- | --- | --- |
| `git fetch --prune --no-prune-tags --quiet --end-of-options <remote>` | at start, and on `r` / `R` | remote-tracking refs only ([decision 4](architecture/0004-guard-fetch-with-prune.md)) |
| `git merge --ff-only --no-autostash --no-overwrite-ignore --end-of-options <sha>` | after you confirm a pull | moves the current branch forward to the commit you reviewed ([decision 3](architecture/0003-pull-is-a-fast-forward-of-the-reviewed-commit.md)) |
| `git branch -D --end-of-options <name>` | after you confirm a gone branch deletion | deletes that local branch ([decision 5](architecture/0005-delete-gone-branches-after-checks.md)) |
| `git config --local gitdrift.base <value>` | after you choose a base in the picker | the repository's local config |
| `git config --local --unset gitdrift.base` | after you choose "Reset to default" | the repository's local config |

gitdrift never commits, pushes, checks out, stashes, resets or rebases. It
writes no files of its own, keeps no state between runs, and opens no network
connection itself: the only network traffic is the two git commands that talk
to your remotes, `git fetch` and `git ls-remote`, with your own credentials.
There is no telemetry.

## Trust boundaries

| Source | Treated as | Why |
| --- | --- | --- |
| The `git` binary on `PATH` | trusted | gitdrift is a front end to your git. |
| Your git config (system, global, repository), credential helpers, `ssh` configuration | trusted, never overridden | They are your choices; gitdrift must behave like your git. |
| Hooks and other programs your config runs (`core.fsmonitor`, filters, `post-merge`) | trusted | git runs them, as it would in your shell. |
| Your environment | trusted, except variables that select another repository (`GIT_DIR` and similar), which are removed | Leftover variables would point every command at the wrong repository. |
| Repository content: branch names, commit subjects, file paths, file contents, stash messages | untrusted | Other people write it ([decision 12](architecture/0012-sanitize-repository-text.md)). |
| Remote responses and error output | untrusted | The remote's operator writes them. |

**Run gitdrift only on folders whose repositories you would run `git status`
in.** gitdrift runs `git status` in every repository it lists. A repository's
own `.git/config` can make git run programs (`core.fsmonitor`, for example).
git's `safe.directory` check refuses repositories owned by another user, and
gitdrift skips those; repositories you own are trusted like any other git
command you run in them.

## Protections

- **Exact writes.** Each write form is matched argument by argument; operands
  come after `--end-of-options`; force flags, `--output` and `--ext-diff` are
  refused everywhere.
- **Re-checks before every write.** A pull merges only the reviewed commit
  and only while HEAD, the branch and the upstream are unchanged. A deletion
  re-checks the branch after asking the remote, right before deleting.
- **Fail closed.** A remote whose state is unknown blocks pulls and deletions
  of what depends on it
  ([decision 7](architecture/0007-fetch-state-fails-closed.md)).
- **No prompts, bounded time.** git never asks for input, and every call has
  a deadline ([decision 2](architecture/0002-isolate-git-processes.md)).
- **Writes are not interrupted.** Quitting waits for a running merge or
  deletion ([decision 8](architecture/0008-quit-waits-for-git-writes.md)).
- **Sanitized display.** Escape sequences, control characters and invisible
  format characters from repositories never reach the terminal.
- **Confirm what you see.** A write is confirmed only through a dialog that
  is on screen, built from the data it shows
  ([decision 13](architecture/0013-confirm-only-what-is-on-screen.md)).

## Limits

These are known and accepted; they are listed so no one relies on more than
gitdrift does.

- **Deleted branches.** `git branch -D` removes the branch and its reflog.
  The printed restore command works while the commits still exist; git's
  garbage collection can remove unreachable commits at any time. Keep the
  restore command.
- **A short window before deletion.** The last re-check and `git branch -D`
  are separate commands a few milliseconds apart; `git branch -D` takes no
  expected old value.
- **Submodules.** With `fetch.recurseSubmodules=on-demand`, git may fetch
  submodules with the same `--prune`, without gitdrift's refspec check for
  the submodules' remotes. Changes in submodules hidden by
  `submodule.<name>.ignore=all` do not count as local changes.
- **Your configuration runs.** Hooks, `core.fsmonitor` and filters behave as
  in your shell, including any side effects they have.
- **Worktree-scoped settings.** With `extensions.worktreeConfig`, a
  worktree-scoped `gitdrift.base` overrides what the picker writes.
- **git versions.** gitdrift requires git 2.30 or newer; its tests run on the
  git installed on the test machine (CI: current Ubuntu and macOS).
- **Output redirection.** gitdrift checks for a character device, not a real
  terminal; with output sent to `/dev/null` it runs and fetches with nothing
  visible.
