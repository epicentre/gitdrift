package repo_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/epicentre/gitdrift/internal/gittest"
	"github.com/epicentre/gitdrift/internal/repo"
)

func locate(t *testing.T, env *gittest.Env, r *gittest.Repo) repo.Location {
	t.Helper()

	loc, err := repo.Locate(t.Context(), env.Runner(), r.Path)
	if err != nil {
		t.Fatal(err)
	}

	return loc
}

func paths(files []repo.FileChange) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}

	slices.Sort(out)

	return out
}

// incomingWithMerge pushes a merged branch (one commit, two files) plus one direct commit
// to origin/main and fetches it into app. It returns the merge commit sha.
func incomingWithMerge(t *testing.T, env *gittest.Env) (*gittest.Repo, string) {
	t.Helper()

	remote, app := pushedMain(t, env)

	mate := env.Clone(remote, "teammate")
	mate.Git("switch", "--quiet", "-c", "feature/side")
	mate.Write("side2.txt", "2")
	mate.Git("add", "--", "side2.txt")
	mate.Commit("side1.txt", "1", "side work")
	mate.Git("switch", "--quiet", "main")
	mate.Git("merge", "--quiet", "--no-ff", "-m", "merge side", "feature/side")
	merge := mate.Git("rev-parse", "HEAD")
	mate.Commit("direct.txt", "d", "direct change")
	mate.Git("push", "--quiet", "origin", "main")
	app.Git("fetch", "--quiet", "origin")

	return app, merge
}

func TestRangeCommitsIncoming(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	app, _ := incomingWithMerge(t, env)
	loc := locate(t, env, app)

	commits, total, err := repo.RangeCommits(t.Context(), env.Runner(), loc, "HEAD", originMain, repo.MaxCommits)
	if err != nil {
		t.Fatal(err)
	}

	if len(commits) != 3 || total != 3 {
		// side work, merge, direct
		t.Fatalf("got %d commits, total %d; want 3 and 3", len(commits), total)
	}

	if commits[0].Subject != "direct change" || commits[1].Subject != "merge side" {
		t.Errorf("order = %q, %q; want newest first", commits[0].Subject, commits[1].Subject)
	}

	merges := 0

	for _, c := range commits {
		if c.Merge {
			merges++
		}
	}

	if merges != 1 {
		t.Errorf("merges = %d, want 1", merges)
	}
}

// TestRangeCommitsAuthorFromMailmap shows the author as the repository's
// .mailmap names them, so that one person committing with several names
// reads as one.
func TestRangeCommitsAuthorFromMailmap(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	app, _ := incomingWithMerge(t, env)
	app.Write(".mailmap", "Jane Doe <test@example.com>\n")
	loc := locate(t, env, app)

	commits, _, err := repo.RangeCommits(t.Context(), env.Runner(), loc, "HEAD", originMain, repo.MaxCommits)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range commits {
		if c.Author != "Jane Doe" {
			t.Errorf("author of %q = %q, want the .mailmap name %q", c.Subject, c.Author, "Jane Doe")
		}
	}
}

func TestRangeCommitsLimit(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	remote, app := pushedMain(t, env)
	mate := env.Clone(remote, "teammate")

	for i := range 7 {
		mate.Commit(fmt.Sprintf("f%d.txt", i), "x", fmt.Sprintf("change %d", i))
	}

	mate.Git("push", "--quiet", "origin", "main")
	app.Git("fetch", "--quiet", "origin")

	commits, total, err := repo.RangeCommits(t.Context(), env.Runner(), locate(t, env, app), "HEAD", originMain, 5)
	if err != nil {
		t.Fatal(err)
	}

	if len(commits) != 5 || total != 7 {
		t.Fatalf("got %d commits, total %d; want 5 and 7", len(commits), total)
	}
}

func TestFilesMergeBaseAndCommit(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	app, merge := incomingWithMerge(t, env)
	loc := locate(t, env, app)
	g := env.Runner()

	files, err := repo.Files(t.Context(), g, loc, repo.DiffSpec{Kind: repo.DiffMergeBase, From: "HEAD", To: originMain})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"direct.txt", "side1.txt", "side2.txt"}
	if diff := cmp.Diff(want, paths(files)); diff != "" {
		t.Errorf("merge-base files mismatch (-want +got):\n%s", diff)
	}

	spec, err := repo.CommitSpec(t.Context(), g, loc, merge)
	if err != nil {
		t.Fatal(err)
	}

	files, err = repo.Files(t.Context(), g, loc, spec)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]string{"side1.txt", "side2.txt"}, paths(files)); diff != "" {
		t.Errorf("merge commit files mismatch (-want +got):\n%s", diff)
	}

	for _, f := range files {
		if f.Status != "A" || f.Added != 1 || f.Deleted != 0 {
			t.Errorf("file = %+v, want status A with 1 added", f)
		}
	}
}

