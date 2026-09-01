package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExpandPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"absolute", "/tmp/a.txt", "/tmp/a.txt", false},
		{"tilde alone", "~", home, false},
		{"tilde rooted", "~/Downloads/a.txt", filepath.Join(home, "Downloads/a.txt"), false},
		{"surrounding space", "  /tmp/a.txt  ", "/tmp/a.txt", false},
		{"uncleaned", "/tmp//sub/../a.txt", "/tmp/a.txt", false},
		{"env var stays literal", "$HOME/a.txt", "$HOME/a.txt", false},
		{"tilde mid-path stays literal", "/tmp/~a.txt", "/tmp/~a.txt", false},
		{"empty", "   ", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandPath(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("expandPath(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("expandPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveDest(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "taken.mp3")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("fresh path", func(t *testing.T) {
		dest, confirm, err := resolveDest(filepath.Join(dir, "new.mp3"), "obj.mp3")
		if err != nil || confirm {
			t.Fatalf("resolveDest = (%q, %v, %v), want a clean path", dest, confirm, err)
		}
		if dest != filepath.Join(dir, "new.mp3") {
			t.Errorf("dest = %q, want the typed path", dest)
		}
	})

	t.Run("existing directory gets the object name", func(t *testing.T) {
		dest, confirm, err := resolveDest(dir, "obj.mp3")
		if err != nil || confirm {
			t.Fatalf("resolveDest = (%q, %v, %v), want a clean path", dest, confirm, err)
		}
		if want := filepath.Join(dir, "obj.mp3"); dest != want {
			t.Errorf("dest = %q, want %q", dest, want)
		}
	})

	t.Run("existing file needs confirmation", func(t *testing.T) {
		dest, confirm, err := resolveDest(existing, "obj.mp3")
		if err != nil {
			t.Fatalf("resolveDest error = %v", err)
		}
		if !confirm {
			t.Errorf("resolveDest(%q) confirm = false, want true (would overwrite)", existing)
		}
		if dest != existing {
			t.Errorf("dest = %q, want %q", dest, existing)
		}
	})

	t.Run("missing parent directory errors", func(t *testing.T) {
		_, _, err := resolveDest(filepath.Join(dir, "nope", "a.mp3"), "obj.mp3")
		if err == nil {
			t.Fatal("resolveDest with a missing parent returned no error")
		}
	})

	t.Run("empty errors", func(t *testing.T) {
		if _, _, err := resolveDest("", "obj.mp3"); err == nil {
			t.Fatal("resolveDest(\"\") returned no error")
		}
	})
}

func TestDefaultDownloadPath(t *testing.T) {
	t.Run("XDG_DOWNLOAD_DIR wins", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_DOWNLOAD_DIR", dir)
		if got, want := defaultDownloadPath("a.mp3"), filepath.Join(dir, "a.mp3"); got != want {
			t.Errorf("defaultDownloadPath = %q, want %q", got, want)
		}
	})

	t.Run("~/Downloads when it exists", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("XDG_DOWNLOAD_DIR", "")
		t.Setenv("HOME", home)
		dl := filepath.Join(home, "Downloads")
		if err := os.Mkdir(dl, 0o755); err != nil {
			t.Fatal(err)
		}
		if got, want := defaultDownloadPath("a.mp3"), filepath.Join(dl, "a.mp3"); got != want {
			t.Errorf("defaultDownloadPath = %q, want %q", got, want)
		}
	})

	t.Run("home when ~/Downloads is missing", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("XDG_DOWNLOAD_DIR", "")
		t.Setenv("HOME", home)
		if got, want := defaultDownloadPath("a.mp3"), filepath.Join(home, "a.mp3"); got != want {
			t.Errorf("defaultDownloadPath = %q, want %q", got, want)
		}
	})
}

func TestPercentOf(t *testing.T) {
	tests := []struct {
		written, total int64
		want           int
	}{
		{0, 100, 0},
		{1, 100, 1},
		{42, 100, 42},
		{99, 100, 99},
		{100, 100, 100},
		{150, 100, 100}, // never over 100
		{99999, 100000, 99},
		{50, 0, 0},  // unknown total
		{-5, 10, 0}, // nonsense input
	}
	for _, tt := range tests {
		if got := percentOf(tt.written, tt.total); got != tt.want {
			t.Errorf("percentOf(%d, %d) = %d, want %d", tt.written, tt.total, got, tt.want)
		}
	}
}

func TestFillCells(t *testing.T) {
	const w = 16
	tests := []struct {
		name           string
		written, total int64
		want           int
	}{
		{"zero", 0, 100, 0},
		{"one percent rounds to nothing", 1, 100, 0},
		{"42 percent", 42, 100, 7},
		{"half", 50, 100, 8},
		{"99 percent never fills the track", 99, 100, 15},
		{"complete", 100, 100, w},
		{"unknown total", 50, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fillCells(tt.written, tt.total, w)
			if got != tt.want {
				t.Errorf("fillCells(%d, %d, %d) = %d, want %d", tt.written, tt.total, w, got, tt.want)
			}
			if got < 0 || got > w {
				t.Errorf("fillCells = %d, out of the [0,%d] track", got, w)
			}
		})
	}
}

func TestFormatETA(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "—"},
		{-time.Second, "—"},
		{12 * time.Second, "12s"},
		{59 * time.Second, "59s"},
		{63 * time.Second, "1m03s"},
		{59*time.Minute + 59*time.Second, "59m59s"},
		{2*time.Hour + 5*time.Minute, "2h05m"},
	}
	for _, tt := range tests {
		if got := formatETA(tt.in); got != tt.want {
			t.Errorf("formatETA(%s) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTransferAbortWithoutCancelFunc(t *testing.T) {
	// The transfer bar is rendered from a transfer built without a cancel func
	// in tests; abort must not panic on one.
	tr := newTransfer("a.mp3", nil)
	tr.abort()
	if !tr.cancelled.Load() {
		t.Error("abort() did not mark the transfer cancelled")
	}
}

func TestTransferNote(t *testing.T) {
	tr := newTransfer("a.mp3", nil)
	tr.note(512, 2048)
	if got, want := tr.written.Load(), int64(512); got != want {
		t.Errorf("written = %d, want %d", got, want)
	}
	if got, want := tr.total.Load(), int64(2048); got != want {
		t.Errorf("total = %d, want %d", got, want)
	}
}
