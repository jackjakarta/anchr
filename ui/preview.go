package ui

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// previewMaxBytes is how much of an object is fetched for the preview popup.
const previewMaxBytes = 64 * 1024 // 64 KB

// preview is the centered popup that shows the first chunk of an object. Like
// sidebar/browser it is a plain struct with methods (not a nested tea.Model);
// pointer receivers mutate, the root Model holds it by value.
type preview struct {
	active  bool
	loading bool
	binary  bool
	title   string   // object Name shown in the box header
	lines   []string // content split into lines (text, or a metadata message)
	err     error
	scroll  int // index of the top visible line
}

func (p *preview) open(title string) {
	p.active = true
	p.loading = true
	p.binary = false
	p.title = title
	p.lines = nil
	p.err = nil
	p.scroll = 0
}

func (p *preview) close() {
	p.active = false
}

func (p *preview) setError(err error) {
	p.loading = false
	p.err = err
}

func (p *preview) setContent(data []byte, contentType string) {
	p.loading = false
	p.err = nil

	if isBinary(data) {
		p.binary = true
		ct := contentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		msg := fmt.Sprintf(
			"Binary file — preview unavailable.\n\nContent-Type: %s\nFetched %d bytes.",
			ct, len(data),
		)
		p.lines = strings.Split(msg, "\n")
		return
	}

	p.binary = false
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\t", "    ")
	p.lines = strings.Split(text, "\n")
}

func (p *preview) scrollUp() {
	if p.scroll > 0 {
		p.scroll--
	}
}

func (p *preview) scrollDown(height int) {
	if max := p.maxScroll(height); p.scroll < max {
		p.scroll++
	}
}

func (p *preview) maxScroll(height int) int {
	max := len(p.lines) - previewBodyRows(height)
	if max < 0 {
		max = 0
	}
	return max
}

// View renders the popup centered within the given content area. spinnerView is
// the root's spinner (animated by the global tick) used while loading.
func (p preview) View(width, height int, spinnerView string) string {
	boxW := previewBoxWidth(width)
	boxH := previewBoxHeight(height)
	innerW := boxW - 4 // 2 border + 2 horizontal padding
	if innerW < 8 {
		innerW = 8
	}

	var b strings.Builder

	title := p.title
	if title == "" {
		title = "preview"
	}
	b.WriteString(previewTitle.Render(truncate(title, innerW)))
	b.WriteString("\n\n")

	switch {
	case p.loading:
		b.WriteString(fmt.Sprintf("%s Loading preview…", spinnerView))
	case p.err != nil:
		b.WriteString(errorStyle.Render(truncate(fmt.Sprintf("Error: %s", p.err), innerW)))
	default:
		rows := previewBodyRows(height)
		start := p.scroll
		if start > len(p.lines) {
			start = len(p.lines)
		}
		end := start + rows
		if end > len(p.lines) {
			end = len(p.lines)
		}
		for i := start; i < end; i++ {
			line := truncate(p.lines[i], innerW)
			if p.binary {
				line = previewMeta.Render(line)
			}
			b.WriteString(line)
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	box := previewBox.Width(boxW).Height(boxH).Render(b.String())
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// previewBoxWidth/Height size the popup relative to the screen, clamped so it
// stays readable on small terminals and not absurdly wide on large ones.
func previewBoxWidth(width int) int {
	w := width - 8
	if w > 100 {
		w = 100
	}
	if w < 24 {
		w = 24
	}
	return w
}

func previewBoxHeight(height int) int {
	h := height - 4
	if h > 30 {
		h = 30
	}
	if h < 6 {
		h = 6
	}
	return h
}

// previewBodyRows is how many content lines fit inside the box:
// box height − 2 border rows − 2 header rows (title + blank).
func previewBodyRows(height int) int {
	rows := previewBoxHeight(height) - 4
	if rows < 1 {
		rows = 1
	}
	return rows
}

// isBinary reports whether data looks non-textual (invalid UTF-8 or a NUL byte).
func isBinary(data []byte) bool {
	return !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0
}

// truncate shortens s to at most max runes, appending an ellipsis when cut.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return string(r[:1])
	}
	return string(r[:max-1]) + "…"
}
