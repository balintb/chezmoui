package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	PgUp    key.Binding
	PgDown  key.Binding
	Home    key.Binding
	End     key.Binding
	Toggle  key.Binding
	View    key.Binding
	ReAdd   key.Binding
	OnlyMod key.Binding
	Refresh key.Binding
	Confirm key.Binding
	Cancel  key.Binding
	Back    key.Binding
	Help    key.Binding
	Quit    key.Binding
	NextTab key.Binding
	PrevTab key.Binding

	SessionStart key.Binding
	KeepLive     key.Binding
	Revert       key.Binding
	SkipEntry    key.Binding
	BackEntry    key.Binding

	RelocateRepo key.Binding
}

var keys = keyMap{
	Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	PgUp:    key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("pgup", "page up")),
	PgDown:  key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("pgdn", "page down")),
	Home:    key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "top")),
	End:     key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "bottom")),
	Toggle:  key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "select")),
	View:    key.NewBinding(key.WithKeys("enter", "d", "s"), key.WithHelp("enter", "view diff")),
	ReAdd:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "re-add")),
	OnlyMod: key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "modified-only")),
	Refresh: key.NewBinding(key.WithKeys("R", "ctrl+r"), key.WithHelp("R", "refresh")),
	Confirm: key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y", "confirm")),
	Cancel:  key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n/esc", "cancel")),
	Back:    key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "back")),
	Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	NextTab: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
	PrevTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),

	SessionStart: key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "start sync session")),
	KeepLive:     key.NewBinding(key.WithKeys("k"), key.WithHelp("k", "keep live (re-add)")),
	Revert:       key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "revert (apply, with backup)")),
	SkipEntry:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "skip")),
	BackEntry:    key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "previous entry")),

	RelocateRepo: key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "re-detect dotfiles repo")),
}
