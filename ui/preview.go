package ui

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// previewMaxBytes is how much of an object is fetched for the preview popup.
// Images get a much larger budget: 64 KB is a couple of scanlines of a photo,
// and a partial image decodes to nothing useful.
const (
	previewMaxBytes      = 64 * 1024        // 64 KB, text and everything else
	imagePreviewMaxBytes = 10 * 1024 * 1024 // 10 MB
)

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

	img *previewImage // non-nil once the payload is known to be an image
}

// previewImage is the image half of the popup's state. rows is pre-rendered by
// renderImage off the UI thread — either halfblock cells or kitty placeholder
// cells — and View prints it verbatim.
type previewImage struct {
	data      []byte   // raw bytes, kept so a resize can re-render without refetching
	rows      []string // rendered cell rows, each exactly cols wide
	cols      int
	srcW      int // intrinsic pixel dimensions, shown in the caption
	srcH      int
	format    string // "png", "jpeg", "gif"
	truncated bool   // the fetch hit imagePreviewMaxBytes, so data is a prefix
	backend   imageBackend
	rendering bool
	err       error
}

func (p *preview) open(title string) {
	p.active = true
	p.loading = true
	p.binary = false
	p.title = title
	p.lines = nil
	p.err = nil
	p.scroll = 0
	p.img = nil
}

func (p *preview) close() {
	p.active = false
	p.img = nil
}

// hasImage reports whether the popup is showing (or preparing) an image.
func (p preview) hasImage() bool { return p.img != nil }

func (p *preview) setError(err error) {
	p.loading = false
	p.err = err
	p.img = nil
}

// setImageRows installs the rendered cell rows for the current image.
func (p *preview) setImageRows(rows []string, cols int, err error) {
	if p.img == nil {
		return // the popup moved on while the render was in flight
	}
	p.img.rendering = false
	p.img.rows = rows
	p.img.cols = cols
	p.img.err = err
}

func (p *preview) setContent(data []byte, contentType string) {
	p.loading = false
	p.err = nil

	// Images are decided by magic bytes, not by the object name, so an
	// extension-less or mislabelled object still previews. The actual decode
	// and scale happen in a tea.Cmd; this only records that one is coming.
	if w, h, format, ok := imageConfig(data); ok {
		p.binary = false
		p.lines = nil
		p.img = &previewImage{
			data:      data,
			srcW:      w,
			srcH:      h,
			format:    format,
			truncated: len(data) >= imagePreviewMaxBytes,
			backend:   detectImageBackend(),
			rendering: true,
		}
		return
	}

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
	boxW, boxH := previewBoxWidth(width), previewBoxHeight(height)
	if p.hasImage() {
		boxW, boxH = imageBoxWidth(width), imageBoxHeight(height)
	}
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
	case p.img != nil:
		b.WriteString(p.renderImageBody(innerW, imageBodyRows(height), spinnerView))
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
	return lipgloss.Place(
		width, height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceBackground(cAppBg),
	)
}

// renderImageBody renders the image half of the popup: the pre-rendered cell
// rows centered in the box, with a dim caption underneath. The rows are written
// verbatim — deliberately *not* through truncate, which counts runes and would
// slice a multi-byte SGR run or a kitty APC payload in half. They are already
// sized to fit by renderImage.
func (p preview) renderImageBody(innerW, bodyRows int, spinnerView string) string {
	img := p.img

	switch {
	case img.backend == backendNone:
		return previewMeta.Render(truncate("Image preview disabled (ANCHR_IMAGE_BACKEND=none).", innerW))
	case img.rendering:
		return fmt.Sprintf("%s Rendering image…", spinnerView)
	case img.err != nil && img.truncated:
		return previewMeta.Render(truncate(fmt.Sprintf(
			"Image is larger than the %d MB preview limit — only the first %d MB was fetched, which is not enough to decode it.",
			imagePreviewMaxBytes>>20, imagePreviewMaxBytes>>20), innerW))
	case img.err != nil && errors.Is(img.err, errUnsupportedImage):
		return previewMeta.Render(truncate(fmt.Sprintf(
			"%s images cannot be decoded — anchr reads PNG, JPEG and GIF.",
			strings.ToUpper(img.format)), innerW))
	case img.err != nil:
		return errorStyle.Render(truncate(fmt.Sprintf("Cannot render image: %s", img.err), innerW))
	case len(img.rows) == 0:
		return previewMeta.Render(truncate("Not enough room to render this image.", innerW))
	}

	// Rows are rasterised for one specific box. A resize changes the box before
	// the re-render lands, so for a frame or two the rows in hand are the wrong
	// size — and unlike text they cannot be truncated to fit, because slicing a
	// row would cut through an SGR run or a kitty payload. Printing them anyway
	// would overflow the popup and tear the whole frame, so hold the previous
	// state until the re-render arrives.
	if img.cols > innerW || len(img.rows) > bodyRows {
		return fmt.Sprintf("%s Rendering image…", spinnerView)
	}

	var b strings.Builder
	pad := (innerW - img.cols) / 2
	if pad < 0 {
		pad = 0
	}
	indent := darkBase.Render(strings.Repeat(" ", pad))

	for _, row := range img.rows {
		b.WriteString(indent)
		b.WriteString(row)
		b.WriteString("\n")
	}

	// One blank line, then the caption, if the box has room for them.
	if len(img.rows)+2 <= bodyRows {
		b.WriteString("\n")
		b.WriteString(previewMeta.Render(truncate(p.imageCaption(), innerW)))
	}
	return b.String()
}

// imageCaption is the dim line under the image: intrinsic size, format, and the
// backend in use so a user can tell halfblocks from real pixels at a glance.
func (p preview) imageCaption() string {
	img := p.img
	backend := "halfblocks"
	if img.backend == backendKitty {
		backend = "kitty graphics"
	}
	return fmt.Sprintf("%d×%d · %s · %s",
		img.srcW, img.srcH, strings.ToUpper(img.format), backend)
}

// imageBoxWidth/Height size the popup when it holds an image: more generous
// than the text popup, since a bigger box is simply a better picture. They are
// separate functions on purpose — previewBoxWidth/Height are shared with the
// save prompt, which must not grow.
func imageBoxWidth(width int) int {
	w := width - 6
	if w > 140 {
		w = 140
	}
	if w < 24 {
		w = 24
	}
	return w
}

func imageBoxHeight(height int) int {
	h := height - 2
	if h > 46 {
		h = 46
	}
	if h < 6 {
		h = 6
	}
	return h
}

// imageBodyRows is how many rows the image and its caption may occupy.
func imageBodyRows(height int) int {
	rows := imageBoxHeight(height) - 4 // 2 border + title + blank
	if rows < 1 {
		rows = 1
	}
	return rows
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
