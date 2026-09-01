package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// gradientImage builds a w×h test image with a horizontal/vertical gradient so
// adjacent cells differ (exercising the run-coalescing path in
// renderHalfblocks) without needing a binary fixture on disk.
func gradientImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x * 255 / max(w-1, 1)),
				G: uint8(y * 255 / max(h-1, 1)),
				B: 0x40,
				A: 0xff,
			})
		}
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func TestFitCells(t *testing.T) {
	tests := []struct {
		name                         string
		imgW, imgH, maxCols, maxRows int
		wantCols, wantRows           int
	}{
		// A 2:1 image in a 2:1 pixel grid (cols × rows*2) fills the width.
		{"landscape fills width", 800, 400, 80, 40, 80, 20},
		{"square is width-bound", 400, 400, 80, 40, 80, 40},
		// Tall images hit the row cap and give width back.
		{"portrait is height-bound", 400, 1600, 80, 20, 10, 20},
		{"tiny image upscales to box", 4, 4, 40, 40, 40, 20},
		// Degenerate inputs must not panic or return negatives.
		{"zero source", 0, 0, 80, 40, 0, 0},
		{"zero box", 800, 400, 0, 0, 0, 0},
		{"negative box", 800, 400, -5, -5, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cols, rows := fitCells(tc.imgW, tc.imgH, tc.maxCols, tc.maxRows)
			if cols != tc.wantCols || rows != tc.wantRows {
				t.Errorf("fitCells(%d,%d,%d,%d) = %d,%d; want %d,%d",
					tc.imgW, tc.imgH, tc.maxCols, tc.maxRows, cols, rows, tc.wantCols, tc.wantRows)
			}
			if tc.maxCols > 0 && tc.maxRows > 0 && (cols > tc.maxCols || rows > tc.maxRows) {
				t.Errorf("result %dx%d exceeds box %dx%d", cols, rows, tc.maxCols, tc.maxRows)
			}
		})
	}
}

func TestFitCellsPreservesAspect(t *testing.T) {
	// The rendered box, measured in square pixels, should stay close to the
	// source aspect ratio. Cell quantisation means this can only be approximate.
	for _, tc := range []struct{ w, h int }{
		{1920, 1080}, {1080, 1920}, {800, 800}, {3000, 500}, {500, 3000},
	} {
		cols, rows := fitCells(tc.w, tc.h, 96, 26)
		if cols == 0 || rows == 0 {
			t.Fatalf("fitCells(%d,%d) returned an empty box", tc.w, tc.h)
		}
		srcAspect := float64(tc.w) / float64(tc.h)
		gotAspect := float64(cols) / float64(rows*2)
		ratio := gotAspect / srcAspect
		if ratio < 0.75 || ratio > 1.33 {
			t.Errorf("fitCells(%d,%d) = %dx%d cells: aspect %.2f vs source %.2f (ratio %.2f)",
				tc.w, tc.h, cols, rows, gotAspect, srcAspect, ratio)
		}
	}
}

// TestRenderHalfblocksDimensions is the load-bearing one: the popup embeds
// these rows directly, so any row that is not exactly cols cells wide shifts
// the whole grid and breaks TestViewGridInvariants.
func TestRenderHalfblocksDimensions(t *testing.T) {
	sources := []struct {
		name string
		w, h int
	}{
		{"landscape", 200, 100},
		{"portrait", 100, 200},
		{"square", 120, 120},
		{"single pixel", 1, 1},
		{"very wide", 400, 3},
	}
	boxes := []struct{ cols, rows int }{{96, 26}, {40, 10}, {8, 3}, {1, 1}}

	for _, src := range sources {
		img := gradientImage(src.w, src.h)
		for _, box := range boxes {
			cols, rows := fitCells(src.w, src.h, box.cols, box.rows)
			out := renderHalfblocks(img, cols, rows, previewBgRGBA)
			if len(out) != rows {
				t.Errorf("%s in %dx%d: got %d rows, want %d", src.name, box.cols, box.rows, len(out), rows)
				continue
			}
			for i, line := range out {
				if w := lipgloss.Width(line); w != cols {
					t.Errorf("%s in %dx%d: row %d width = %d, want %d",
						src.name, box.cols, box.rows, i, w, cols)
				}
			}
		}
	}
}

func TestRenderHalfblocksEmptyBox(t *testing.T) {
	img := gradientImage(10, 10)
	for _, tc := range []struct{ cols, rows int }{{0, 5}, {5, 0}, {-1, -1}} {
		if got := renderHalfblocks(img, tc.cols, tc.rows, previewBgRGBA); got != nil {
			t.Errorf("renderHalfblocks(%d,%d) = %v, want nil", tc.cols, tc.rows, got)
		}
	}
}

