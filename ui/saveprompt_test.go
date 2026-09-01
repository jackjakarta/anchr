package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSavePromptOpenPrefillsAndCloses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DOWNLOAD_DIR", "")
	t.Setenv("HOME", home)

	s := newSavePrompt()
	s.open(2, "projects/files/mixdown_final.mp3", "mixdown_final.mp3")

	if !s.active {
		t.Fatal("open() did not activate the prompt")
	}
	if s.clientIdx != 2 || s.key != "projects/files/mixdown_final.mp3" {
		t.Errorf("open() kept (%d, %q), want the client index and key it was given", s.clientIdx, s.key)
	}
	if !strings.HasSuffix(s.input.Value(), "mixdown_final.mp3") {
		t.Errorf("prefilled value = %q, want it to end in the object name", s.input.Value())
	}
	if !s.input.Focused() {
		t.Error("open() left the input unfocused")
	}

	s.close()
	if s.active || s.input.Focused() {
		t.Error("close() left the prompt active or focused")
	}
}

func TestSavePromptOpenResetsPreviousState(t *testing.T) {
	s := newSavePrompt()
	s.open(0, "a", "a.mp3")
	s.confirm = true
	s.err = "stale"

	s.open(0, "b", "b.mp3")
	if s.confirm || s.err != "" {
		t.Errorf("open() kept confirm=%v err=%q, want both cleared", s.confirm, s.err)
	}
}

func TestSavePromptResolve(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "obj.mp3")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("clean path resolves at once", func(t *testing.T) {
		s := newSavePrompt()
		s.open(0, "k", "obj.mp3")
		s.input.SetValue(filepath.Join(dir, "fresh.mp3"))

		dest, ok := s.resolve()
		if !ok {
			t.Fatalf("resolve() = (%q, false), want ok", dest)
		}
		if dest != filepath.Join(dir, "fresh.mp3") {
			t.Errorf("dest = %q, want the typed path", dest)
		}
	})

	t.Run("existing file takes two enters", func(t *testing.T) {
		s := newSavePrompt()
		s.open(0, "k", "obj.mp3")
		s.input.SetValue(existing)

		if _, ok := s.resolve(); ok {
			t.Fatal("first resolve() on an existing file returned ok, want a confirmation step")
		}
		if !s.confirm {
			t.Fatal("first resolve() did not set confirm")
		}
		dest, ok := s.resolve()
		if !ok || dest != existing {
			t.Errorf("second resolve() = (%q, %v), want (%q, true)", dest, ok, existing)
		}
	})

	t.Run("bad path reports inline and stays open", func(t *testing.T) {
		s := newSavePrompt()
		s.open(0, "k", "obj.mp3")
		s.input.SetValue(filepath.Join(dir, "nope", "obj.mp3"))

		if _, ok := s.resolve(); ok {
			t.Fatal("resolve() with a missing parent returned ok")
		}
		if s.err == "" {
			t.Error("resolve() did not record an inline error")
		}
		if !s.active {
			t.Error("resolve() closed the prompt; it should stay open for a fix")
		}
	})
}

func TestSavePromptEditClearsConfirm(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "obj.mp3")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newSavePrompt()
	s.open(0, "k", "obj.mp3")
	s.input.SetValue(existing)
	if _, ok := s.resolve(); ok || !s.confirm {
		t.Fatal("setup: expected a pending overwrite confirmation")
	}

	// Typing after the warning must invalidate it, so the next enter cannot
	// overwrite a path other than the one the warning was about.
	s.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if s.confirm {
		t.Error("an edit left confirm set")
	}

	s.err = "stale"
	s.update(tea.KeyMsg{Type: tea.KeyBackspace})
	if s.err != "" {
		t.Error("an edit left a stale inline error")
	}
}

func TestSavePromptNonEditingKeyKeepsConfirm(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "obj.mp3")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newSavePrompt()
	s.open(0, "k", "obj.mp3")
	s.input.SetValue(existing)
	s.resolve()

	// Moving the cursor is not an edit.
	s.update(tea.KeyMsg{Type: tea.KeyLeft})
	if !s.confirm {
		t.Error("a cursor move cleared the pending confirmation")
	}
}
