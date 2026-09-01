package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestDetectImageBackend(t *testing.T) {
	// Every var the detector consults, cleared per-case so the developer's own
	// terminal (Apple Terminal here, Ghostty on someone else's box) can't leak in.
	vars := []string{
		imageBackendEnv, "TMUX", "TERM", "KITTY_WINDOW_ID",
		"GHOSTTY_RESOURCES_DIR", "GHOSTTY_BIN_DIR",
		"WEZTERM_PANE", "WEZTERM_EXECUTABLE", "TERM_PROGRAM",
	}

	tests := []struct {
		name string
		env  map[string]string
		want imageBackend
	}{
		{"bare terminal", nil, backendHalfblock},
		{"apple terminal", map[string]string{"TERM_PROGRAM": "Apple_Terminal", "TERM": "xterm-256color"}, backendHalfblock},
		{"iterm2 gets halfblocks", map[string]string{"TERM_PROGRAM": "iTerm.app"}, backendHalfblock},

		{"kitty via TERM", map[string]string{"TERM": "xterm-kitty"}, backendKitty},
		{"kitty via window id", map[string]string{"KITTY_WINDOW_ID": "1"}, backendKitty},
		{"ghostty via resources dir", map[string]string{"GHOSTTY_RESOURCES_DIR": "/x"}, backendKitty},
		{"ghostty via TERM_PROGRAM", map[string]string{"TERM_PROGRAM": "ghostty"}, backendKitty},
		{"wezterm via pane", map[string]string{"WEZTERM_PANE": "0"}, backendKitty},
		{"wezterm via TERM_PROGRAM", map[string]string{"TERM_PROGRAM": "WezTerm"}, backendKitty},

		// Multiplexers downgrade even when the outer terminal could do kitty.
		{"tmux downgrades kitty", map[string]string{"TMUX": "/tmp/x", "TERM": "xterm-kitty"}, backendHalfblock},
		{"screen TERM downgrades", map[string]string{"TERM": "screen-256color", "KITTY_WINDOW_ID": "1"}, backendHalfblock},

		// The override wins over everything, in both directions.
		{"override to kitty", map[string]string{imageBackendEnv: "kitty", "TERM_PROGRAM": "Apple_Terminal"}, backendKitty},
		{"override to halfblock", map[string]string{imageBackendEnv: "halfblock", "TERM": "xterm-kitty"}, backendHalfblock},
		{"override to none", map[string]string{imageBackendEnv: "none", "TERM": "xterm-kitty"}, backendNone},
		{"override beats tmux", map[string]string{imageBackendEnv: "kitty", "TMUX": "/tmp/x"}, backendKitty},
		{"override is case-insensitive", map[string]string{imageBackendEnv: " KiTTy "}, backendKitty},
		{"unknown override falls through", map[string]string{imageBackendEnv: "sixel", "TERM": "xterm-kitty"}, backendKitty},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range vars {
				t.Setenv(v, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := detectImageBackend(); got != tc.want {
				t.Errorf("detectImageBackend() = %d, want %d", got, tc.want)
			}
		})
	}
}

// The kitty rows sit in the popup body next to ordinary text, so they have to
// measure exactly like the halfblock rows do: the APC payload contributes no
// width, each placeholder cell contributes one.
func TestKittyRowsWidth(t *testing.T) {
	img := gradientImage(64, 32)

	for _, box := range []struct{ cols, rows int }{{96, 26}, {40, 10}, {7, 3}, {1, 1}} {
		out, err := kittyRows(img, kittyImageID, box.cols, box.rows)
		if err != nil {
			t.Fatalf("kittyRows: %v", err)
		}
		if len(out) != box.rows {
			t.Fatalf("got %d rows, want %d", len(out), box.rows)
		}
		for i, line := range out {
			if w := lipgloss.Width(line); w != box.cols {
				t.Errorf("box %dx%d row %d: width = %d, want %d", box.cols, box.rows, i, w, box.cols)
			}
		}
		// The transmission escape must ride on row 0 and only row 0.
		if !strings.Contains(out[0], "\x1b_G") {
			t.Errorf("box %dx%d: row 0 carries no transmission escape", box.cols, box.rows)
		}
		for i, line := range out[1:] {
			if strings.Contains(line, "\x1b_G") {
				t.Errorf("box %dx%d: row %d unexpectedly re-transmits", box.cols, box.rows, i+1)
			}
		}
	}
}

func TestKittyRowsEmptyBox(t *testing.T) {
	img := gradientImage(16, 16)
	for _, tc := range []struct{ cols, rows int }{{0, 5}, {5, 0}, {-1, -1}} {
		out, err := kittyRows(img, kittyImageID, tc.cols, tc.rows)
		if err != nil {
			t.Fatalf("kittyRows(%d,%d): %v", tc.cols, tc.rows, err)
		}
		if out != nil {
			t.Errorf("kittyRows(%d,%d) = %v, want nil", tc.cols, tc.rows, out)
		}
	}
}
