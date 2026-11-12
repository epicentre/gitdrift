package ui_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/epicentre/gitdrift/internal/gittest"
	"github.com/epicentre/gitdrift/internal/ui"
	"github.com/epicentre/gitdrift/internal/workspace"
)

const (
	// e2eWait bounds each wait of the end-to-end test: the first refresh
	// fetches every remote, which is slow under the race detector.
	e2eWait = 20 * time.Second
	// e2eCheck is how often a wait looks at the screen again.
	e2eCheck = 200 * time.Millisecond
)

// behindClone creates work/<name> with two commits on its remote, of which
// it has only the first: the clone is behind its upstream by one commit,
// which writes the file incoming.
func behindClone(t *testing.T, env *gittest.Env, name, incoming string) *gittest.Repo {
	t.Helper()

	clone := env.Clone(env.Remote(name+".git"), name)
	clone.Commit("README.md", "hello\n", "initial")
	clone.Commit(incoming, "one\n", "teammate change")
	clone.Git("push", "--quiet", "-u", "origin", "main")
	// The second commit now exists only on the remote and origin/main.
	clone.Git("reset", "--quiet", "--hard", "HEAD~1")

	return clone
}

// waitForScreen waits until the program's output, stripped of escape
// sequences, contains want. The renderer writes only the cells that changed
// since the last frame, so a line that changed in place never shows up whole;
// each check that fails asks for a full repaint, which the next check reads.
func waitForScreen(t *testing.T, tm *teatest.TestModel, want string) {
	t.Helper()

	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		if strings.Contains(ansi.Strip(string(out)), want) {
			return true
		}

		tm.Send(tea.ClearScreen())

		return false
	}, teatest.WithDuration(e2eWait), teatest.WithCheckInterval(e2eCheck))
}

// waitForLoaded waits until the last full repaint of the screen shows want
// in its header and no progress: every read and fetch is done. Other frames
// redraw only the cells that changed, so their text says nothing whole; each
// check that fails asks for a full repaint, which starts by erasing the
// screen, and the next check reads its first line.
func waitForLoaded(t *testing.T, tm *teatest.TestModel, want string) {
	t.Helper()

	const eraseScreen = "\x1b[2J"

	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		if i := bytes.LastIndex(out, []byte(eraseScreen)); i >= 0 {
			frame := ansi.Strip(string(out[i+len(eraseScreen):]))

			header, _, whole := strings.Cut(frame, "\n")
			if whole && strings.Contains(header, want) &&
				!strings.Contains(header, "reading") && !strings.Contains(header, "fetching") {
				return true
			}
		}

		tm.Send(tea.ClearScreen())

		return false
	}, teatest.WithDuration(e2eWait), teatest.WithCheckInterval(e2eCheck))
}

// TestEndToEndPull pulls every safe repository with P: api is clean, web has
// a local change to a file the pull does not touch and keeps it, and docs
// has a local change to the incoming file and is left alone.
func TestEndToEndPull(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	api := behindClone(t, env, "api", "CHANGELOG.md")
	web := behindClone(t, env, "web", "CHANGELOG.md")
	web.Write("README.md", "local edit\n")

	docs := behindClone(t, env, "docs", "README.md")
	docs.Write("README.md", "local edit\n")
	docsHead := docs.Git("rev-parse", "HEAD")

	root := env.WorkDir()

	groups, err := workspace.Discover(t.Context(), env.Runner(), root, workspace.DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}

	ws := workspace.New(env.Runner(), groups, workspace.DefaultOptions())
	// t.Context is cancelled before cleanups run; Close then waits for the
	// git processes the program started, as gdrift does on quit.
	t.Cleanup(ws.Close)

	tm := teatest.NewTestModel(t, ui.New(t.Context(), ws, root), teatest.WithInitialTermSize(120, 40))

	// P offers only repositories whose fetch is done.
	waitForLoaded(t, tm, "3 repos · 3 behind · 1 need attention")
	tm.Type("P")
	tm.Type("y")
	// Each pull re-reads its repository; only docs is still behind.
	waitForScreen(t, tm, "3 repos · 1 behind · 1 need attention")

	if err := tm.Quit(); err != nil {
		t.Fatal(err)
	}

	tm.WaitFinished(t, teatest.WithFinalTimeout(e2eWait))

	for _, r := range []*gittest.Repo{api, web} {
		if head, upstream := r.Git("rev-parse", "HEAD"), r.Git("rev-parse", "@{upstream}"); head != upstream {
			t.Errorf("%s HEAD = %s, want its upstream %s", r.Path, head, upstream)
		}
	}

	if status := web.Git("status", "--porcelain"); status != "M README.md" {
		t.Errorf("web status = %q, want its local change kept", status)
	}

	if head := docs.Git("rev-parse", "HEAD"); head != docsHead {
		t.Errorf("docs HEAD = %s, want it unchanged at %s", head, docsHead)
	}

	if status := docs.Git("status", "--porcelain"); status != "M README.md" {
		t.Errorf("docs status = %q, want its local change kept", status)
	}
}
