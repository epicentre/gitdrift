package ui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/epicentre/gitdrift/internal/drift"
	"github.com/epicentre/gitdrift/internal/repo"
)

// detailDebounce is how long the detail panes wait after a cursor or tab
// change before they load, so that moving across many rows loads only the
// row the cursor stops on.
const detailDebounce = 100 * time.Millisecond

// Tab is one tab of the Commits pane.
type Tab int

// Tabs, in the order ] cycles through them.
const (
	TabIncoming Tab = iota + 1
	TabBase
	TabLocal
	TabBranches
)

const (
	tabCount = 4
	// tabGap separates the tabs of the Commits pane title.
	tabGap = "  "
	// remoteRefs qualifies a remote-tracking branch name, so that a tag of
	// the same name is never read instead.
	remoteRefs = "refs/remotes/"
)

// next returns the tab after t, wrapping around.
func (t Tab) next() Tab { return t%tabCount + 1 }

// prev returns the tab before t, wrapping around.
func (t Tab) prev() Tab { return (t+tabCount-2)%tabCount + 1 }

// names returns the tab's title and the short form inactive tabs take when
// the title does not fit.
func (t Tab) names() (name, short string) {
	switch t {
	case TabIncoming:
		return "Incoming", "Inc"
	case TabBase:
		return "Base", "Base"
	case TabLocal:
		return "Local", "Loc"
	case TabBranches:
		return "Branches", "Br"
	default:
		return "", ""
	}
}

// tabTitle renders the tabs of the Commits pane for s: each tab with its
// count, the active one in brackets. When that is wider than width, the
// inactive tabs shrink to their short names without counts.
func tabTitle(s drift.Snapshot, active Tab, width int) string {
	if title := joinTabs(s, active, false); ansi.StringWidth(title) <= width {
		return title
	}

	return joinTabs(s, active, true)
}

func joinTabs(s drift.Snapshot, active Tab, shrink bool) string {
	parts := make([]string, 0, tabCount)

	for t := TabIncoming; t <= TabBranches; t++ {
		name, short := t.names()
		if count := tabCountLabel(s, t); count != "" {
			name += " " + count
		}

		switch {
		case t == active:
			parts = append(parts, "["+name+"]")
		case shrink:
			parts = append(parts, short)
		default:
			parts = append(parts, name)
		}
	}

	return strings.Join(parts, tabGap)
}

