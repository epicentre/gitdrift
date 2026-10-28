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
	}{
		{name: "version flag", args: []string{"--version"}, wantStdout: "gdrift dev"},
		{
			name:       "help flag",
			args:       []string{"--help"},
			wantStderr: []string{"Usage: gdrift [flags] [dir]", "git config --local or --global", "gitdrift.base", "gitdrift.badge"},
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
		{name: "explicit dir", args: []string{tmp}, wantStart: resolved(t, tmp)},
		{
			name:            "start error",
			args:            []string{tmp},
			startErr:        errors.New("boom"),
			wantCode:        1,
			wantStderrExact: "gdrift: boom\n",
			wantStart:       resolved(t, tmp),
		},
		{
			name:            "interrupted",
			args:            []string{tmp},
			startErr:        context.Canceled,
			wantCode:        130,
			wantEmptyStderr: true,
			wantStart:       resolved(t, tmp),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				stdout, stderr bytes.Buffer
				gotDir         string
			)

			start := func(_ context.Context, dir string) error {
				gotDir = dir

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

			if gotDir != tt.wantStart {
				t.Errorf("start dir = %q, want %q", gotDir, tt.wantStart)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			start := func(context.Context, string) error { return nil }
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

	var gotDir string

	start := func(_ context.Context, d string) error {
		gotDir = d

		return nil
	}

	var stdout, stderr bytes.Buffer
	if code := run(t.Context(), nil, &stdout, &stderr, start); code != 0 {
		t.Fatalf("code = %d, want 0 (stderr %q)", code, stderr.String())
	}

	if want := resolved(t, dir); gotDir != want {
		t.Errorf("start dir = %q, want %q", gotDir, want)
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
