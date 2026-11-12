package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/epicentre/gitdrift/internal/drift"
)

const (
	brandName = "gitdrift"
	// separator joins the parts of the header, status and footer lines.
	separator = " · "
	fetching  = "fetching…"
	reading   = "reading…"
	// fetchFailed starts drift's reason for a remote whose fetch failed.
	fetchFailed = "fetch failed: "
)

// Footer hints per focused pane. In a workspace with folders the Repos pane
// names its folder tabs and the filter.
const (
	footerRepos              = "space select · p pull · P pull all safe · r/R refresh · b base · z zoom · tab pane · ? help"
	footerReposZoomed        = "space select · p pull · P pull all safe · f only-here filter · z unzoom · ? help"
	footerReposFolders       = "space select · p pull · P pull all safe · ←→ folders · / filter · r/R refresh · b base · z zoom · ? help"
	footerReposZoomedFolders = "space select · p pull · P pull all safe · f only-here filter · ←→ folders · / filter · z unzoom · ? help"
	footerCommits            = "←→ tabs · enter files · z zoom · esc back · ? help"
	footerBranches           = "space select · d delete gone · ←→ tabs · z zoom · esc back · ? help"
	footerFiles              = "enter diff · ←→ tabs · z zoom · esc back · ? help"
	footerDiff               = "↑↓ scroll · pgup/pgdn page · z zoom · esc back · ? help"
)

// headerLine renders "gitdrift  <root>" on the left and the workspace counts
// on the right, filling width: total repositories, then from the snapshots
// read so far those behind, needing attention and with work only here, and
// the loading progress while there is one. The root is cut first when space
// runs out.
func headerLine(root string, snaps []drift.Snapshot, total int, progress string, width int, st Styles) string {
	if width <= 0 {
		return ""
	}

	right := headerCounts(snaps, total)
	if progress != "" {
		right += separator + st.Spinner.Render(progress)
	}

	rightW := ansi.StringWidth(right)

	if rightW >= width {
		return ansi.Truncate(right, width, "…")
	}

	left := brandName + "  " + sanitize(root)
	if room := width - rightW - 1; ansi.StringWidth(left) > room {
		left = ansi.Truncate(left, room, "…")
	}

	gap := width - ansi.StringWidth(left) - rightW

	styled := st.Brand.Render(left)
	if rest, ok := strings.CutPrefix(left, brandName); ok {
		styled = st.Brand.Render(brandName) + st.Muted.Render(rest)
	}

	return styled + strings.Repeat(" ", gap) + right
}

// headerCounts summarizes the workspace: total repositories, repositories
// behind their upstream, repositories needing attention, and repositories
// (counted once for all worktrees) with work that exists only here.
func headerCounts(snaps []drift.Snapshot, total int) string {
	var behind, attention int

	onlyHere := make(map[string]bool)

	for _, s := range snaps {
		if s.Status.Behind > 0 {
			behind++
		}

		if drift.CategoryOf(s) == drift.CategoryNeedsAttention {
			attention++
		}

		if hasOnlyHere(s) {
			onlyHere[s.Loc.CommonDir] = true
		}
	}

	noun := "repos"
	if total == 1 {
		noun = "repo"
	}

	return fmt.Sprintf("%d %s · %d behind · %d need attention · %d only here",
		total, noun, behind, attention, len(onlyHere))
}

// hasOnlyHere reports whether the repository has work no remote has.
func hasOnlyHere(s drift.Snapshot) bool {
	return s.Work.OnlyHereTotal() > 0 || s.Work.NoRemote
}

// tildePath shows path relative to home as "~/...".
func tildePath(path, home string) string {
	if home == "" || home == string(filepath.Separator) {
		return path
	}

	if path == home {
		return "~"
	}

	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rest
	}

	return path
}

// statusLine describes the repository under the cursor: its folder and
// name (see repoLabel), the note its row shows (a pull or fetch in progress,
// or the last pull's result), its base and why it needs attention, with the
// message of each failed fetch.
func statusLine(s drift.Snapshot, loaded bool, note token, width int, st Styles) string {
	parts := []string{st.Strong.Render(repoLabel(s.Loc))}

	if note.text != "" {
		parts = append(parts, note.style.Render(note.text))
	}

	switch {
	case !loaded:
		parts = append(parts, st.Muted.Render(reading))
	case s.Base.Ref != "":
		parts = append(parts, fmt.Sprintf("base %s (%s)", sanitize(s.Base.Ref), s.Base.Source))
	default:
		parts = append(parts, "no base")
	}

	if loaded {
		for _, reason := range drift.AttentionReasons(s) {
			parts = append(parts, st.Attention.Render(sanitize(withFetchMessage(reason, s.Fetch))))
		}
	}

	return ansi.Truncate(strings.Join(parts, separator), width, "…")
}

// withFetchMessage appends to a "fetch failed: <remote>" reason the message
// of that fetch, in parentheses. Other reasons are returned as they are.
func withFetchMessage(reason string, f drift.FetchState) string {
	remote, ok := strings.CutPrefix(reason, fetchFailed)
	if !ok {
		return reason
	}

	if msg := f.Errors[remote]; msg != "" {
		return reason + " (" + msg + ")"
	}

	return reason
}

// footerLine shows the key hints for the focused pane; the Repos pane's
// depend on zoom and on whether the workspace has folders, the Commits
// pane's on its active tab.
func footerLine(focus Pane, zoom, folders bool, tab Tab, width int, st Styles) string {
	hints := footerRepos

	switch focus {
	case PaneRepos:
		switch {
		case zoom && folders:
			hints = footerReposZoomedFolders
		case zoom:
			hints = footerReposZoomed
		case folders:
			hints = footerReposFolders
		}
	case PaneCommits:
		hints = footerCommits
		if tab == TabBranches {
			hints = footerBranches
		}
	case PaneFiles:
		hints = footerFiles
	case PaneDiff:
		hints = footerDiff
	}

	return st.Muted.Render(ansi.Truncate(hints, width, "…"))
}
