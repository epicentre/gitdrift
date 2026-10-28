// Package gittest builds throwaway git repositories and bare remotes for
// tests. Every git process runs with an isolated global configuration, so the
// developer's own git config never leaks into a test.
package gittest

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/epicentre/gitdrift/internal/git"
)

const (
	authorName  = "Test"
	authorEmail = "test@example.com"
)

// repoEnvVars select a repository and are dropped from the inherited
// environment so fixtures only ever touch their own directories.
var repoEnvVars = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_NAMESPACE", "GIT_PREFIX",
}

// leakedEnvVars are inherited variables that would change git's behavior
// despite the isolated global config.
var leakedEnvVars = []string{
	"GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_EXTERNAL_DIFF", "GIT_PAGER",
	"GIT_EDITOR", "GIT_SEQUENCE_EDITOR", "GIT_TEMPLATE_DIR", "GIT_ASKPASS",
	"SSH_ASKPASS", "GIT_SSH_COMMAND", "GIT_AUTHOR_DATE", "GIT_COMMITTER_DATE",
	"GIT_EXEC_PATH",
}

// leakedEnvPrefixes drop whole families of inherited variables.
var leakedEnvPrefixes = []string{"GIT_CONFIG_KEY_", "GIT_CONFIG_VALUE_"}

// Env is an isolated git environment rooted in a temporary directory.
type Env struct {
	tb     testing.TB
	root   string
	config string
	vars   []string
	// base supplies the inherited environment; nil means os.Environ.
	base func() []string
}

// Repo is a working repository created by an Env.
type Repo struct {
	// Path is the absolute, symlink-resolved worktree root.
	Path string

	env *Env
}

// New creates an isolated environment under tb.TempDir.
func New(tb testing.TB) *Env {
	tb.Helper()

	root, err := filepath.EvalSymlinks(tb.TempDir())
	if err != nil {
		tb.Fatalf("gittest: resolve temp dir: %v", err)
	}

	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o750); err != nil {
		tb.Fatalf("gittest: create home: %v", err)
	}

	cfg := filepath.Join(home, "gitconfig")
	content := "[user]\n\tname = " + authorName + "\n\temail = " + authorEmail + "\n" +
		"[init]\n\tdefaultBranch = main\n" +
		"[commit]\n\tgpgsign = false\n" +
		"[advice]\n\tdetachedHead = false\n" +
		"[protocol \"file\"]\n\tallow = always\n"

	if err := os.WriteFile(cfg, []byte(content), 0o600); err != nil {
		tb.Fatalf("gittest: write gitconfig: %v", err)
	}

	return &Env{
		tb:     tb,
		root:   root,
		config: cfg,
		vars: []string{
			"HOME=" + home,
			"GIT_CONFIG_GLOBAL=" + cfg,
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=" + authorName,
			"GIT_AUTHOR_EMAIL=" + authorEmail,
			"GIT_COMMITTER_NAME=" + authorName,
			"GIT_COMMITTER_EMAIL=" + authorEmail,
		},
	}
}

// Runner returns a production Runner whose environment carries the isolation
// variables, so code under test reads the isolated global config.
func (e *Env) Runner() *git.Runner {
	e.tb.Helper()

	r, err := git.NewRunner()
	if err != nil {
		e.tb.Fatalf("gittest: %v", err)
	}

	r.Env = append(slices.Clone(e.vars), "GIT_CONFIG_COUNT=0", "GIT_CONFIG_PARAMETERS=")

	return r
}

// WorkDir returns the directory that holds working clones and repositories.
func (e *Env) WorkDir() string {
	return filepath.Join(e.root, "work")
}

// GlobalConfig sets key to value in the isolated global git config.
func (e *Env) GlobalConfig(key, value string) {
	e.tb.Helper()

	e.run(e.root, "config", "--file", e.config, key, value)
}

// Remote creates a bare repository named name under remotes/ and returns its
// path.
func (e *Env) Remote(name string) string {
	e.tb.Helper()

	path := filepath.Join(e.root, "remotes", name)
	e.mkdir(filepath.Dir(path))
	e.run(e.root, "init", "--bare", "--quiet", path)

	return path
}

