# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Nested discovery: repositories are found in the folder and up to five
  directory levels below it; the search never enters repositories, hidden
  directories, `node_modules` or `vendor`.
- `--depth N` sets how many directory levels are searched.
- A FOLDER column shows where a repository sits below the workspace.
- Folder tabs in the Repos pane, with counts.
- `←`/`→` and `h`/`l` switch the tabs of the focused pane, besides `[` and
  `]`, which need AltGr on some keyboard layouts.
- A `/` filter for the Repos pane by name or folder; `P` and `R` act on the
  repositories shown.
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
- Repos pane with aligned, full-width columns and a header row naming them.
- The Repos pane spans the top of the screen, with Commits, Files and Diff
  below it, so that every column and whole folder names fit; the focused
  pane has a thick border, every pane title names its pane, and the folder
  tabs sit on their own line with the active one as a filled box.
- Architecture overview and decision records in `docs/architecture/`.
