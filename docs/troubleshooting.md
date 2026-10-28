# Troubleshooting

Find the message you see, then the cause and what to do. Messages appear on
the status line (the line above the key hints), on a row after a pull, or in
the deletion result window.

## gdrift does not start

| Message | Cause | What to do |
| --- | --- | --- |
| `gdrift: an interactive terminal is required` | Output is redirected to a file or pipe. | Run gdrift directly in a terminal. |
| `gdrift: git 2.30 or newer is required (found X.Y.Z)` | The `git` on `PATH` is too old. | Install a newer git, or put it first on `PATH`. |
| `gdrift: no git repositories found in <dir>` | Neither `<dir>` nor any directory below it, down to the depth limit (5 by default), is the top of a git worktree. | Run it in the folder that holds your repositories, or pass that folder: `gdrift ~/code/work`. Raise the limit with `--depth` if they are deeper. |
| `gdrift: --depth must be 1 or more` | `--depth` was 0 or negative. | Pass a positive number, for example `--depth 3`. |
| `terminal too small (need 60×15)` | The window is below the minimum size. | Enlarge the window. Pull and delete cannot be confirmed until it fits. |

## The Repos pane shows no rows

| Message | Cause | What to do |
| --- | --- | --- |
| `no matching repositories` | The folder tab, the `/` filter and `f` together leave no repository on screen. | Press `esc` in the Repos pane to clear the filter, press `f` to turn the only-here filter off, or switch the folder tab with `←` and `→`. |

## A repository is missing from the list

gitdrift looks at the folder itself and at the directories below it, up to
five levels deep. A repository is left out when:

- it is deeper than the depth limit. Start gdrift closer to it, or raise the
  limit: `gdrift --depth 8`;
- it is inside a hidden directory, `node_modules` or `vendor`, which are not
  searched;
- it is inside another repository's work tree: the search stops at the first
  work tree git opens, so neither a nested independent repository nor a linked
  worktree placed inside a work tree is listed. Start gdrift inside the outer
  repository to see them;
- you started gdrift in a different folder than you meant to. In your home
  directory the search walks down to the depth limit, and on macOS this can
  show permission prompts for Documents, Desktop or Downloads. Run gdrift in
  your projects folder, or lower `--depth`;
- git refuses it, for example with "detected dubious ownership" because the
  directory belongs to another user. The search then goes on inside it, so
  repositories below it can still be listed. Run `git status` in it to see git's
  message; `git config --global --add safe.directory <path>` accepts it if
  you trust it;
- the directory cannot be read (permissions);
- it is a subdirectory of a worktree rather than its top;
- it is a symbolic link to a repository that is already listed.

Linked worktrees inside the folder get their own rows. So do the worktrees of
a bare clone kept beside them, with `proj/.git` a file that points at
`proj/.bare` and the worktrees in `proj/main` and `proj/feature`: `proj` has no
work tree of its own, so the search goes on inside it. A repository the
search found can also be hidden by the folder tab, the `/` filter or `f`:
check the tab counts in the Repos title and press `esc` to clear the filter.

## Fetch problems

The status line shows `fetch failed: <remote> (<git's message>)` for the
repository under the cursor.

| Message in parentheses | Cause | What to do |
| --- | --- | --- |
| `fetch refused: remote "<r>" refspec "…" writes outside refs/remotes/` | The remote's fetch refspec targets local branches or tags (a mirror setup). Fetching with `--prune` could delete local refs, so gitdrift does not fetch it. | Use git directly for this remote, or change `remote.<r>.fetch` to `+refs/heads/*:refs/remotes/<r>/*`. |
| `fetch refused: remote "<r>": remote has no url in git config` | The remote is defined without a `url`, for example only through a legacy `.git/remotes/<r>` file. | Add the url: `git remote set-url <r> <url>`. |
| `timed out` | The remote did not answer within 60 seconds. | Check the network or VPN, then press `r`. |
| `fatal: Authentication failed …`, `Permission denied (publickey)` and similar | git could not authenticate. gitdrift never prompts for credentials. | Run `git fetch` in that repository once to fix the credentials, then press `r`. |
| `fatal: … does not appear to be a git repository` | The remote's url is wrong or unreachable. | Check `git remote -v`. |

