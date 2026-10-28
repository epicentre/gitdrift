package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/epicentre/gitdrift/internal/workspace"
)

func resolved(t *testing.T, dir string) string {
	t.Helper()

	got, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}

	return got
}

func TestRun(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	file := filepath.Join(tmp, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name            string
		args            []string
		startErr        error
		wantCode        int
		wantStdout      string
		wantStderr      []string
		wantStderrExact string
		wantEmptyStderr bool
		wantStart       string
		wantDepth       int
	}{
		{name: "version flag", args: []string{"--version"}, wantStdout: "gdrift dev"},
		{
			name:       "help flag",
			args:       []string{"--help"},
			wantStderr: []string{"Usage: gdrift [flags] [dir]", "git config --local or --global", "gitdrift.base", "gitdrift.badge", "--depth N", "node_modules or vendor"},
		},
		{
			name:       "too many args",
			args:       []string{"a", "b"},
			wantCode:   2,
			wantStderr: []string{"gdrift: at most one directory argument"},
		},
		{
			name:       "unknown flag",
			args:       []string{"--nope"},
			wantCode:   2,
			wantStderr: []string{"flag provided but not defined"},
		},
		{
			name:       "missing dir",
			args:       []string{filepath.Join(tmp, "missing")},
			wantCode:   1,
			wantStderr: []string{"gdrift: ", "no such file or directory"},
		},
		{
			name:       "file not dir",
			args:       []string{file},
			wantCode:   1,
			wantStderr: []string{"is not a directory"},
		},
		{name: "explicit dir", args: []string{tmp}, wantStart: resolved(t, tmp), wantDepth: workspace.DefaultDepth},
		{
			name:            "start error",
			args:            []string{tmp},
			startErr:        errors.New("boom"),
			wantCode:        1,
			wantStderrExact: "gdrift: boom\n",
			wantStart:       resolved(t, tmp),
			wantDepth:       workspace.DefaultDepth,
		},
		{
			name:            "interrupted",
			args:            []string{tmp},
			startErr:        context.Canceled,
			wantCode:        130,
			wantEmptyStderr: true,
			wantStart:       resolved(t, tmp),
			wantDepth:       workspace.DefaultDepth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				stdout, stderr bytes.Buffer
				gotCfg         Config
			)

			start := func(_ context.Context, cfg Config) error {
				gotCfg = cfg

				return tt.startErr
			}

			code := run(t.Context(), tt.args, &stdout, &stderr, start)

			if code != tt.wantCode {
				t.Errorf("code = %d, want %d (stderr %q)", code, tt.wantCode, stderr.String())
			}

			if !strings.HasPrefix(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want prefix %q", stdout.String(), tt.wantStdout)
			}

			for _, want := range tt.wantStderr {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
				}
			}

			if tt.wantEmptyStderr && stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}

			if tt.wantStderrExact != "" && stderr.String() != tt.wantStderrExact {
				t.Errorf("stderr = %q, want exactly %q", stderr.String(), tt.wantStderrExact)
			}

			if gotCfg.Dir != tt.wantStart {
				t.Errorf("start dir = %q, want %q", gotCfg.Dir, tt.wantStart)
			}

			if gotCfg.Depth != tt.wantDepth {
				t.Errorf("start depth = %d, want %d", gotCfg.Depth, tt.wantDepth)
			}
		})
	}
}

func TestRunDepth(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	const flagDepth = "--depth"

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantDepth  int
		wantStderr string
	}{
		{name: "separate value", args: []string{flagDepth, "3", tmp}, wantDepth: 3},
		{name: "equals value", args: []string{flagDepth + "=2", tmp}, wantDepth: 2},
		{name: "default", args: []string{tmp}, wantDepth: workspace.DefaultDepth},
		{
			name:       "zero",
			args:       []string{flagDepth, "0", tmp},
			wantCode:   2,
			wantStderr: "gdrift: --depth must be 1 or more\nUsage: gdrift",
		},
		{
			name:       "negative",
			args:       []string{flagDepth + "=-1", tmp},
			wantCode:   2,
			wantStderr: "gdrift: --depth must be 1 or more",
		},
		{
			name:       "not a number",
			args:       []string{flagDepth, "x", tmp},
			wantCode:   2,
			wantStderr: "gdrift: invalid value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				stdout, stderr bytes.Buffer
				gotCfg         Config
				called         bool
			)

			start := func(_ context.Context, cfg Config) error {
				gotCfg, called = cfg, true

				return nil
			}

			if code := run(t.Context(), tt.args, &stdout, &stderr, start); code != tt.wantCode {
				t.Fatalf("code = %d, want %d (stderr %q)", code, tt.wantCode, stderr.String())
			}

			if tt.wantCode != 0 {
				if called {
					t.Error("start was called for an invalid depth")
				}

				if !strings.HasPrefix(stderr.String(), tt.wantStderr) {
					t.Errorf("stderr = %q, want prefix %q", stderr.String(), tt.wantStderr)
				}

				return
			}

			if want := (Config{Dir: resolved(t, tmp), Depth: tt.wantDepth}); gotCfg != want {
				t.Errorf("start config = %+v, want %+v", gotCfg, want)
			}
		})
	}
}

func TestRunPrintsUsageOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantFirst string
	}{
		{name: "help flag", args: []string{"--help"}},
		{name: "short help flag", args: []string{"-h"}},
		{name: "unknown flag", args: []string{"--nope"}, wantFirst: "gdrift: flag provided but not defined"},
		{name: "too many args", args: []string{"a", "b"}, wantFirst: "gdrift: at most one directory argument"},
		{name: "depth zero", args: []string{"--depth", "0"}, wantFirst: "gdrift: --depth must be 1 or more"},
		{name: "depth negative", args: []string{"--depth=-2"}, wantFirst: "gdrift: --depth must be 1 or more"},
		{name: "depth not a number", args: []string{"--depth", "x"}, wantFirst: "gdrift: invalid value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			start := func(context.Context, Config) error { return nil }
			run(t.Context(), tt.args, &stdout, &stderr, start)

			got := stderr.String()
			if n := strings.Count(got, "Usage: gdrift"); n != 1 {
				t.Errorf("usage printed %d times, want 1:\n%s", n, got)
			}

			if tt.wantFirst != "" && !strings.HasPrefix(got, tt.wantFirst) {
				t.Errorf("stderr = %q, want it to start with %q", got, tt.wantFirst)
			}
		})
	}
}

func TestRunDefaultDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var gotCfg Config

	start := func(_ context.Context, cfg Config) error {
		gotCfg = cfg

		return nil
	}

	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), nil, &stdout, &stderr, start); code != 0 {
		t.Fatalf("code = %d, want 0 (stderr %q)", code, stderr.String())
	}

	if want := (Config{Dir: resolved(t, dir), Depth: workspace.DefaultDepth}); gotCfg != want {
		t.Errorf("start config = %+v, want %+v", gotCfg, want)
	}
}

// TestQuitSignals pins the signals that run the quit sequence instead of
// killing gdrift: SIGHUP arrives when the terminal closes.
func TestQuitSignals(t *testing.T) {
	t.Parallel()

	want := []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
	if !slices.Equal(quitSignals, want) {
		t.Fatalf("quitSignals = %v, want %v", quitSignals, want)
	}
}
