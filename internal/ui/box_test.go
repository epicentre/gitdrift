package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFit(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	st := newStyles(true)

	tests := []struct {
		name  string
		s     string
		width int
		want  string
	}{
		{"shorter is padded", "ok", 5, "ok   "},
		{"exact stays", "fits!", 5, "fits!"},
		{"longer is cut", "too long", 5, "too l"},
		{"a wide character cut at the edge leaves a padded cell", "ab漢字", 5, "ab漢 "},
		{"a wide character that does not fit at all", "漢", 1, " "},
		{"styling is kept", st.Badge.Render("deps") + "x", 4, "deps"},
		{"no room", "ok", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := fit(tt.s, tt.width)
			if s := ansi.Strip(got); s != tt.want {
				t.Errorf("fit(%q, %d) = %q, want %q", tt.s, tt.width, s, tt.want)
			}

			if w := ansi.StringWidth(got); w != max(tt.width, 0) {
				t.Errorf("fit(%q, %d) width = %d, want %d", tt.s, tt.width, w, tt.width)
			}
		})
	}
}
