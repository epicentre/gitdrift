package ui_test

import (
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
// it has only the first: the clone is behind its upstream by one.
func behindClone(t *testing.T, env *gittest.Env, name string) *gittest.Repo {
	t.Helper()

	clone := env.Clone(env.Remote(name+".git"), name)
	clone.Commit("README.md", "hello\n", "initial")
	clone.Commit("CHANGELOG.md", "one\n", "teammate change")
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

func TestEndToEndPull(t *testing.T) {
	t.Parallel()

	env := gittest.New(t)
	api := behindClone(t, env, "api")
	web := behindClone(t, env, "web")
	web.Write("README.md", "local edit\n")
	webHead := web.Git("rev-parse", "HEAD")

	root := env.WorkDir()

	groups, err := workspace.Discover(t.Context(), env.Runner(), root)
	if err != nil {
		t.Fatal(err)
	}

	ws := workspace.New(env.Runner(), groups, workspace.DefaultOptions())
	// t.Context is cancelled before cleanups run; Close then waits for the
	// git processes the program started, as gdrift does on quit.
	t.Cleanup(ws.Close)

	tm := teatest.NewTestModel(t, ui.New(t.Context(), ws, root), teatest.WithInitialTermSize(120, 40))

	waitForScreen(t, tm, "2 repos · 2 behind · 1 need attention")
	tm.Type("P")
	tm.Type("y")
	waitForScreen(t, tm, "pulled 1")

	if err := tm.Quit(); err != nil {
		t.Fatal(err)
	}

	tm.WaitFinished(t, teatest.WithFinalTimeout(e2eWait))

	if head, upstream := api.Git("rev-parse", "HEAD"), api.Git("rev-parse", "@{upstream}"); head != upstream {
		t.Errorf("api HEAD = %s, want its upstream %s", head, upstream)
	}

	if head := web.Git("rev-parse", "HEAD"); head != webHead {
		t.Errorf("web HEAD = %s, want it unchanged at %s", head, webHead)
	}

	if status := web.Git("status", "--porcelain"); status != "M README.md" {
		t.Errorf("web status = %q, want its local change kept", status)
	}
}
