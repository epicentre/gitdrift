# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial release: a terminal dashboard (`gdrift [dir]`) for drift across the
  git repositories in one folder.
- Four panes (Repos, Commits, Files or Only here, Diff) with a zoomed Repos
  table, and a status marker and tokens per repository.
- Confirmed, fast-forward-only pull of the reviewed upstream commit for one,
  selected or all safe repositories; it never runs `git pull` or fetches at
  pull time.
- Base branch per repository, global or detected, set with `gitdrift.base` or
  the in-app picker, and shown as `base↓N`.
- Post-pull badges (`deps`, `mig`, `env`) with custom rules through
  `gitdrift.badge`.
- Work that exists only on this machine (`⌂N`) with an only-here filter.
- Gone branch cleanup (`✂N`) with pre-checks, the list of local-only commits
  and a printed restore command.
- Fetch refuses remotes without a url or with a refspec that writes outside
  `refs/remotes/`, and never prunes tags.