func TestFilesSpacesUnicodeBinary(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	remote, app := pushedMain(t, env)
	mate := env.Clone(remote, "teammate")
	mate.Commit("dir with space/café menu.txt", "latte\n", "menu")
	mate.Commit("logo.png", "\x00\x01\x02"+strings.Repeat("\xff", 1021), "logo")
	mate.Git("push", "--quiet", "origin", "main")
	app.Git("fetch", "--quiet", "origin")

	files, err := repo.Files(t.Context(), env.Runner(), locate(t, env, app),
		repo.DiffSpec{Kind: repo.DiffMergeBase, From: "HEAD", To: originMain})
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]string{"dir with space/café menu.txt", "logo.png"}, paths(files)); diff != "" {
		t.Fatalf("paths mismatch (-want +got):\n%s", diff)
	}

	for _, f := range files {
		if f.Binary != (f.Path == "logo.png") {
			t.Errorf("%s: Binary = %v", f.Path, f.Binary)
		}
	}
}

func TestFilesRename(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	app := env.Init("app")
	app.Commit("old name.txt", strings.Repeat("line\n", 20), "base")
	app.Git("mv", "old name.txt", "new name.txt")
	app.Git("commit", "--quiet", "-m", "rename")

	g := env.Runner()
	loc := locate(t, env, app)

	spec, err := repo.CommitSpec(t.Context(), g, loc, "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	files, err := repo.Files(t.Context(), g, loc, spec)
	if err != nil {
		t.Fatal(err)
	}

	want := []repo.FileChange{{Path: "new name.txt", OldPath: "old name.txt", Status: "R"}}
	if diff := cmp.Diff(want, files); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

// changedLines lists the added and deleted lines of a diff, without the file
// headers.
func changedLines(diff string) []string {
	var out []string

	for line := range strings.SplitSeq(diff, "\n") {
		if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- ") {
			continue
		}

		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			out = append(out, line)
		}
	}

	return out
}

