package git

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestMessage(t *testing.T) {
	t.Parallel()

	const overwritten = "error: Your local changes to the following files would be overwritten by merge:"

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"fatal after warning", &CommandError{Stderr: "Warning: Permanently added host\nfatal: no route\n"}, "fatal: no route"},
		{"error line", &CommandError{Stderr: "hint: x\nerror: bad\nmore\n"}, "error: bad"},
		{"last line fallback", &CommandError{Stderr: "first\nsecond\n\n"}, "second"},
		{"trimmed", &CommandError{Stderr: "  fatal: spaced \r\n"}, "fatal: spaced"},
		{
			"listed files",
			&CommandError{Stderr: overwritten + "\n\tcmd/payments/total.go\n\tREADME.md\nPlease commit your changes or stash them before you merge.\nAborting\n"},
			overwritten + " cmd/payments/total.go, README.md",
		},
		{
			"more than five listed files",
			&CommandError{Stderr: overwritten + "\n\ta\n\tb\n\tc\n\td\n\te\n\tf\n\tg\nAborting\n"},
			overwritten + " a, b, c, d, e +2 more",
		},
		{
			"colon without a list",
			&CommandError{Stderr: "fatal: unable to access 'https://example.com/':\nAborting\n"},
			"fatal: unable to access 'https://example.com/':",
		},
		{
			"command error without stderr",
			&CommandError{Args: []string{"merge"}, Err: errors.New("signal: killed")},
			"git merge: signal: killed",
		},
		{
			"wrapped command error",
			fmt.Errorf("ops: fetch origin: %w", &CommandError{Args: []string{cmdFetch}, Stderr: "fatal: 'nowhere' does not appear to be a git repository\n"}),
			"fatal: 'nowhere' does not appear to be a git repository",
		},
		{"other error keeps its first line", errors.New("fetch refused: remote \"x\"\nsecond line"), "fetch refused: remote \"x\""},
		{
			"timeout wins over a stray stderr line",
			fmt.Errorf("ops: fetch origin: %w", &CommandError{Args: []string{cmdFetch}, Stderr: "Warning: Permanently added host\n", Err: context.DeadlineExceeded}),
			"timed out",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := Message(tt.err); got != tt.want {
				t.Errorf("Message() = %q, want %q", got, tt.want)
			}
		})
	}
}