// A fully transparent image must composite to the background color, not black.
func TestRenderHalfblocksCompositesAlpha(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4)) // zero value = fully transparent
	px := scaleTo(img, 2, 2, previewBgRGBA)
	got := px.RGBAAt(0, 0)
	want := color.RGBA{R: 0x1d, G: 0x20, B: 0x21, A: 0xff}
	if got != want {
		t.Errorf("transparent pixel composited to %v, want %v (popup background)", got, want)
	}
}

func TestDecodeImage(t *testing.T) {
	img := gradientImage(16, 16)

	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, img, nil); err != nil {
		t.Fatal(err)
	}
	var g bytes.Buffer
	if err := gif.Encode(&g, img, nil); err != nil {
		t.Fatal(err)
	}

	ok := []struct {
		name, format string
		data         []byte
	}{
		{"png", "png", encodePNG(t, img)},
		{"jpeg", "jpeg", jpg.Bytes()},
		{"gif", "gif", g.Bytes()},
	}
	for _, tc := range ok {
		t.Run(tc.name, func(t *testing.T) {
			decoded, format, err := decodeImage(tc.data)
			if err != nil {
				t.Fatalf("decodeImage: %v", err)
			}
			if format != tc.format {
				t.Errorf("format = %q, want %q", format, tc.format)
			}
			if b := decoded.Bounds(); b.Dx() != 16 || b.Dy() != 16 {
				t.Errorf("bounds = %v, want 16x16", b)
			}
		})
	}

	bad := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"garbage", []byte("not an image at all")},
		// A real WebP header — unsupported until golang.org/x/image is a dep.
		{"webp", append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 32)...)},
		// Truncated PNG: header matches, payload is cut short.
		{"truncated png", encodePNG(t, img)[:20]},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := decodeImage(tc.data); err == nil {
				t.Error("decodeImage succeeded, want an error")
			}
		})
	}
}

func TestDecodeImageUnsupportedIsSentinel(t *testing.T) {
	// The popup distinguishes "we can't read this format" from "these bytes are
	// broken", so the sentinel has to survive for formats we simply don't have.
	_, _, err := decodeImage([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "))
	if err != errUnsupportedImage {
		t.Errorf("err = %v, want errUnsupportedImage", err)
	}
}

// scaleTo is where a vertical flip or an R/B channel swap would hide: the
// halfblock rows read straight out of it, and a mirrored preview is the kind of
// bug that looks plausible until you compare against the real file.
func TestScaleToOrientationAndColor(t *testing.T) {
	// Four solid quadrants in a 40x40 source.
	src := image.NewRGBA(image.Rect(0, 0, 40, 40))
	quadrants := map[string]struct {
		x0, y0 int
		c      color.RGBA
	}{
		"top-left":     {0, 0, color.RGBA{0xff, 0x00, 0x00, 0xff}},
		"top-right":    {20, 0, color.RGBA{0x00, 0xff, 0x00, 0xff}},
		"bottom-left":  {0, 20, color.RGBA{0x00, 0x00, 0xff, 0xff}},
		"bottom-right": {20, 20, color.RGBA{0xff, 0xff, 0xff, 0xff}},
	}
	for _, q := range quadrants {
		for y := q.y0; y < q.y0+20; y++ {
			for x := q.x0; x < q.x0+20; x++ {
				src.SetRGBA(x, y, q.c)
			}
		}
	}

	dst := scaleTo(src, 4, 4, previewBgRGBA)

	// Sample the middle of each destination quadrant; the color must match the
	// source quadrant in the same corner, not a flipped or swapped one.
	checks := []struct {
		name string
		x, y int
		want color.RGBA
	}{
		{"top-left", 0, 0, quadrants["top-left"].c},
		{"top-right", 3, 0, quadrants["top-right"].c},
		{"bottom-left", 0, 3, quadrants["bottom-left"].c},
		{"bottom-right", 3, 3, quadrants["bottom-right"].c},
	}
	for _, c := range checks {
		if got := dst.RGBAAt(c.x, c.y); got != c.want {
			t.Errorf("%s pixel = %v, want %v (image flipped or channels swapped?)", c.name, got, c.want)
		}
	}
}

// Downscaling must average, not sample: a checkerboard has to come out grey,
// otherwise fine detail turns into aliasing noise at thumbnail sizes.
func TestScaleToAverages(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			if (x+y)%2 == 0 {
				src.SetRGBA(x, y, color.RGBA{0xff, 0xff, 0xff, 0xff})
			} else {
				src.SetRGBA(x, y, color.RGBA{0x00, 0x00, 0x00, 0xff})
			}
		}
	}

	got := scaleTo(src, 2, 2, previewBgRGBA).RGBAAt(0, 0)
	if got.R < 0x70 || got.R > 0x90 {
		t.Errorf("checkerboard averaged to R=%#x, want mid-grey (~0x80); is it point-sampling?", got.R)
	}
	if got.R != got.G || got.G != got.B {
		t.Errorf("grey average came out tinted: %v", got)
	}
}
