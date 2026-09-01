package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// savePromptRows is the popup's fixed content height: title, blank, input,
// blank, hint/warning.
const savePromptRows = 5

// savePrompt is the centered "Save as" popup opened by D. Like sidebar/browser/
// preview it is a plain struct with methods (not a nested tea.Model); pointer
// receivers mutate, the root Model holds it by value.
type savePrompt struct {
	active    bool
	name      string // object base name, for the header and directory defaults
	key       string // S3 key being downloaded
	clientIdx int
	input     textinput.Model
	confirm   bool   // destination exists; a second enter overwrites
	err       string // inline validation message
}

func newSavePrompt() savePrompt {
	ti := textinput.New()
	ti.Prompt = "▸ "
	// The popup paints its own background and inner styled segments emit their
	// own SGR reset, so every one of the input's styles has to carry it too.
	ti.PromptStyle = savePromptArrow
	ti.TextStyle = savePromptText
	ti.PlaceholderStyle = savePromptHint
	ti.Cursor.Style = savePromptText
	ti.Cursor.TextStyle = savePromptText
	// Static cursor: the widget never emits blink commands, so the root Update
	// needs no cursor.BlinkMsg case.
	_ = ti.Cursor.SetMode(cursor.CursorStatic)
	return savePrompt{input: ti}
}

func (s *savePrompt) open(clientIdx int, key, name string) {
	s.active = true
	s.clientIdx = clientIdx
	s.key = key
	s.name = name
	s.confirm = false
	s.err = ""
	s.input.SetValue(defaultDownloadPath(name))
	s.input.CursorEnd()
	_ = s.input.Focus()
}

func (s *savePrompt) close() {
	s.active = false
	s.input.Blur()
}

// update forwards a key to the text input. Any edit invalidates a pending
// overwrite confirmation, so a second enter can never overwrite a path other
// than the one the warning was about.
func (s *savePrompt) update(msg tea.Msg) tea.Cmd {
	before := s.input.Value()
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	if s.input.Value() != before {
		s.confirm = false
		s.err = ""
	}
	return cmd
}

// resolve validates the typed path. It returns the destination file only once
// any overwrite has been confirmed; otherwise it records the error or the
// pending confirmation and reports ok == false, so the popup stays open.
func (s *savePrompt) resolve() (string, bool) {
	dest, needsConfirm, err := resolveDest(s.input.Value(), s.name)
	if err != nil {
		s.err = err.Error()
		s.confirm = false
		return "", false
	}
	s.err = ""
	if needsConfirm && !s.confirm {
		s.confirm = true
		return "", false
	}
	return dest, true
}

// savePromptBoxWidth sizes the popup like the preview box but caps it narrower —
// it holds a single path, not a file.
func savePromptBoxWidth(width int) int {
	w := previewBoxWidth(width)
	if w > 72 {
		w = 72
	}
	return w
}

// View renders the popup centered within the content area, sized like the
// preview popup so the grid invariants hold. Every line is truncated to the box
// width — one wider line would soft-wrap and add a row.
func (s savePrompt) View(width, height int) string {
	boxW := savePromptBoxWidth(width)
	innerW := boxW - 4 // 2 border + 2 horizontal padding
	if innerW < 8 {
		innerW = 8
	}
	boxH := savePromptRows
	if maxH := height - 2; boxH > maxH {
		boxH = maxH
	}
	if boxH < 1 {
		boxH = 1
	}

	// The input renders prompt + Width cells + one cursor cell; MaxWidth is the
	// backstop for the viewport's off-by-one at the scroll edges.
	s.input.Width = innerW - lipgloss.Width(s.input.Prompt) - 1
	if s.input.Width < 4 {
		s.input.Width = 4
	}
	field := lipgloss.NewStyle().MaxWidth(innerW).Render(s.input.View())

	var foot string
	switch {
	case s.err != "":
		foot = savePromptErr.Render(truncate(s.err, innerW))
	case s.confirm:
		foot = savePromptWarn.Render(truncate("file exists — enter again to overwrite", innerW))
	default:
		foot = savePromptHint.Render(truncate("enter save · esc cancel · ^u clear", innerW))
	}

	lines := []string{
		savePromptTitle.Render(truncate("Save "+s.name, innerW)),
		"",
		field,
		"",
		foot,
	}
	if len(lines) > boxH {
		lines = lines[:boxH]
	}

	box := savePromptBox.Width(boxW).Height(boxH).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(
		width, height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceBackground(cAppBg),
	)
}
