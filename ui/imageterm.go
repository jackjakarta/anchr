package ui

import (
	"fmt"
	"image"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi/kitty"
)

// imageBackend is how the preview popup paints an image.
type imageBackend int

const (
	// backendHalfblock draws with ▀ cells (fg = top pixel, bg = bottom). Coarse
	// but made of ordinary runes, so it works in every terminal — iTerm2, Apple
	// Terminal, tmux, CI — and never confuses the frame diff.
	backendHalfblock imageBackend = iota

	// backendKitty uses the kitty graphics protocol with Unicode virtual
	// placements: real pixels, bound to placeholder cells that scroll and erase
	// like text. kitty, Ghostty and WezTerm.
	backendKitty

	// backendNone disables image rendering entirely (ANCHR_IMAGE_BACKEND=none).
	backendNone
)

// kittyImageID is the fixed image ID anchr transmits under. Only one preview is
// ever on screen, so a single slot is enough; it stays under 256 so the ID fits
// the 256-color foreground channel of a placeholder cell.
//
// Reusing one ID is also why no delete escape is needed on close: a virtual
// placement only paints where its placeholder cells are, so the image vanishes
// with the popup, and the next preview overwrites this same slot. Terminal-side
// memory is therefore bounded to a single image.
const kittyImageID = 31

// imageBackendEnv forces a backend, bypassing detection.
const imageBackendEnv = "ANCHR_IMAGE_BACKEND"

// detectImageBackend picks a backend from the environment. Bubble Tea v1 has no
// capability query, so this is env-sniffing rather than an escape-sequence
// probe; ANCHR_IMAGE_BACKEND is the escape hatch when the guess is wrong.
func detectImageBackend() imageBackend {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(imageBackendEnv))) {
	case "kitty":
		return backendKitty
	case "halfblock", "halfblocks":
		return backendHalfblock
	case "none", "off":
		return backendNone
	}

	// Inside tmux/screen the graphics escape would need DCS passthrough, and a
	// stale image can outlive a pane switch. Halfblocks are always correct.
	if os.Getenv("TMUX") != "" {
		return backendHalfblock
	}
	if term := os.Getenv("TERM"); strings.HasPrefix(term, "screen") || strings.HasPrefix(term, "tmux") {
		return backendHalfblock
	}

	if os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("TERM") == "xterm-kitty" {
		return backendKitty
	}
	if os.Getenv("GHOSTTY_RESOURCES_DIR") != "" || os.Getenv("GHOSTTY_BIN_DIR") != "" {
		return backendKitty
	}
	if os.Getenv("WEZTERM_PANE") != "" || os.Getenv("WEZTERM_EXECUTABLE") != "" {
		return backendKitty
	}

	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "ghostty", "wezterm":
		return backendKitty
	}

	// Everything else, Apple Terminal and iTerm2 included. iTerm2 has its own
	// inline-image protocol, but it draws from the cursor and consumes rows the
	// TUI must also emit, which cannot be reconciled with a fixed-height frame.
	return backendHalfblock
}

// kittyRows renders an image as kitty virtual placements: a transmission escape
// (zero display width) followed by rows of Unicode placeholder cells that the
// terminal fills with pixels. Each returned row is exactly cols display cells,
// so it drops into the popup exactly like a halfblock row.
//
// The escape rides on row 0 rather than being written to stdout out-of-band:
// Bubble Tea owns the output stream, and a concurrent write would interleave
// mid-frame. It is safe there because ansi.StringWidth treats an APC payload as
// zero width and ansi.Truncate leaves such a line untouched.
func kittyRows(img image.Image, id, cols, rows int) ([]string, error) {
	if cols <= 0 || rows <= 0 {
		return nil, nil
	}

	var buf strings.Builder
	err := kitty.EncodeGraphics(&buf, img, &kitty.Options{
		Action:           kitty.TransmitAndPut,
		ID:               id,
		Format:           kitty.PNG,
		Transmission:     kitty.Direct,
		Compression:      kitty.Zlib,
		VirtualPlacement: true,
		Columns:          cols,
		Rows:             rows,
		Chunk:            true,
		Quite:            2, // suppress the terminal's OK/error replies
	})
	if err != nil {
		return nil, err
	}

	// The placeholder's foreground carries the image ID; the two combining
	// diacritics carry the cell's row and column within the placement.
	idFg := fmt.Sprintf("\x1b[38;5;%dm", id)

	out := make([]string, rows)
	for r := 0; r < rows; r++ {
		var line strings.Builder
		if r == 0 {
			line.WriteString(buf.String())
		}
		line.WriteString(idFg)
		for c := 0; c < cols; c++ {
			line.WriteRune(kitty.Placeholder)
			line.WriteRune(kitty.Diacritic(r))
			line.WriteRune(kitty.Diacritic(c))
		}
		line.WriteString("\x1b[0m")
		out[r] = line.String()
	}
	return out, nil
}
