package gittest

import (
	"os"
	"testing"
	"time"
)

func TestFixturesIgnoreInheritedEnv(t *testing.T) {
	t.Parallel()

	env := New(t)
	env.base = func() []string {
		return []string{
			"PATH=" + os.Getenv("PATH"),
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=user.name",
			"GIT_CONFIG_VALUE_0=Leaked",
			"GIT_CONFIG_PARAMETERS='user.email=leak@example.com'",
			"GIT_AUTHOR_DATE=2001-01-01T00:00:00Z",
			"GIT_COMMITTER_DATE=2001-01-01T00:00:00Z",
			"GIT_EDITOR=false",
			"GIT_PAGER=false",
		}
	}

	r := env.Init("fresh")
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	r.CommitAt("f.txt", "x", "first", when)
	r.Commit("g.txt", "y", "second")

	if got := r.Git("log", "-1", "--format=%an <%ae>"); got != "Test <test@example.com>" {
		t.Errorf("author = %q, want the isolated identity", got)
	}

	if got := r.Git("log", "-1", "--format=%ad", "--date=format:%Y"); got == "2001" {
		t.Errorf("inherited GIT_AUTHOR_DATE leaked: %q", got)
	}
}

func TestRunnerNeutralizesInheritedConfig(t *testing.T) {
	t.Parallel()

	r := New(t).Runner()

	want := map[string]bool{"GIT_CONFIG_COUNT=0": false, "GIT_CONFIG_PARAMETERS=": false}

	for _, kv := range r.Env {
		if _, ok := want[kv]; ok {
			want[kv] = true
		}
	}

	for kv, ok := range want {
		if !ok {
			t.Errorf("Runner.Env lacks %q", kv)
		}
	}
}
