package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jackjakarta/anchr/s3client"
)

func runeKey(s string) tea.KeyMsg      { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func special(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

// downloadTestModel is testModel with the cursor parked on a file and a
// (never-dialled) client, so the download keys have something to act on.
func downloadTestModel(t *testing.T) Model {
	t.Helper()
	m := testModel()
	m.width, m.height = 100, 30
	m.clients = []*s3client.Client{nil} // the cmds are inspected, never run
	m.browser.cursor = 2                // past "../" and the directory
	m.updateLayout()
	if item, ok := m.browser.selectedItem(); !ok || item.IsDir {
		t.Fatalf("setup: cursor is not on a file (%+v)", item)
	}
	return m
}

// step drives one key through the root and returns the updated model.
func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want ui.Model", next)
	}
	return got, cmd
}

func TestDownloadFlow(t *testing.T) {
	dir := t.TempDir()
	m := downloadTestModel(t)
	item, _ := m.browser.selectedItem()

	// D opens the prompt rather than starting any I/O.
	m, cmd := step(t, m, runeKey("D"))
	if !m.savePrompt.active {
		t.Fatal("D did not open the save prompt")
	}
	if cmd != nil {
		t.Error("D returned a command; it should only open the prompt")
	}
	if m.savePrompt.name != item.Name || m.savePrompt.key != item.Key {
		t.Errorf("prompt opened for %q/%q, want %q/%q",
			m.savePrompt.name, m.savePrompt.key, item.Name, item.Key)
	}

	// Keys that are browser actions outside the prompt are just text inside it.
	before := m.savePrompt.input.Value()
	m, _ = step(t, m, runeKey("q"))
	m, _ = step(t, m, runeKey("s"))
	if got := m.savePrompt.input.Value(); got != before+"qs" {
		t.Errorf("input = %q, want the browser-action keys typed into it (%q)", got, before+"qs")
	}

	// A real destination, then enter starts the transfer.
	m.savePrompt.input.SetValue(filepath.Join(dir, "obj.bin"))
	m, cmd = step(t, m, special(tea.KeyEnter))
	if m.savePrompt.active {
		t.Error("enter left the prompt open on a valid path")
	}
	if m.transfer == nil {
		t.Fatal("enter did not start a transfer")
	}
	if cmd == nil {
		t.Error("enter returned no command; the download and its progress poll should both be queued")
	}
	if !m.transferBarVisible() {
		t.Error("the transfer bar is not visible during a download")
	}

	// A progress sample fills in the percentage, rate and ETA.
	m.transfer.started = time.Now().Add(-2 * time.Second)
	m, cmd = step(t, m, DownloadProgressMsg{Written: 1 << 20, Total: 4 << 20})
	if m.dlWritten != 1<<20 || m.dlTotal != 4<<20 {
		t.Errorf("sample stored as (%d, %d), want (%d, %d)", m.dlWritten, m.dlTotal, 1<<20, 4<<20)
	}
	if m.dlRate <= 0 || m.dlETA <= 0 {
		t.Errorf("rate = %f, eta = %s; want both positive after a sample", m.dlRate, m.dlETA)
	}
	if cmd == nil {
		t.Error("the progress handler did not re-arm the poll")
	}
	if !strings.Contains(m.renderTransferBar(), "25%") {
		t.Errorf("transfer bar does not show the percentage:\n%s", m.renderTransferBar())
	}

	// ctrl+x aborts.
	m, _ = step(t, m, special(tea.KeyCtrlX))
	if !m.transfer.cancelled.Load() {
		t.Fatal("ctrl+x did not mark the transfer cancelled")
	}

	// The download cmd reports back; the bar goes away and the status explains.
	m, _ = step(t, m, FileDownloadedMsg{DestPath: filepath.Join(dir, "obj.bin"), Cancelled: true})
	if m.transfer != nil || m.transferBarVisible() {
		t.Error("the transfer outlived its FileDownloadedMsg")
	}
	if m.status != "Download cancelled" {
		t.Errorf("status = %q, want %q", m.status, "Download cancelled")
	}
}

func TestDownloadPromptEscapes(t *testing.T) {
	m := downloadTestModel(t)
	m, _ = step(t, m, runeKey("D"))
	m, cmd := step(t, m, special(tea.KeyEsc))
	if m.savePrompt.active {
		t.Error("esc did not close the save prompt")
	}
	if m.transfer != nil || cmd != nil {
		t.Error("esc started a download")
	}
}

func TestDownloadPromptKeepsBadPathOpen(t *testing.T) {
	m := downloadTestModel(t)
	m, _ = step(t, m, runeKey("D"))
	m.savePrompt.input.SetValue(filepath.Join(t.TempDir(), "missing-dir", "obj.bin"))

	m, cmd := step(t, m, special(tea.KeyEnter))
	if !m.savePrompt.active {
		t.Error("enter on an unusable path closed the prompt")
	}
	if m.savePrompt.err == "" {
		t.Error("enter on an unusable path showed no inline error")
	}
	if m.transfer != nil || cmd != nil {
		t.Error("enter on an unusable path started a download")
	}
}

func TestDownloadRefusesWhileOneIsRunning(t *testing.T) {
	m := downloadTestModel(t)
	m.transfer = newTransfer("busy.bin", nil)

	m, _ = step(t, m, runeKey("D"))
	if m.savePrompt.active {
		t.Error("D opened a second save prompt during a download")
	}
	if m.status == "" {
		t.Error("D during a download said nothing")
	}
}

func TestProgressSampleAfterCompletionIsDropped(t *testing.T) {
	m := downloadTestModel(t)
	m, cmd := step(t, m, DownloadProgressMsg{Written: 10, Total: 20})
	if cmd != nil {
		t.Error("a sample with no transfer in flight re-armed the poll")
	}
	if m.dlWritten != 0 {
		t.Errorf("dlWritten = %d, want the stale sample dropped", m.dlWritten)
	}
}
