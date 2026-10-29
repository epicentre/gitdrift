package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"
)

// TestBoxBorders checks that an unfocused pane has a thin rounded border and
// the focused one a thick border, so that the focus stands out.
func TestBoxBorders(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	tests := []struct {
		name    string
		focused bool
		want    []string
	}{
		{"unfocused", false, []string{"╭─ 2 Commits ──╮", "│ a            │", "╰──────────────╯"}},
		{"focused", true, []string{"┏━ 2 Commits ━━┓", "┃ a            ┃", "┗━━━━━━━━━━━━━━┛"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := strings.Split(ansi.Strip(box("2 Commits", []string{" a"}, 16, 3, tt.focused, newStyles(true))), "\n")
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("box mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
