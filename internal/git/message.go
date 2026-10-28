package git

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

const (
	// maxListed is the most lines of a git file list that Message keeps.
	maxListed = 5
	// timedOut is the message for a command that hit its deadline.
	timedOut = "timed out"
)

// Message returns the most telling line of err for a status line. For a
// failed git command it is git's own message: the first stderr line that
// starts with "fatal:" or "error:", else the last non-empty one (a leading
// line may be a warning or a hint). When that line ends with ":" and
// indented lines follow, as in git's lists of files, up to five of them are
// appended, joined by ", ", then " +N more". A command that hit its
// deadline reports "timed out", since its stderr may hold only a warning.
// Without stderr, and for any other error, it is the first line of the
// error text. The result is not sanitized.
func Message(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return timedOut
	}

	if ce, ok := errors.AsType[*CommandError](err); ok {
		if msg := stderrMessage(ce.Stderr); msg != "" {
			return msg
		}
	}

	first, _, _ := strings.Cut(err.Error(), "\n")

	return strings.TrimSpace(first)
}

func stderrMessage(stderr string) string {
	lines := strings.Split(stderr, "\n")
	picked := -1

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		picked = i

		if strings.HasPrefix(line, "fatal:") || strings.HasPrefix(line, "error:") {
			break
		}
	}

	if picked < 0 {
		return ""
	}

	msg := strings.TrimSpace(lines[picked])
	if strings.HasSuffix(msg, ":") {
		msg += listed(lines[picked+1:])
	}

	return msg
}

// listed joins the indented lines at the start of lines, the entries of a
// git file list.
func listed(lines []string) string {
	var items []string

	for _, line := range lines {
		item := strings.TrimSpace(line)
		if item == "" || !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, " ") {
			break
		}

		items = append(items, item)
	}

	if len(items) == 0 {
		return ""
	}

	shown := items[:min(len(items), maxListed)]
	out := " " + strings.Join(shown, ", ")

	if more := len(items) - len(shown); more > 0 {
		out += " +" + strconv.Itoa(more) + " more"
	}

	return out
}
