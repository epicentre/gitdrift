# 2. Isolate git processes and stop them gracefully

- Status: Accepted
- Date: 2026-10-28

## Context

A git process can ask for credentials on the terminal, hang on a network
remote, start children (`ssh`, credential helpers, upload-pack), and hold
lock files while it updates refs. Environment variables such as `GIT_DIR`
inherited from the user's shell would point every command at the wrong
repository. A TUI owns the terminal, so any prompt would corrupt the screen
or block forever.

## Decision

- Each git process starts in its own session and process group
  (`Setsid`), without a controlling terminal.
- The environment is the user's, with repository-selecting variables
  removed (`GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`,
  `GIT_OBJECT_DIRECTORY`, `GIT_ALTERNATE_OBJECT_DIRECTORIES`,
  `GIT_COMMON_DIR`, `GIT_NAMESPACE`, `GIT_PREFIX`) and with
  `GIT_TERMINAL_PROMPT=0` and `GCM_INTERACTIVE=never` added. Reads also get
  `GIT_OPTIONAL_LOCKS=0`, so a refresh never takes the index lock. The
  user's `GIT_SSH_COMMAND`, credential helper and git config are never
  overridden.
- Every call has a deadline: 30 seconds unless the caller sets one; fetch 60
  seconds, `ls-remote` 20 seconds, merge 5 minutes, `branch -D` 30 seconds,
  config writes 10 seconds.
- On cancellation or timeout the whole process group gets SIGTERM, then
  SIGKILL after a 2 second grace if it is still running. git removes its
  lock files on SIGTERM; SIGKILL would leave `*.lock` files that break the
  user's next `git fetch`.
- `WaitDelay` is 3 seconds. A git that exited 0 while a descendant (for
  example an `ssh` ControlPersist master) still holds the pipes counts as
  success with the output captured so far. `ESRCH` while signalling maps to
  `os.ErrProcessDone`.

## Consequences

- No hidden prompts and no hung UI; a failing remote shows git's message.
- A user-configured askpass helper may still open a GUI prompt; gitdrift
  does not override it.
- A child that ignores SIGTERM and does not hold git's pipes survives git's
  exit, which is the wanted behavior for `ssh` masters.
- Guarded by `internal/git/runner_unix_test.go`
  (`TestRunnerTimeoutKillsProcessGroup`,
  `TestRunnerCancelLetsGitRemoveLocks`,
  `TestRunnerCancelKillsGroupThatIgnoresTerm`) and the environment tests in
  `runner_test.go`.