func TestFileDiffRename(t *testing.T) {
	t.Parallel()

	var lines strings.Builder
	for i := range 20 {
		fmt.Fprintf(&lines, "line %d\n", i+1)
	}

	renamed := strings.Replace(lines.String(), "line 7\n", "changed 7\n", 1)

	tests := []struct {
		name string
		// commit commits the rename when set; else it stays staged.
		commit bool
	}{
		{"commit", true},
		{"worktree", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			env := gittest.New(t)
			app := env.Init("app")
			app.Commit("old name.txt", lines.String(), "base")
			app.Git("mv", "old name.txt", "new name.txt")
			app.Write("new name.txt", renamed)
			app.Git("add", "--", "new name.txt")

			g := env.Runner()
			loc := locate(t, env, app)
			spec := repo.DiffSpec{Kind: repo.DiffWorktree}

			if tt.commit {
				app.Git("commit", "--quiet", "-m", "rename")

				var err error
				if spec, err = repo.CommitSpec(t.Context(), g, loc, "HEAD"); err != nil {
					t.Fatal(err)
				}
			}

			text, _, err := repo.FileDiff(t.Context(), g, loc, spec, "new name.txt", "old name.txt", repo.MaxDiffBytes)
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(text, "rename from old name.txt") || !strings.Contains(text, "rename to new name.txt") {
				t.Errorf("diff does not show the rename:\n%s", text)
			}

			if diff := cmp.Diff([]string{"-line 7", "+changed 7"}, changedLines(text)); diff != "" {
				t.Errorf("changed lines mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFilesWorktree(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	_, app := pushedMain(t, env)
	app.Write("base.txt", "changed")
	app.Write("new file.txt", "hello\n")

	g := env.Runner()
	loc := locate(t, env, app)
	spec := repo.DiffSpec{Kind: repo.DiffWorktree}

	files, err := repo.Files(t.Context(), g, loc, spec)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	for _, f := range files {
		got[f.Path] = f.Status
	}

	if diff := cmp.Diff(map[string]string{"base.txt": "M", "new file.txt": "?"}, got); diff != "" {
		t.Fatalf("statuses mismatch (-want +got):\n%s", diff)
	}

	text, truncated, err := repo.FileDiff(t.Context(), g, loc, spec, "new file.txt", "", repo.MaxDiffBytes)
	if err != nil {
		t.Fatal(err)
	}

	if truncated || !strings.Contains(text, "+hello") {
		t.Errorf("untracked diff = %q (truncated %v), want +hello", text, truncated)
	}

	text, _, err = repo.FileDiff(t.Context(), g, loc, spec, "", "", repo.MaxDiffBytes)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(text, "+changed") || !strings.Contains(text, "+hello") {
		t.Errorf("whole diff = %q, want tracked and untracked changes", text)
	}
}

func TestFilesWorktreeUnborn(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	app := env.Init("app")
	app.Write("a.txt", "a\n")

	files, err := repo.Files(t.Context(), env.Runner(), locate(t, env, app), repo.DiffSpec{Kind: repo.DiffWorktree})
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff([]repo.FileChange{{Path: "a.txt", Status: "?"}}, files); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestFileDiffTruncated(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	app := env.Init("app")
	app.Commit("big.txt", "start\n", "base")
	app.Commit("big.txt", strings.Repeat("0123456789abcdef\n", 256), "grow")

	g := env.Runner()
	loc := locate(t, env, app)

	spec, err := repo.CommitSpec(t.Context(), g, loc, "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	text, truncated, err := repo.FileDiff(t.Context(), g, loc, spec, "big.txt", "", 1024)
	if err != nil {
		t.Fatal(err)
	}

	if !truncated || len(text) > 1024 || !strings.HasSuffix(text, "\n") {
		t.Errorf("truncated = %v, len = %d, want true, <= 1024, line boundary", truncated, len(text))
	}

	full, truncated, err := repo.FileDiff(t.Context(), g, loc, spec, "big.txt", "", repo.MaxDiffBytes)
	if err != nil || truncated || len(full) <= 1024 {
		t.Errorf("full diff: len %d, truncated %v, err %v", len(full), truncated, err)
	}
}

func TestCommitSpecRoot(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	app := env.Init("app")
	sha := app.Commit("a.txt", "a\n", "root")

	spec, err := repo.CommitSpec(t.Context(), env.Runner(), locate(t, env, app), sha)
	if err != nil {
		t.Fatal(err)
	}

	want := repo.DiffSpec{Kind: repo.DiffRange, From: "4b825dc642cb6eb9a060e54bf8d69288fbee4904", To: sha}
	if diff := cmp.Diff(want, spec); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestStashSpec(t *testing.T) {
	t.Parallel()

	got := repo.StashSpec(repo.Stash{SHA: "abc"})
	want := repo.DiffSpec{Kind: repo.DiffRange, From: "abc^1", To: "abc"}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestQueriesIgnoreUserDiffConfig(t *testing.T) {
	t.Parallel()

	query := func(poison bool) ([]repo.FileChange, string) {
		env := gittest.New(t)
		app := env.Init("app")
		app.Commit("café.txt", "one\n", "base")
		app.Commit("plain.txt", "one\n", "plain base")
		app.Write("café.txt", "two\n")
		app.Git("add", "--", "café.txt")
		app.Commit("plain.txt", "two\n", "change")
		sha := app.Git("rev-parse", "HEAD")

		if poison {
			app.Git("config", "--local", "color.ui", "always")
			app.Git("config", "--local", "diff.external", "false")
			app.Git("config", "--local", "diff.noprefix", "true")
			app.Git("config", "--local", "diff.renames", "false")
			app.Git("config", "--local", "core.quotepath", "true")
		}

		g := env.Runner()
		loc := locate(t, env, app)

		spec, err := repo.CommitSpec(t.Context(), g, loc, sha)
		if err != nil {
			t.Fatal(err)
		}

		files, err := repo.Files(t.Context(), g, loc, spec)
		if err != nil {
			t.Fatal(err)
		}

		text, _, err := repo.FileDiff(t.Context(), g, loc, spec, "plain.txt", "", repo.MaxDiffBytes)
		if err != nil {
			t.Fatal(err)
		}

		return files, text
	}

	cleanFiles, cleanText := query(false)
	files, text := query(true)

	if diff := cmp.Diff(cleanFiles, files); diff != "" {
		t.Errorf("files differ under user config (-clean +poisoned):\n%s", diff)
	}

	if strings.Contains(text, "\x1b") || !strings.Contains(text, "--- a/") || !strings.Contains(text, "+++ b/") {
		t.Errorf("diff under user config = %q, want plain with a/ b/ prefixes", text)
	}

	// Blob ids and commit ids differ between environments only through
	// timestamps; the hunk body must match.
	if !strings.Contains(cleanText, "+two") || !strings.Contains(text, "+two") {
		t.Errorf("diffs lack the change: %q / %q", cleanText, text)
	}
}
