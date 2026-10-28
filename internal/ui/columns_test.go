package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/google/go-cmp/cmp"
)

// placed is where a column landed: its first cell and its width, or -1 and 0
// when it is not shown.
type placed struct{ X, W int }

func geometry(l columnLayout) []placed {
	out := make([]placed, len(l.cols))
	for i := range l.cols {
		out[i] = placed{X: -1}
		if l.shown[i] {
			out[i] = placed{X: l.x[i], W: l.widths[i]}
		}
	}

	return out
}

func TestLayoutColumns(t *testing.T) {
	t.Parallel()

	// name and branch shrink, branch first; a and b drop, b first.
	name := column{width: 10, min: 6, shrink: 2}
	branch := column{width: 12, min: 5, shrink: 1}
	a := column{width: 3, drop: 2}
	b := column{width: 4, drop: 1}

	tests := []struct {
		name  string
		cols  []column
		width int
		want  []placed
	}{
		{
			"spare width widens the gaps",
			[]column{{width: 3}, {width: 4}, {width: 2}},
			15,
			[]placed{{0, 3}, {6, 4}, {13, 2}},
		},
		{
			"uneven spare goes to the first gaps",
			[]column{{width: 3}, {width: 4}, {width: 2}},
			16,
			[]placed{{0, 3}, {7, 4}, {14, 2}},
		},
		{
			"exact fit keeps one space",
			[]column{{width: 3}, {width: 4}, {width: 2}},
			11,
			[]placed{{0, 3}, {4, 4}, {9, 2}},
		},
		{
			"a fill column takes the spare instead of the gaps",
			[]column{{width: 7}, {width: 10, fill: true, shrink: 1, min: 1}, {width: 2, align: alignRight}},
			30,
			[]placed{{0, 7}, {8, 19}, {28, 2}},
		},
		{"one column leaves the spare at the end", []column{{width: 5}}, 10, []placed{{0, 5}}},
		{"empty columns take no room", []column{{width: 3}, {}, {width: 2}}, 10, []placed{{0, 3}, {-1, 0}, {8, 2}}},
		{"an empty fill column still fills", []column{{width: 3}, {fill: true}}, 10, []placed{{0, 3}, {4, 6}}},
		{"a column's own gap", []column{{width: 3}, {width: 2, gap: 2}}, 7, []placed{{0, 3}, {5, 2}}},
		{"shrink order: branch first", []column{name, branch, a}, 20, []placed{{0, 10}, {11, 5}, {17, 3}}},
		{"then the next down to its minimum", []column{name, branch, a}, 18, []placed{{0, 8}, {9, 5}, {15, 3}}},
		{
			"drop order once every column is at its minimum",
			[]column{name, branch, a, b},
			20,
			[]placed{{0, 10}, {11, 5}, {17, 3}, {-1, 0}},
		},
		{
			"dropping goes on in priority order",
			[]column{name, branch, a, b},
			14,
			[]placed{{0, 8}, {9, 5}, {-1, 0}, {-1, 0}},
		},
		{
			"a dropped column gives its room back to shrunk ones",
			[]column{{width: 10, min: 6, shrink: 1}, {width: 8, drop: 1}},
			12,
			[]placed{{0, 10}, {-1, 0}},
		},
		{"a minimum above the content changes nothing", []column{{width: 4, min: 13, shrink: 1}, {width: 2}}, 7, []placed{{0, 4}, {5, 2}}},
		{
			"columns never dropped: the shrinkable go below their minimum",
			[]column{{width: 20, min: 5, shrink: 1, fill: true}, {width: 3}},
			6,
			[]placed{{0, 2}, {3, 3}},
		},
		{"fixed columns wider than the row stay whole", []column{{width: 10}, {width: 10}}, 5, []placed{{0, 10}, {11, 10}}},
		{"no columns", nil, 10, []placed{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(tt.want, geometry(layoutColumns(tt.cols, tt.width))); diff != "" {
				t.Errorf("layoutColumns(%d) mismatch (-want +got):\n%s", tt.width, diff)
			}
		})
	}
}

