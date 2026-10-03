package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	PgUp     key.Binding
	PgDown   key.Binding
	Home     key.Binding
	End      key.Binding
	Toggle   key.Binding
	View     key.Binding
	ReAdd    key.Binding
	Apply    key.Binding
	Undo     key.Binding
	Add      key.Binding
	OnlyMod  key.Binding
	Refresh  key.Binding
	Search   key.Binding
	SideMode key.Binding
	NextHunk key.Binding
	PrevHunk key.Binding
	Confirm  key.Binding
	Cancel   key.Binding
	Back     key.Binding
	Help     key.Binding
	Quit     key.Binding
	NextTab  key.Binding
	PrevTab  key.Binding

	SessionStart key.Binding
	KeepLive     key.Binding
	Revert       key.Binding
	SkipEntry    key.Binding
	BackEntry    key.Binding

	RelocateRepo key.Binding
}

var keys = keyMap{
	Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	PgUp:     key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("pgup", "page up")),
	PgDown:   key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("pgdn", "page down")),
	Home:     key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "top")),
	End:      key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "bottom")),
	Toggle:   key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "select")),
	View:     key.NewBinding(key.WithKeys("enter", "d", "s"), key.WithHelp("enter", "view diff")),
	ReAdd:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "re-add (live → source)")),
	Apply:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "apply (source → live)")),
	Undo:     key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "undo last action")),
	Add:      key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "add unmanaged → source")),
	OnlyMod:  key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "modified-only")),
	Refresh:  key.NewBinding(key.WithKeys("R", "ctrl+r"), key.WithHelp("R", "refresh")),
	Search:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	SideMode: key.NewBinding(key.WithKeys("U"), key.WithHelp("U", "toggle unified/side-by-side")),
	NextHunk: key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next hunk")),
	PrevHunk: key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev hunk")),
	Confirm:  key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y", "confirm")),
	Cancel:   key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n/esc", "cancel")),
	Back:     key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc", "back")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	NextTab:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
	PrevTab:  key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),

	SessionStart: key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "start sync session")),
	KeepLive:     key.NewBinding(key.WithKeys("k"), key.WithHelp("k", "keep live (re-add)")),
	Revert:       key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "revert (apply, with backup)")),
	SkipEntry:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "skip")),
	BackEntry:    key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "previous entry")),

	RelocateRepo: key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "re-detect dotfiles repo")),
}

// helpGroup is a titled cluster of keybindings shown on the Help tab.
type helpGroup struct {
	title string
	rows  [][2]string
}

// helpGroups returns the Help-tab contents. Bindings are referenced from the live keyMap so the displayed keys and descriptions cannot drift; a test asserts every binding in keyMap appears here.
func helpGroups() []helpGroup {
	b := func(binding key.Binding) [2]string {
		h := binding.Help()
		return [2]string{"  " + h.Key, h.Desc}
	}
	return []helpGroup{
		{"Movement", [][2]string{
			b(keys.Up), b(keys.Down), b(keys.PgUp), b(keys.PgDown), b(keys.Home), b(keys.End),
			{"  mouse wheel", "scroll list / diff"},
		}},
		{"Navigation", [][2]string{
			b(keys.NextTab), b(keys.PrevTab),
			{"  click tab", "switch tab"},
			b(keys.OnlyMod), b(keys.Search), b(keys.Help), b(keys.Back), b(keys.RelocateRepo),
		}},
		{"Selection", [][2]string{
			b(keys.Toggle),
			{"  click row", "move cursor"},
		}},
		{"Actions", [][2]string{
			b(keys.View), b(keys.ReAdd), b(keys.Apply), b(keys.Add), b(keys.Undo), b(keys.Refresh), b(keys.SessionStart),
		}},
		{"Diff", [][2]string{
			b(keys.SideMode), b(keys.NextHunk), b(keys.PrevHunk),
		}},
		{"Sync session", [][2]string{
			b(keys.KeepLive), b(keys.Revert), b(keys.SkipEntry), b(keys.BackEntry),
			b(keys.Confirm), b(keys.Cancel),
		}},
		{"Other", [][2]string{
			b(keys.Quit),
		}},
	}
}
