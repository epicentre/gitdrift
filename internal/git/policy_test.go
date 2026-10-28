package git

import (
	"errors"
	"strings"
	"testing"
)

const (
	flagFFOnly      = "--ff-only"
	flagNoOverwrite = "--no-overwrite-ignore"
	flagNoAutostash = "--no-autostash"
)

type classifyCase struct {
	name string
	kind Kind
	args []string
}

func readCases() []classifyCase {
	return []classifyCase{
		{"status", KindRead, strings.Fields("status --porcelain=v2 -z --branch --untracked-files=normal")},
		{"rev-parse", KindRead, strings.Fields("rev-parse --show-toplevel")},
		{"rev-parse verify", KindRead, strings.Fields("rev-parse --verify --quiet --absolute-git-dir --git-common-dir")},
		{"rev-list", KindRead, strings.Fields("rev-list --count HEAD..origin/main")},
		{"rev-list not", KindRead, strings.Fields("rev-list --max-count=5 --branches --not --remotes")},
		{"log", KindRead, strings.Fields("log -z --format=x HEAD")},
		{"diff numstat", KindRead, strings.Fields("diff --no-color --no-ext-diff --numstat -z a b --")},
		{"diff no-index", KindRead, strings.Fields("diff --no-color --no-index -- /dev/null f")},
		{"for-each-ref", KindRead, strings.Fields("for-each-ref refs/heads")},
		{"symbolic-ref", KindRead, strings.Fields("symbolic-ref -q refs/remotes/origin/HEAD")},
		{"merge-base", KindRead, strings.Fields("merge-base HEAD x")},
		{"ls-remote", KindRead, strings.Fields("ls-remote --exit-code --end-of-options origin refs/heads/x")},
		{"ls-files", KindRead, strings.Fields("ls-files --others --exclude-standard -z")},
		{"hash-object", KindRead, strings.Fields("hash-object -t tree /dev/null")},
		{"remote", KindRead, []string{"remote"}},
		{"worktree list", KindRead, strings.Fields("worktree list --porcelain")},
		{"stash list", KindRead, strings.Fields("stash list --format=%gd")},
		{"config get", KindRead, strings.Fields("config --show-scope --get gitdrift.base")},
		{"config get-all", KindRead, strings.Fields("config --get-all gitdrift.badge")},
		{"config list", KindRead, strings.Fields("config --list --local -z")},
		{"version", KindRead, []string{"version"}},
		{"ls-remote flags", KindRead, strings.Fields("ls-remote --heads --refs --sort=refname -q origin")},
		{"ls-remote operand after end", KindRead, strings.Fields("ls-remote --end-of-options origin -weird")},
		{"symbolic-ref short", KindRead, strings.Fields("symbolic-ref --short -q HEAD")},
	}
}

func writeCases(sha40, sha64 string) []classifyCase {
	return []classifyCase{
		{"fetch", KindWrite, strings.Fields("fetch --prune --no-prune-tags --quiet --end-of-options origin")},
		{"merge sha40", KindWrite, []string{cmdMerge, flagFFOnly, flagNoAutostash, flagNoOverwrite, flagEndOfOptions, sha40}},
		{"merge sha64", KindWrite, []string{cmdMerge, flagFFOnly, flagNoAutostash, flagNoOverwrite, flagEndOfOptions, sha64}},
		{"branch -D", KindWrite, strings.Fields("branch -D --end-of-options feature/x")},
		{"config set", KindWrite, strings.Fields("config --local gitdrift.base develop")},
		{"config unset", KindWrite, strings.Fields("config --local --unset gitdrift.base")},
	}
}