`remote not fetched yet: <remote>` means the fetch has not run, for example
while gitdrift is still starting or when the repository could not be read.
Press `r` to refresh it.

## Why a repository cannot be pulled

`space` shows `cannot select: <reason>`; a pull that was attempted shows
`skipped: <reason>` on its row.

| Reason | Meaning | What to do |
| --- | --- | --- |
| `local changes` | Tracked files have uncommitted changes, staged or not. | Commit or stash them in git, then press `r`. |
| `diverged` | The branch has commits the upstream does not, and the upstream has commits the branch does not. | Merge or rebase in git; gitdrift only fast-forwards. |
| `no upstream` | The branch does not track a remote branch. | `git branch --set-upstream-to=<remote>/<branch>`. |
| `upstream gone` | The upstream branch was deleted on the remote. | Switch to another branch in git; the old one appears in the Branches tab. |
| `detached HEAD` | No branch is checked out. | Check out a branch in git. |
| `merge in progress` (also rebase, cherry-pick, revert, bisect) | git is in the middle of an operation. | Finish or abort it in git. |
| `fetch failed: <remote>`, `remote not fetched yet: <remote>` | The upstream's remote is not current. | See [Fetch problems](#fetch-problems). |
| `inspection failed` | gitdrift could not read the repository; the status line shows the error. | Run `git status` there to see what git reports. |
| `repository changed since review, press r` | HEAD or the checked-out branch changed after the screen was drawn. | Press `r` and review again. |
| `upstream changed since review, press r` | The upstream moved, for example after a fetch in another terminal. | Press `r` and review again. |
| `still fetching` | The row has not been read yet. | Wait a moment. |
| `pull in progress` | This row is being pulled. | Wait for the result. |

`nothing safe to pull` means `P` found no repository without a blocker.

`pull failed: <git's line>` means git refused the merge. The most common
case is `error: Your local changes to the following files would be
overwritten by merge: …` or `error: The following untracked working tree
files would be overwritten by merge: …`: an untracked or ignored file in your
worktree has the same path as a file the pull adds. Move it away, then pull
again. Nothing was changed.

## Gone branch deletion

In the Branches tab:

| Message | Meaning |
| --- | --- |
| `current branch cannot be deleted` | It is checked out in this worktree. Switch branches in git first. |
| `branch is checked out in <worktree>` | Another worktree uses it. |
| `fetch failed: <remote>`, `remote not fetched yet: <remote>` | The remote's state is not current, so gitdrift cannot tell that the branch is really gone. Press `r`. |
| `open the Branches tab to delete gone branches` | `d` was pressed outside the Branches tab. |

The result window lists every branch as `deleted`, `skipped` or `failed`:

| Skip reason | Meaning |
| --- | --- |
| `branch no longer exists` | It was deleted in the meantime. |
| `branch changed since review` | A commit was added after the dialog was shown. |
| `branch is checked out` | It was checked out in the meantime. |
| `upstream exists again` | The remote branch came back after a fetch. |
| `branch still exists on remote` | `git ls-remote` found the branch on the remote, so it is not gone. |
| `could not verify remote: <reason>` | The remote could not be asked; nothing was deleted. |
| `branch has no remote-tracking upstream` | The branch's upstream is local or unset. |

## Restoring a deleted branch

The result window prints a command for every deleted branch, for example
`git branch feature/cash-rounding 3c8e590…`. Run it in the repository to
bring the branch back at the same commit. Unusual names are quoted, so the
command can be pasted as is.

If you closed the window without copying the command, the commits are
unreachable but usually still present until git's garbage collection removes
them:

```
git reflog --date=relative            # commits made in this worktree
git fsck --unreachable --no-reflogs | grep commit
git branch <name> <sha>               # once you found the tip
```

Do not rely on this; garbage collection can run at any time, including
during a fetch.

## Quitting

`waiting for git to finish…` after quitting means a merge or a branch
deletion was still running. gitdrift waits for it instead of interrupting it,
so the repository is never left half-updated.

## Lock files

If git reports `Unable to create '…/index.lock': File exists` or
`cannot lock ref …`, a git process was interrupted somewhere. First make sure
no git command is still running in that repository; then remove the
`.lock` file that git names.