func TestColumnLayoutRender(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	st := newStyles(true)

	tests := []struct {
		name  string
		cols  []column
		cells []string
		width int
		want  string
	}{
		{
			"aligned and spread to the edge",
			[]column{{width: 4}, {width: 3, align: alignRight}, {width: 2}},
			[]string{"ab", "7", "ok"},
			14,
			"ab       7  ok",
		},
		{
			"styled columns keep their width",
			[]column{{width: 4, style: st.Badge}, {width: 4, style: st.Attention}},
			[]string{"deps", "x"},
			9,
			"deps x   ",
		},
		{
			"a shrunk value is cut with an ellipsis",
			[]column{{width: 16, min: 8, shrink: 1}, {width: 2}},
			[]string{"feature/checkout", "ok"},
			11,
			"feature… ok",
		},
		{
			"a shrunk value cut from the left keeps its end",
			[]column{{width: 21, min: 4, shrink: 1, keepEnd: true}, {width: 2}},
			[]string{"app/Cash/Total.php", "+1"},
			14,
			"…/Total.php +1",
		},
		{
			"empty and missing cells are blank",
			[]column{{width: 2}, {width: 3}, {width: 2}},
			[]string{"*2", ""},
			9,
			"*2       ",
		},
		{"no room draws nothing", []column{{width: 3}}, []string{"xyz"}, 0, ""},
		{"wide values are measured in cells", []column{{width: 4}, {width: 1}}, []string{"漢字", "x"}, 7, "漢字  x"},
		{
			"a wide value is cut on a cell boundary",
			[]column{{width: 8, min: 1, shrink: 1}, {width: 1}},
			[]string{"漢字漢字", "x"},
			7,
			"漢字… x",
		},
		{
			"fixed columns that overflow are cut to the row, a cut wide character padded",
			[]column{{width: 3}, {width: 4}},
			[]string{"end", "漢字"},
			5,
			"end  ",
		},
		{
			"a wide value cut from the left on a cell boundary",
			[]column{{width: 8, min: 1, shrink: 1, keepEnd: true}, {width: 1}},
			[]string{"漢字漢字", "x"},
			6,
			"…字  x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := layoutColumns(tt.cols, tt.width).render(tt.cells)

			if s := ansi.Strip(got); s != tt.want {
				t.Errorf("render() = %q, want %q", s, tt.want)
			}

			if w := ansi.StringWidth(got); w != tt.width {
				t.Errorf("render() width = %d, want %d", w, tt.width)
			}
		})
	}
}

func TestColumnLayoutRenderSpan(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	cols := []column{{width: 4}, {width: 2}, {width: 3, align: alignRight}, {width: 2}}
	cells := []string{"name", "*2", "-80", "ok"}

	tests := []struct {
		name     string
		cols     []column
		width    int
		from, to int
		value    token
		a        align
		want     string
	}{
		{"a note across the columns to the end", cols, 16, 1, 4, token{text: "pulling…"}, alignLeft, "name  pulling…  "},
		{"a note cut to the span", cols, 12, 1, 4, token{text: "pull failed: timed out"}, alignLeft, "name pull f…"},
		{"a value right-aligned across two columns", cols, 14, 1, 3, token{text: "binary"}, alignRight, "name binary ok"},
		{
			"a note after the last column when its own are empty",
			[]column{{width: 4}, {}, {}},
			12, 1, 3,
			token{text: "fetching…"},
			alignLeft, "name fetchi…",
		},
		{
			"a note that has no room is not drawn",
			[]column{{width: 4}, {width: 2}, {width: 3}},
			4, 1, 3,
			token{text: "fetching…"},
			alignLeft, "name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := layoutColumns(tt.cols, tt.width).renderSpan(cells, tt.from, tt.to, tt.value, tt.a)

			if s := ansi.Strip(got); s != tt.want {
				t.Errorf("renderSpan() = %q, want %q", s, tt.want)
			}

			if w := ansi.StringWidth(got); w != tt.width {
				t.Errorf("renderSpan() width = %d, want %d", w, tt.width)
			}
		})
	}
}

func TestInclude(t *testing.T) {
	t.Parallel()
	skipEastAsianWidths(t)

	cols := []column{{width: 1}, {}, {min: 13}}
	include(cols, []string{"xyz", "漢字漢"})
	include(cols, []string{"a\x1b[31mb", "x", "z"})

	got := []int{cols[0].width, cols[1].width, cols[2].width, cols[2].min}
	if diff := cmp.Diff([]int{3, 6, 1, 13}, got); diff != "" {
		t.Errorf("include() widths and minimum mismatch (-want +got):\n%s", diff)
	}

	if got := columnsWidth(cols); got != 12 {
		t.Errorf("columnsWidth() = %d, want 12", got)
	}
}