// tabCountLabel is the count shown after a tab name; empty for zero. The
// Branches count is the gone branches (✂) and the work only here (⌂).
func tabCountLabel(s drift.Snapshot, t Tab) string {
	switch t {
	case TabIncoming:
		return positive(s.Status.Behind)
	case TabBase:
		return positive(baseBehind(s.Base))
	case TabLocal:
		return positive(localChanges(s.Status))
	case TabBranches:
		var parts []string

		if n := len(s.Work.Gone); n > 0 {
			parts = append(parts, "✂"+strconv.Itoa(n))
		}

		if n := s.Work.OnlyHereTotal(); n > 0 {
			parts = append(parts, "⌂"+strconv.Itoa(n))
		}

		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func positive(n int) string {
	if n <= 0 {
		return ""
	}

	return strconv.Itoa(n)
}

// localChanges counts the changed tracked entries and untracked paths.
func localChanges(st repo.Status) int { return st.Tracked + st.Untracked }

// rangeTarget is the commit the Incoming or Base tab compares HEAD with: the
// upstream, or the fully qualified base. It is false when the tab has no
// commits to list.
func rangeTarget(s drift.Snapshot, t Tab) (string, bool) {
	if s.Status.Unborn {
		return "", false
	}

	switch t {
	case TabIncoming:
		return s.Status.UpstreamSHA, s.Status.UpstreamSHA != "" && s.Status.Behind > 0
	case TabBase:
		return remoteRefs + s.Base.Ref, s.Base.Ref != "" && baseBehind(s.Base) > 0
	case TabLocal, TabBranches:
		return "", false
	default:
		return "", false
	}
}

// allSpec is the comparison of the "All …" entry: everything target brought
// in since its merge base with HEAD, which is what a pull or merge would
// bring.
func allSpec(target string) repo.DiffSpec {
	return repo.DiffSpec{Kind: repo.DiffMergeBase, From: "HEAD", To: target}
}

// detail is the state of the Commits, Files and Diff panes, which show the
// repository under the Repos cursor.
type detail struct {
	tab Tab
	// token names the latest load generation; an answer that carries
	// another token is stale and dropped.
	token   int
	commits commitsState
	files   filesState
	diff    diffState
}

// commitsState is the Commits pane.
type commitsState struct {
	loading bool
	err     error
	// commits and total are the range of the Incoming and Base tabs: its
	// newest commits and how many there are in all.
	commits []repo.Commit
	total   int
	// items are the rows of the Branches tab.
	items []branchItem
	// cursor indexes the entries: "All …" first, then commits; the single
	// Local entry; or items. top is the first entry shown.
	cursor, top int
}

// filesState is the Files pane: the files of the entry under the Commits
// cursor, or in the Branches tab the commits of the selected row that exist
// only here.
type filesState struct {
	loading bool
	// ready is set once the files were listed.
	ready bool
	err   error
	spec  repo.DiffSpec
	files []repo.FileChange
	// list is files as the pane draws them, computed when they arrived.
	list    fileList
	commits []repo.Commit
	cursor  int
	top     int
}

// detailSubject is what the detail panes show: one tab of one repository.
type detailSubject struct {
	id     repo.ID
	loaded bool
	tab    Tab
}

func (m Model) detailSubject() detailSubject {
	r, ok := m.cursorRow()
	if !ok {
		return detailSubject{tab: m.detail.tab}
	}

	return detailSubject{id: r.snap.Loc.ID, loaded: r.loaded, tab: m.detail.tab}
}

// detailSnapshot returns the repository the detail panes show, once it was
// read.
func (m Model) detailSnapshot() (drift.Snapshot, bool) {
	r, ok := m.cursorRow()
	if !ok || !r.loaded {
		return drift.Snapshot{}, false
	}

	return r.snap, true
}

// reload starts a new load generation: answers still in flight become stale,
// and the panes waiting for data load once the debounce delay passes without
// another reload.
func (m *Model) reload() tea.Cmd {
	m.detail.token++
	token := m.detail.token

	return tea.Tick(detailDebounce, func(time.Time) tea.Msg { return detailTickMsg{token: token} })
}

// resetDetail starts the panes over for a new repository, tab or snapshot.
func (m *Model) resetDetail() tea.Cmd {
	d := &m.detail
	d.commits, d.files, d.diff = commitsState{}, filesState{}, diffState{}

	s, ok := m.detailSnapshot()
	if !ok {
		return m.reload()
	}

	switch d.tab {
	case TabIncoming, TabBase:
		if _, ok := rangeTarget(s, d.tab); ok {
			d.commits.loading = true

			m.selectEntry() // the "All …" entry
		}
	case TabLocal:
		if localChanges(s.Status) > 0 {
			m.selectEntry()
		}
	case TabBranches:
		d.commits.items = branchItems(s.Work, s.Status.Branch, m.now())
		d.commits.cursor, _ = nearestSelectable(d.commits.items, 0, 1)

		m.selectEntry()
	}

	return m.reload()
}

// selectEntry resets the Files and Diff panes for the entry under the
// Commits cursor. A stash row needs no query: its only commit is the stash
// itself; neither does a branch with no commits only here.
func (m *Model) selectEntry() {
	d := &m.detail
	d.files, d.diff = filesState{}, diffState{}

	if d.tab != TabBranches {
		d.files.loading = true

		return
	}

	it, ok := m.cursorBranchItem()

	switch {
	case !ok:
		return
	case it.onlyHereCount() == 0:
		d.files.ready = true // nothing to list, and no query to make
	case it.kind == branchStash && it.stash != nil:
		d.files = filesState{
			ready:   true,
			spec:    repo.StashSpec(*it.stash),
			commits: []repo.Commit{stashCommit(*it.stash)},
		}

		m.selectFile()
	default:
		d.files.loading = true
	}
}

// selectFile resets the Diff pane for the entry under the Files cursor. A
// binary file shows a notice instead of a diff.
func (m *Model) selectFile() {
	d := &m.detail
	d.diff = diffState{}

	if d.tab == TabBranches {
		if c, ok := m.onlyHereCommit(); ok {
			d.diff = diffState{loading: true, title: commitShort(c), header: commitHeader(c)}
		}

		return
	}

	if f, ok := m.fileChange(); ok {
		d.diff = diffState{loading: !f.Binary, binary: f.Binary, title: sanitize(f.Path)}
	}
}

// fetchDetail queries the backend for every pane that waits for data and
// can be loaded now; the Diff pane waits for the Files pane.
func (m Model) fetchDetail() tea.Cmd {
	s, ok := m.detailSnapshot()
	if !ok {
		return nil
	}

	var cmds []tea.Cmd

	if m.detail.commits.loading {
		if target, ok := rangeTarget(s, m.detail.tab); ok {
			cmds = append(cmds, m.loader(s.Loc).commits(m.ctx, "HEAD", target))
		}
	}

	switch {
	case m.detail.files.loading:
		cmds = append(cmds, m.filesCmd(s))
	case m.detail.diff.loading:
		cmds = append(cmds, m.diffCmd(s.Loc))
	}

	return tea.Batch(cmds...)
}

func (m Model) loader(loc repo.Location) detailLoader {
	return detailLoader{backend: m.backend, loc: loc, token: m.detail.token}
}

// filesCmd lists what the Files pane shows for the entry under the Commits
// cursor.
func (m Model) filesCmd(s drift.Snapshot) tea.Cmd {
	l := m.loader(s.Loc)
	c := m.detail.commits

	switch m.detail.tab {
	case TabIncoming, TabBase:
		if i := c.cursor - 1; i >= 0 && i < len(c.commits) {
			return l.commitFiles(m.ctx, c.commits[i].SHA)
		}

		target, _ := rangeTarget(s, m.detail.tab)

		return l.files(m.ctx, allSpec(target))
	case TabLocal:
		return l.files(m.ctx, repo.DiffSpec{Kind: repo.DiffWorktree})
	case TabBranches:
		if it, ok := m.cursorBranchItem(); ok {
			return l.onlyHere(m.ctx, it.ref)
		}
	}

	return nil
}

// diffCmd loads the diff of the entry under the Files cursor: one file of the
// listed comparison, or a whole commit in the Branches tab.
func (m Model) diffCmd(loc repo.Location) tea.Cmd {
	l := m.loader(loc)

	if m.detail.tab != TabBranches {
		if f, ok := m.fileChange(); ok {
			return l.diff(m.ctx, m.detail.files.spec, f.Path, f.OldPath)
		}

		return nil
	}

	if it, ok := m.cursorBranchItem(); ok && it.kind == branchStash {
		return l.diff(m.ctx, m.detail.files.spec, "", "")
	}

	if c, ok := m.onlyHereCommit(); ok {
		return l.commitDiff(m.ctx, c.SHA)
	}

	return nil
}

// updateDetail applies the debounce tick and the answers of the backend.
// Messages from an older load generation change nothing.
func (m *Model) updateDetail(msg tea.Msg) tea.Cmd {
	d := &m.detail

	switch msg := msg.(type) {
	case detailTickMsg:
		if msg.token == d.token {
			return m.fetchDetail()
		}
	case commitsLoadedMsg:
		if msg.token == d.token {
			d.commits.loading, d.commits.err = false, msg.err
			d.commits.commits, d.commits.total = msg.commits, msg.total
		}
	case filesLoadedMsg:
		if msg.token == d.token {
			d.files = filesState{
				ready: msg.err == nil, err: msg.err, spec: msg.spec, files: msg.files,
				list: newFileList(msg.files, m.fileRules()),
			}

			return m.loadFirstDiff()
		}
	case onlyHereLoadedMsg:
		if msg.token == d.token {
			d.files = filesState{ready: msg.err == nil, err: msg.err, commits: msg.commits}

			return m.loadFirstDiff()
		}
	case diffLoadedMsg:
		if msg.token == d.token {
			d.diff = d.diff.loaded(msg)
		}
	}

	return nil
}

// loadFirstDiff shows the diff of the first listed entry right away: the
// Files pane was just loaded, the user did not move.
func (m *Model) loadFirstDiff() tea.Cmd {
	m.selectFile()

	s, ok := m.detailSnapshot()
	if !ok || !m.detail.diff.loading {
		return nil
	}

	return m.diffCmd(s.Loc)
}

// syncDetailScroll keeps the Commits and Files cursors in view and the Diff
// scroll offset in range for the current layout.
func (m *Model) syncDetailScroll(lay Layout) {
	if h := innerHeight(lay.Commits); h > 0 {
		n, hidden := m.commitEntries()
		c := &m.detail.commits
		c.top, _, _ = listWindow(n, hidden, c.cursor, c.top, h)
	}

	if h := innerHeight(lay.Files); h > 0 {
		n, hidden := m.fileEntries()
		f := &m.detail.files
		f.top, _, _ = listWindow(n, hidden, f.cursor, f.top, h)
	}

	if h := innerHeight(lay.Diff); h > 0 {
		m.detail.diff.scroll = min(m.detail.diff.scroll, m.detail.diff.maxScroll(h))
	}
}

// innerHeight is the number of lines inside the border of a pane.
func innerHeight(r Rect) int { return max(r.H-2, 0) }

// listWindow returns the entries [start, end) of a list of n entries to show
// in capacity lines around cursor, moving as little as possible from top,
// and how many remain below them: the rest of the list plus hidden entries
// that were not loaded.
func listWindow(n, hidden, cursor, top, capacity int) (start, end, more int) {
	total := n
	if hidden > 0 {
		total++ // the "… N more" line always follows the loaded entries
	}

	start, end = scrollWindow(total, cursor, top, capacity)
	end = min(end, n)

	return start, end, n - end + hidden
}
