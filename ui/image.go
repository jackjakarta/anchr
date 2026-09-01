package ui

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"strings"

	// Registered for image.Decode. WebP/AVIF are deliberately absent: they would
	// pull in golang.org/x/image and anchr ships as a dependency-free binary.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/charmbracelet/lipgloss"
)

// halfBlock is the upper-half block: foreground paints the top pixel of the
// cell, background the bottom one, so a single cell carries two pixel rows.
const halfBlock = "▀"

// errUnsupportedImage is returned when the bytes decode to no format we know.
var errUnsupportedImage = errors.New("unsupported image format")

// previewBgRGBA is cDarkBg (#1d2021, the popup background) as a concrete color.
// It has to be spelled out rather than reused from the palette: lipgloss.Color
// resolves RGBA() through the global renderer's color profile, so off a TTY
// (tests, pipes) every palette color reports black and transparent pixels would
// composite onto the wrong ground. Keep this in sync with cDarkBg in styles.go.
var previewBgRGBA = color.RGBA{R: 0x1d, G: 0x20, B: 0x21, A: 0xff}

// decodeImage decodes PNG/JPEG/GIF (the formats registered above) and reports
// the format name alongside the image. Animated GIFs yield their first frame.
func decodeImage(data []byte) (image.Image, string, error) {
	if len(data) == 0 {
		return nil, "", errUnsupportedImage
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// image.Decode reports its own sentinel for "no registered format
		// matched"; anything else is a genuine decode failure worth surfacing.
		if errors.Is(err, image.ErrFormat) {
			return nil, "", errUnsupportedImage
		}
		return nil, "", err
	}
	return img, format, nil
}

// imageConfig sniffs just the header, so it is cheap enough to call on the UI
// thread to decide whether a payload is worth handing to the decode command.
// It keys off magic bytes rather than the object name, which means an
// extension-less or mislabelled object still previews correctly.
func imageConfig(data []byte) (w, h int, format string, ok bool) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, "", false
	}
	return cfg.Width, cfg.Height, format, true
}

// fitCells returns the largest cols×rows cell box that holds an imgW×imgH image
// at its original aspect ratio. A cell is one column wide and two pixel rows
// tall (see halfBlock), so the usable pixel grid is cols×(rows*2) and pixels
// come out roughly square.
func fitCells(imgW, imgH, maxCols, maxRows int) (cols, rows int) {
	if imgW <= 0 || imgH <= 0 || maxCols <= 0 || maxRows <= 0 {
		return 0, 0
	}

	// Scale to the width limit first, then pull back if that overflows height.
	cols = maxCols
	rows = (imgH * cols) / (imgW * 2)
	if rows > maxRows {
		rows = maxRows
		cols = (imgW * rows * 2) / imgH
	}

	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	if cols > maxCols {
		cols = maxCols
	}
	if rows > maxRows {
		rows = maxRows
	}
	return cols, rows
}

// scaleTo resamples src to exactly w×h. Downscaling box-averages every source
// pixel that lands in a destination cell (cheap, and much cleaner than dropping
// samples on the photos this is usually pointed at); upscaling falls out of the
// same loop as nearest-neighbor. Alpha is composited over bg so transparent
// PNGs sit on the popup background instead of turning black.
func scaleTo(src image.Image, w, h int, bg color.Color) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if w <= 0 || h <= 0 {
		return dst
	}

	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return dst
	}

	bgR, bgG, bgB, _ := bg.RGBA()

	for y := 0; y < h; y++ {
		// Source band [y0,y1) for this destination row, always non-empty.
		y0 := y * sh / h
		y1 := (y + 1) * sh / h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0 := x * sw / w
			x1 := (x + 1) * sw / w
			if x1 <= x0 {
				x1 = x0 + 1
			}

			var sumR, sumG, sumB, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					r, g, bl, a := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
					// Un-premultiplied composite over bg, all in 16-bit space.
					inv := 0xffff - a
					sumR += uint64(r + (bgR*inv)/0xffff)
					sumG += uint64(g + (bgG*inv)/0xffff)
					sumB += uint64(bl + (bgB*inv)/0xffff)
					n++
				}
			}
			if n == 0 {
				continue
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8((sumR / n) >> 8),
				G: uint8((sumG / n) >> 8),
				B: uint8((sumB / n) >> 8),
				A: 0xff,
			})
		}
	}
	return dst
}

// renderHalfblocks turns an image into cols×rows terminal cells, one string per
// row, each exactly cols display cells wide. It resamples internally, so pass
// the source image and the cell box from fitCells.
//
// Runs of cells sharing both colors are emitted under a single SGR pair; a
// photo still costs a few KB per frame, but a flat logo collapses to almost
// nothing. On a non-TTY (tests, pipes) lipgloss strips color and each cell
// degrades to a bare ▀ — the width contract holds either way.
func renderHalfblocks(src image.Image, cols, rows int, bg color.Color) []string {
	if cols <= 0 || rows <= 0 {
		return nil
	}

	px := scaleTo(src, cols, rows*2, bg)
	out := make([]string, rows)

	for r := 0; r < rows; r++ {
		var b strings.Builder
		var (
			runTop, runBot color.RGBA
			runLen         int
		)
		flush := func() {
			if runLen == 0 {
				return
			}
			b.WriteString(lipgloss.NewStyle().
				Foreground(rgbColor(runTop)).
				Background(rgbColor(runBot)).
				Render(strings.Repeat(halfBlock, runLen)))
			runLen = 0
		}

		for c := 0; c < cols; c++ {
			top := px.RGBAAt(c, r*2)
			bot := px.RGBAAt(c, r*2+1)
			if runLen > 0 && top == runTop && bot == runBot {
				runLen++
				continue
			}
			flush()
			runTop, runBot, runLen = top, bot, 1
		}
		flush()

		out[r] = b.String()
	}
	return out
}

// rgbColor converts to the hex string lipgloss wants, downsampling itself on
// terminals without truecolor.
func rgbColor(c color.RGBA) lipgloss.Color {
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B))
}
