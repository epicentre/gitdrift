package ui

import "charm.land/bubbles/v2/key"

// keyMap binds every key the UI understands.
type keyMap struct {
	Up, Down, PageUp, PageDown, Home, End key.Binding

	NextPane, PrevPane, FocusPane, Enter, Back key.Binding
	PrevTab, NextTab, Zoom                     key.Binding

	Select, Pull, PullAll, Refresh, RefreshAll key.Binding
	Base, Delete, Filter, Help, Quit           key.Binding

	// Dialog keys. Each dialog names its keys in its footer, so the help
	// leaves them out. Erase and the arrows serve the base picker, where
	// letters type into the filter.
	Confirm, Cancel, Erase, ArrowUp, ArrowDown key.Binding
	// Interrupt quits even while a dialog is open; q does not.
	Interrupt key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "move up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "move down")),
		PageUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdown", "page down")),
		Home:     key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g/home", "go to top")),
		End:      key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G/end", "go to bottom")),

		NextPane:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next pane")),
		PrevPane:  key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "previous pane")),
		FocusPane: key.NewBinding(key.WithKeys("1", "2", "3", "4"), key.WithHelp("1-4", "focus pane")),
		Enter:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open in the next pane")),
		Back:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back or leave zoom")),
		PrevTab:   key.NewBinding(key.WithKeys("["), key.WithHelp("[", "previous commits tab")),
		NextTab:   key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next commits tab")),
		Zoom:      key.NewBinding(key.WithKeys("z"), key.WithHelp("z", "zoom pane")),

		Select:     key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "select for pull")),
		Pull:       key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pull selected or current repo")),
		PullAll:    key.NewBinding(key.WithKeys("P"), key.WithHelp("P", "pull all safe repos")),
		Refresh:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh current repo")),
		RefreshAll: key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh all repos")),
		Base:       key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "choose base branch")),
		Delete:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete gone branches")),
		Filter:     key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "only-here filter")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),

		Confirm:   key.NewBinding(key.WithKeys("y")),
		Cancel:    key.NewBinding(key.WithKeys("n", "esc")),
		Erase:     key.NewBinding(key.WithKeys("backspace")),
		ArrowUp:   key.NewBinding(key.WithKeys("up")),
		ArrowDown: key.NewBinding(key.WithKeys("down")),
		Interrupt: key.NewBinding(key.WithKeys("ctrl+c")),
	}
}

// actions lists the keys that open a window or change what the dashboard
// shows.
func (k keyMap) actions() []key.Binding {
	return []key.Binding{k.Help, k.Select, k.Pull, k.PullAll, k.Delete, k.Base, k.Refresh, k.RefreshAll}
}

// listKeys is the key map whose navigation keys type no text: the arrows,
// pgup, pgdown, home and end. Lists with a text filter navigate with it.
func (k keyMap) listKeys() keyMap {
	k.Up, k.Down = k.ArrowUp, k.ArrowDown
	k.Home = key.NewBinding(key.WithKeys("home"))
	k.End = key.NewBinding(key.WithKeys("end"))

	return k
}

// ShortHelp lists the bindings of the one-line help.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Pull, k.PullAll, k.Refresh, k.Zoom, k.NextPane, k.Help, k.Quit}
}

// FullHelp lists every binding, grouped in columns.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.Home, k.End},
		{k.NextPane, k.PrevPane, k.FocusPane, k.Enter, k.Back, k.PrevTab, k.NextTab, k.Zoom},
		{k.Select, k.Pull, k.PullAll, k.Refresh, k.RefreshAll},
		{k.Base, k.Delete, k.Filter, k.Help, k.Quit},
	}
}
