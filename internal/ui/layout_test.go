package ui

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestComputeLayout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		width, height int
		focus, last   Pane
		zoom          bool
		want          Layout
	}{
		{
			name: "wide repos focus", width: 120, height: 50, focus: PaneRepos, last: PaneRepos,
			want: Layout{
				Repos:   Rect{X: 0, Y: 1, W: 52, H: 41},
				Commits: Rect{X: 0, Y: 42, W: 52, H: 3},
				Files:   Rect{X: 0, Y: 45, W: 52, H: 3},
				Diff:    Rect{X: 52, Y: 1, W: 68, H: 47},
			},
		},
		{
			name: "wide commits focus", width: 120, height: 50, focus: PaneCommits, last: PaneCommits,
			want: Layout{
				Repos:   Rect{X: 0, Y: 1, W: 52, H: 5},
				Commits: Rect{X: 0, Y: 6, W: 52, H: 25},
				Files:   Rect{X: 0, Y: 31, W: 52, H: 17},
				Diff:    Rect{X: 52, Y: 1, W: 68, H: 47},
			},
		},
		{
			name: "wide files focus", width: 120, height: 50, focus: PaneFiles, last: PaneFiles,
			want: Layout{
				Repos:   Rect{X: 0, Y: 1, W: 52, H: 5},
				Commits: Rect{X: 0, Y: 6, W: 52, H: 17},
				Files:   Rect{X: 0, Y: 23, W: 52, H: 25},
				Diff:    Rect{X: 52, Y: 1, W: 68, H: 47},
			},
		},
		{
			name: "diff focus sizes as last left pane", width: 120, height: 50, focus: PaneDiff, last: PaneFiles,
			want: Layout{
				Repos:   Rect{X: 0, Y: 1, W: 52, H: 5},
				Commits: Rect{X: 0, Y: 6, W: 52, H: 17},
				Files:   Rect{X: 0, Y: 23, W: 52, H: 25},
				Diff:    Rect{X: 52, Y: 1, W: 68, H: 47},
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
		{
			name: "left width has a minimum of 40", width: 90, height: 30, focus: PaneRepos, last: PaneRepos,
			want: Layout{
				Repos:   Rect{X: 0, Y: 1, W: 40, H: 21},
				Commits: Rect{X: 0, Y: 22, W: 40, H: 3},
				Files:   Rect{X: 0, Y: 25, W: 40, H: 3},
				Diff:    Rect{X: 40, Y: 1, W: 50, H: 27},
			},
		},
		{
			name: "narrow hides the diff", width: 80, height: 30, focus: PaneRepos, last: PaneRepos,
			want: Layout{
				Repos:      Rect{X: 0, Y: 1, W: 80, H: 21},
				Commits:    Rect{X: 0, Y: 22, W: 80, H: 3},
				Files:      Rect{X: 0, Y: 25, W: 80, H: 3},
				DiffHidden: true,
			},
		},
		{
			name: "narrow just below the threshold", width: 89, height: 30, focus: PaneRepos, last: PaneRepos,
			want: Layout{
				Repos:      Rect{X: 0, Y: 1, W: 89, H: 21},
				Commits:    Rect{X: 0, Y: 22, W: 89, H: 3},
				Files:      Rect{X: 0, Y: 25, W: 89, H: 3},
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
		{name: "too narrow", width: 59, height: 30, focus: PaneRepos, last: PaneRepos, want: Layout{TooSmall: true}},
		{name: "too short", width: 80, height: 14, focus: PaneRepos, last: PaneRepos, want: Layout{TooSmall: true}},
		{name: "zero size", width: 0, height: 0, focus: PaneRepos, last: PaneRepos, want: Layout{TooSmall: true}},
	}

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
