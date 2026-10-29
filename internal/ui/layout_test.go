package ui

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

// layoutCase is a screen size, focus and zoom with the layout it gets.
type layoutCase struct {
	name          string
	width, height int
	focus, last   Pane
	zoom          bool
	want          Layout
}

// TestComputeLayout checks the layout of a wide screen: Repos on top with
// the bottom row below it, sized for the focused pane, and zoom.
func TestComputeLayout(t *testing.T) {
	t.Parallel()

	runLayoutCases(t, []layoutCase{
		{
			name: "wide repos focus", width: 120, height: 50, focus: PaneRepos, last: PaneRepos,
			want: Layout{
				Repos:   Rect{X: 0, Y: 1, W: 120, H: 29},
				Commits: Rect{X: 0, Y: 30, W: 52, H: 9},
				Files:   Rect{X: 0, Y: 39, W: 52, H: 9},
				Diff:    Rect{X: 52, Y: 30, W: 68, H: 18},
			},
		},
		{
			name: "wide commits focus", width: 120, height: 50, focus: PaneCommits, last: PaneCommits,
			want: Layout{
				Repos:   Rect{X: 0, Y: 1, W: 120, H: 8},
				Commits: Rect{X: 0, Y: 9, W: 52, H: 23},
				Files:   Rect{X: 0, Y: 32, W: 52, H: 16},
				Diff:    Rect{X: 52, Y: 9, W: 68, H: 39},
			},
		},
		{
			name: "wide files focus", width: 120, height: 50, focus: PaneFiles, last: PaneFiles,
			want: Layout{
				Repos:   Rect{X: 0, Y: 1, W: 120, H: 8},
				Commits: Rect{X: 0, Y: 9, W: 52, H: 16},
				Files:   Rect{X: 0, Y: 25, W: 52, H: 23},
				Diff:    Rect{X: 52, Y: 9, W: 68, H: 39},
			},
		},
		{
			name: "diff focus sizes as last left pane", width: 120, height: 50, focus: PaneDiff, last: PaneFiles,
			want: Layout{
				Repos:   Rect{X: 0, Y: 1, W: 120, H: 8},
				Commits: Rect{X: 0, Y: 9, W: 52, H: 16},
				Files:   Rect{X: 0, Y: 25, W: 52, H: 23},
				Diff:    Rect{X: 52, Y: 9, W: 68, H: 39},
			},
		},
		{
			name: "diff focus after repos sizes as commits", width: 120, height: 50, focus: PaneDiff, last: PaneRepos,
			want: Layout{
				Repos:   Rect{X: 0, Y: 1, W: 120, H: 8},
				Commits: Rect{X: 0, Y: 9, W: 52, H: 23},
				Files:   Rect{X: 0, Y: 32, W: 52, H: 16},
				Diff:    Rect{X: 52, Y: 9, W: 68, H: 39},
			},
		},
		{
			name: "zoom repos", width: 120, height: 50, focus: PaneRepos, last: PaneRepos, zoom: true,
			want: Layout{Repos: Rect{X: 0, Y: 1, W: 120, H: 47}},
		},
		{
			name: "zoom files", width: 120, height: 50, focus: PaneFiles, last: PaneFiles, zoom: true,
			want: Layout{Files: Rect{X: 0, Y: 1, W: 120, H: 47}},
		},
	})
}

// TestComputeLayoutSmallScreens checks the layout of narrow and short
// screens: the minimum width of the bottom left column, the hidden Diff
// pane and the smallest supported size.
func TestComputeLayoutSmallScreens(t *testing.T) {
	t.Parallel()

	runLayoutCases(t, []layoutCase{
		{
			name: "left width has a minimum of 40", width: 90, height: 30, focus: PaneRepos, last: PaneRepos,
			want: Layout{
				Repos:   Rect{X: 0, Y: 1, W: 90, H: 17},
				Commits: Rect{X: 0, Y: 18, W: 40, H: 5},
				Files:   Rect{X: 0, Y: 23, W: 40, H: 5},
				Diff:    Rect{X: 40, Y: 18, W: 50, H: 10},
			},
		},
		{
			name: "narrow hides the diff", width: 80, height: 30, focus: PaneRepos, last: PaneRepos,
			want: Layout{
				Repos:      Rect{X: 0, Y: 1, W: 80, H: 17},
				Commits:    Rect{X: 0, Y: 18, W: 80, H: 5},
				Files:      Rect{X: 0, Y: 23, W: 80, H: 5},
				DiffHidden: true,
			},
		},
		{
			name: "narrow just below the threshold", width: 89, height: 30, focus: PaneRepos, last: PaneRepos,
			want: Layout{
				Repos:      Rect{X: 0, Y: 1, W: 89, H: 17},
				Commits:    Rect{X: 0, Y: 18, W: 89, H: 5},
				Files:      Rect{X: 0, Y: 23, W: 89, H: 5},
				DiffHidden: true,
			},
		},
		{
			name: "narrow zoom diff shows the diff", width: 80, height: 30, focus: PaneDiff, last: PaneRepos, zoom: true,
			want: Layout{Diff: Rect{X: 0, Y: 1, W: 80, H: 27}},
		},
		{
			name: "minimum size", width: 60, height: 15, focus: PaneCommits, last: PaneCommits,
			want: Layout{
				Repos:      Rect{X: 0, Y: 1, W: 60, H: 5},
				Commits:    Rect{X: 0, Y: 6, W: 60, H: 4},
				Files:      Rect{X: 0, Y: 10, W: 60, H: 3},
				DiffHidden: true,
			},
		},
		{
			name: "minimum size repos focus", width: 90, height: 15, focus: PaneRepos, last: PaneRepos,
			want: Layout{
				Repos:   Rect{X: 0, Y: 1, W: 90, H: 6},
				Commits: Rect{X: 0, Y: 7, W: 40, H: 3},
				Files:   Rect{X: 0, Y: 10, W: 40, H: 3},
				Diff:    Rect{X: 40, Y: 7, W: 50, H: 6},
			},
		},
		{name: "too narrow", width: 59, height: 30, focus: PaneRepos, last: PaneRepos, want: Layout{TooSmall: true}},
		{name: "too short", width: 80, height: 14, focus: PaneRepos, last: PaneRepos, want: Layout{TooSmall: true}},
		{name: "zero size", width: 0, height: 0, focus: PaneRepos, last: PaneRepos, want: Layout{TooSmall: true}},
	})
}

func runLayoutCases(t *testing.T, tests []layoutCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ComputeLayout(tt.width, tt.height, tt.focus, tt.last, tt.zoom)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ComputeLayout(%d, %d, %d, %d, %t) mismatch (-want +got):\n%s",
					tt.width, tt.height, tt.focus, tt.last, tt.zoom, diff)
			}
		})
	}
}