func refusedCases(sha40 string) []classifyCase {
	return []classifyCase{
		{"empty", KindUnknown, nil},
		{"push", KindUnknown, strings.Fields("push origin main")},
		{"reset hard", KindUnknown, strings.Fields("reset --hard")},
		{"stash bare", KindUnknown, []string{"stash"}},
		{"stash push", KindUnknown, strings.Fields("stash push")},
		{"stash pop", KindUnknown, strings.Fields("stash pop")},
		{"checkout", KindUnknown, strings.Fields("checkout main")},
		{"switch", KindUnknown, strings.Fields("switch main")},
		{"clean", KindUnknown, strings.Fields("clean -fd")},
		{"commit", KindUnknown, strings.Fields("commit -m x")},
		{"rebase", KindUnknown, strings.Fields("rebase main")},
		{"pull", KindUnknown, []string{"pull"}},
		{"fetch plain", KindUnknown, strings.Fields("fetch origin")},
		{"fetch force", KindUnknown, strings.Fields("fetch --prune --no-prune-tags --quiet --end-of-options --force origin")},
		{"fetch dash remote", KindUnknown, strings.Fields("fetch --prune --no-prune-tags --quiet --end-of-options -x")},
		{"fetch old form", KindUnknown, strings.Fields("fetch --prune --quiet --end-of-options origin")},
		{"merge old form", KindUnknown, []string{cmdMerge, flagFFOnly, flagNoOverwrite, flagEndOfOptions, sha40}},
		{"merge plain", KindUnknown, strings.Fields("merge origin/main")},
		{"merge missing flag", KindUnknown, strings.Fields("merge --ff-only --end-of-options abc")},
		{"merge not sha", KindUnknown, strings.Fields("merge --ff-only --no-autostash --no-overwrite-ignore --end-of-options main")},
		{"merge upper sha", KindUnknown, []string{cmdMerge, flagFFOnly, flagNoAutostash, flagNoOverwrite, flagEndOfOptions, strings.ToUpper(sha40)}},
		{"branch -d", KindUnknown, strings.Fields("branch -d x")},
		{"branch dash name", KindUnknown, strings.Fields("branch -D --end-of-options -x")},
		{"config global", KindUnknown, strings.Fields("config --global gitdrift.base x")},
		{"config other key", KindUnknown, strings.Fields("config --local user.name x")},
		{"config bare", KindUnknown, strings.Fields("config gitdrift.base x")},
		{"config dash value", KindUnknown, strings.Fields("config --local gitdrift.base -x")},
		{"config unset other", KindUnknown, strings.Fields("config --local --unset user.name")},
		{"config get plus unset", KindUnknown, strings.Fields("config --get --unset gitdrift.base")},
		{"config add", KindUnknown, strings.Fields("config --get --add gitdrift.base x")},
		{"hash-object -w", KindUnknown, strings.Fields("hash-object -w f")},
		{"hash-object cluster", KindUnknown, strings.Fields("hash-object -tw blob f")},
		{"worktree add", KindUnknown, strings.Fields("worktree add ../x")},
		{"remote add", KindUnknown, strings.Fields("remote add x y")},
		{"log force", KindUnknown, strings.Fields("log --force")},
		{"log hard", KindUnknown, strings.Fields("log --hard")},
		{"log autostash", KindUnknown, strings.Fields("log --autostash")},
		{"log force=", KindUnknown, strings.Fields("log --force=1")},
		{"log output=", KindUnknown, strings.Fields("log --output=/tmp/x HEAD")},
		{"log output two tokens", KindUnknown, strings.Fields("log --output /tmp/x HEAD")},
		{"diff output", KindUnknown, strings.Fields("diff --output=/tmp/x")},
		{"show output", KindUnknown, strings.Fields("show --output=/tmp/x")},
		{"rev-list output", KindUnknown, strings.Fields("rev-list --output=/tmp/x HEAD")},
		{"diff-tree output", KindUnknown, strings.Fields("diff-tree --output=/tmp/x a b")},
		{"stash list output", KindUnknown, strings.Fields("stash list --output=/tmp/x")},
		{"diff ext-diff", KindUnknown, strings.Fields("diff --ext-diff a b")},
		{"ls-remote upload-pack", KindUnknown, strings.Fields("ls-remote --upload-pack=cmd origin")},
		{"ls-remote exec", KindUnknown, strings.Fields("ls-remote --exec=cmd origin")},
		{"ls-remote abbreviation", KindUnknown, strings.Fields("ls-remote --upl=cmd origin")},
		{"ls-remote -u", KindUnknown, strings.Fields("ls-remote -u cmd origin")},
		{"ls-remote attached -u", KindUnknown, strings.Fields("ls-remote -ucmd origin")},
		{"symbolic-ref write", KindUnknown, strings.Fields("symbolic-ref HEAD refs/heads/other")},
		{"symbolic-ref -d", KindUnknown, strings.Fields("symbolic-ref -d refs/x")},
		{"symbolic-ref --delete", KindUnknown, strings.Fields("symbolic-ref --delete refs/x")},
		{"symbolic-ref --del", KindUnknown, strings.Fields("symbolic-ref --del refs/x")},
		{"symbolic-ref none", KindUnknown, []string{"symbolic-ref", "-q"}},
		{"config edit", KindUnknown, strings.Fields("config --edit")},
		{"config file", KindUnknown, strings.Fields("config --get --file x gitdrift.base")},
		{"-c", KindUnknown, strings.Fields("-c k=v status")},
		{"remote -v", KindUnknown, strings.Fields("remote -v")},
		{"worktree list positional", KindUnknown, strings.Fields("worktree list x")},
		{"stash list positional", KindUnknown, strings.Fields("stash list stash@{0}")},
		{"fetch extra", KindUnknown, strings.Fields("fetch --prune --no-prune-tags --quiet --end-of-options origin x")},
		{"merge extra", KindUnknown, []string{cmdMerge, flagFFOnly, flagNoAutostash, flagNoOverwrite, flagEndOfOptions, sha40, "x"}},
		{"branch extra", KindUnknown, strings.Fields("branch -D --end-of-options x y")},
		{"config set extra", KindUnknown, strings.Fields("config --local gitdrift.base a b")},
		{"config unset extra", KindUnknown, strings.Fields("config --local --unset gitdrift.base x")},
		{"branch empty name", KindUnknown, []string{"branch", "-D", flagEndOfOptions, ""}},
		{"global option", KindUnknown, strings.Fields("-C /tmp status")},
	}
}

func TestClassify(t *testing.T) {
	t.Parallel()

	sha40 := strings.Repeat("a1", 20)
	sha64 := strings.Repeat("0f", 32)

	tests := readCases()
	tests = append(tests, writeCases(sha40, sha64)...)
	tests = append(tests, refusedCases(sha40)...)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			kind, err := Classify(tt.args)
			if kind != tt.kind {
				t.Fatalf("Classify(%q) kind = %d, want %d (err %v)", tt.args, kind, tt.kind, err)
			}

			if tt.kind == KindUnknown {
				if !errors.Is(err, ErrNotAllowed) {
					t.Fatalf("err = %v, want ErrNotAllowed", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestClassifyErrorNamesSubcommand(t *testing.T) {
	t.Parallel()

	_, err := Classify([]string{"push", "origin", "main"})
	if err == nil || err.Error() != "git: command not allowed: push" {
		t.Fatalf("err = %v", err)
	}
}
