package ui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// sanitize makes repository-controlled text safe to draw: it strips ANSI
// escape sequences, then drops every remaining control character (tab and
// newline included) and every character that can reorder or hide text.
// Invalid UTF-8 becomes the replacement character.
func sanitize(s string) string {
	return strings.Map(safeRune, ansi.Strip(s))
}

// sanitizeAll sanitizes each of ss into a new slice.
func sanitizeAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = sanitize(s)
	}

	return out
}

// sanitizeKeepTabs is sanitize for text whose tabs are expanded afterwards:
// it keeps the tabs, so that their stops are measured on the text that is
// drawn.
func sanitizeKeepTabs(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}

		return safeRune(r)
	}, ansi.Strip(s))
}

// safeRune drops control characters and the characters that can reorder or
// hide text.
func safeRune(r rune) rune {
	if unicode.IsControl(r) || isHiddenFormat(r) {
		return -1
	}

	return r
}

// isHiddenFormat reports the characters that change how text around them
// shows without showing themselves: every format character (bidi controls,
// zero-width characters, soft hyphen, tags and the like) and the line and
// paragraph separators.
func isHiddenFormat(r rune) bool {
	return unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)
}
