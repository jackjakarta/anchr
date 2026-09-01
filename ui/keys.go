package ui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up          key.Binding
	Down        key.Binding
	Enter       key.Binding
	Back        key.Binding
	Tab         key.Binding
	Left        key.Binding
	Right       key.Binding
	Download    key.Binding
	CopyKey     key.Binding
	CopyURI     key.Binding
	PresignURL  key.Binding
	Preview     key.Binding
	Filter      key.Binding
	Sort        key.Binding
	SortReverse key.Binding
	Quit        key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter", "l"),
		key.WithHelp("enter/l", "open"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc", "backspace", "h"),
		key.WithHelp("esc/h", "back"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch pane"),
	),
	Left: key.NewBinding(
		key.WithKeys("left"),
		key.WithHelp("←", "sidebar"),
	),
	Right: key.NewBinding(
		key.WithKeys("right"),
		key.WithHelp("→", "browser"),
	),
	Download: key.NewBinding(
		key.WithKeys("D"), // Shift+D arrives as uppercase "D"
		key.WithHelp("D", "download"),
	),
	CopyKey: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "copy key"),
	),
	CopyURI: key.NewBinding(
		key.WithKeys("Y"), // Shift+y arrives as uppercase "Y"
		key.WithHelp("Y", "copy uri"),
	),
	PresignURL: key.NewBinding(
		key.WithKeys("u"),
		key.WithHelp("u", "presign url"),
	),
	Preview: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "preview"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	Sort: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "sort"),
	),
	SortReverse: key.NewBinding(
		key.WithKeys("S"), // Shift+s arrives as uppercase "S"
		key.WithHelp("S", "reverse"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}