// Clone clones remote into work/<name>.
func (e *Env) Clone(remote, name string) *Repo {
	e.tb.Helper()

	e.mkdir(e.WorkDir())

	path := filepath.Join(e.WorkDir(), name)
	e.run(e.WorkDir(), "clone", "--quiet", remote, path)

	return &Repo{Path: path, env: e}
}

// Init creates an empty repository work/<name> with no commits.
func (e *Env) Init(name string) *Repo {
	e.tb.Helper()

	path := filepath.Join(e.WorkDir(), name)
	e.mkdir(path)
	e.run(path, "init", "--quiet")

	return &Repo{Path: path, env: e}
}

// Git runs git with args in the repository, bypassing the production
// allowlist, and returns trimmed stdout. It fails the test on error.
func (r *Repo) Git(args ...string) string {
	r.env.tb.Helper()

	return r.env.run(r.Path, args...)
}

// TryGit is Git for commands that are expected to fail, such as a merge that
// stops on a conflict. It returns trimmed stdout and the error.
func (r *Repo) TryGit(args ...string) (string, error) {
	r.env.tb.Helper()

	out, _, err := r.env.exec(r.Path, nil, args...)

	return out, err
}

// Write writes content to path (relative to the worktree), creating parents.
func (r *Repo) Write(path, content string) {
	r.env.tb.Helper()

	full := filepath.Join(r.Path, path)
	r.env.mkdir(filepath.Dir(full))

	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		r.env.tb.Fatalf("gittest: write %s: %v", path, err)
	}
}

// Commit writes the file, commits it and returns the full commit sha.
func (r *Repo) Commit(path, content, msg string) string {
	r.env.tb.Helper()

	return r.commit(path, content, msg, nil)
}

// CommitAt is Commit with fixed author and committer dates.
func (r *Repo) CommitAt(path, content, msg string, when time.Time) string {
	r.env.tb.Helper()

	return r.commit(path, content, msg, &when)
}

func (r *Repo) commit(path, content, msg string, when *time.Time) string {
	r.env.tb.Helper()

	r.Write(path, content)
	r.Git("add", "--", path)

	args := []string{"commit", "--quiet", "--allow-empty", "-m", msg}
	if when == nil {
		r.Git(args...)
	} else {
		stamp := strconv.FormatInt(when.Unix(), 10) + " +0000"
		r.env.runEnv(r.Path, []string{"GIT_AUTHOR_DATE=" + stamp, "GIT_COMMITTER_DATE=" + stamp}, args...)
	}

	return r.Git("rev-parse", "HEAD")
}

func (e *Env) mkdir(path string) {
	e.tb.Helper()

	if err := os.MkdirAll(path, 0o750); err != nil {
		e.tb.Fatalf("gittest: mkdir %s: %v", path, err)
	}
}

func (e *Env) run(dir string, args ...string) string {
	e.tb.Helper()

	return e.runEnv(dir, nil, args...)
}

// runEnv runs git and fails the test on error.
func (e *Env) runEnv(dir string, extra []string, args ...string) string {
	e.tb.Helper()

	out, stderr, err := e.exec(dir, extra, args...)
	if err != nil {
		e.tb.Fatalf("gittest: git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, stderr)
	}

	return out
}

// exec runs git directly so fixtures may use commands the production
// allowlist refuses. It never runs outside the Env's temporary directory.
func (e *Env) exec(dir string, extra []string, args ...string) (string, string, error) {
	e.tb.Helper()

	if dir != e.root && !strings.HasPrefix(dir, e.root+string(filepath.Separator)) {
		e.tb.Fatalf("gittest: refusing to run git outside %s: %s", e.root, dir)
	}

	cmd := exec.CommandContext(e.tb.Context(), "git", args...) //nolint:gosec // test fixture; args come from test code and never reach a shell
	cmd.Dir = dir

	for _, kv := range e.inherited() {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(repoEnvVars, name) && !slices.Contains(leakedEnvVars, name) &&
			!slices.ContainsFunc(leakedEnvPrefixes, func(p string) bool { return strings.HasPrefix(name, p) }) {
			cmd.Env = append(cmd.Env, kv)
		}
	}

	cmd.Env = append(cmd.Env, e.vars...)
	cmd.Env = append(cmd.Env, extra...)
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0")

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	return strings.TrimSpace(stdout.String()), stderr.String(), err
}

func (e *Env) inherited() []string {
	if e.base != nil {
		return e.base()
	}

	return os.Environ()
}
