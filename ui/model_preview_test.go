package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jackjakarta/anchr/s3client"
)

// previewTestModel parks the cursor on cover.png with a never-dialled client.
func previewTestModel(t *testing.T) Model {
	t.Helper()
	m := testModel()
	m.width, m.height = 120, 38
	m.clients = []*s3client.Client{nil} // cmds are inspected, never run
	m.updateLayout()
	for i, it := range m.browser.rows() {
		if it.Name == "cover.png" {
			m.browser.cursor = i
			return m
		}
	}
	t.Fatal("setup: cover.png not found in the test listing")
	return m
}

// An image payload must take the image path, and the model must follow up with
// the render command rather than trying to decode inside Update.
func TestPreviewImageFlow(t *testing.T) {
	m := previewTestModel(t)
	m.preview.open("cover.png")

	png := encodePNG(t, gradientImage(800, 600))
	m, cmd := step(t, m, ObjectPreviewLoadedMsg{Content: png, ContentType: "image/png"})

	if !m.preview.hasImage() {
		t.Fatal("a PNG payload did not take the image path")
	}
	if m.preview.binary {
		t.Error("image payload was flagged binary")
	}
	if m.preview.img.srcW != 800 || m.preview.img.srcH != 600 {
		t.Errorf("intrinsic size = %dx%d, want 800x600", m.preview.img.srcW, m.preview.img.srcH)
	}
	if m.preview.img.format != "png" {
		t.Errorf("format = %q, want png", m.preview.img.format)
	}
	if !m.preview.img.rendering {
		t.Error("image should be marked as rendering while the cmd is in flight")
	}
	if cmd == nil {
		t.Fatal("no render command was returned")
	}

	msg, ok := cmd().(ImageRenderedMsg)
	if !ok {
		t.Fatal("render command did not produce an ImageRenderedMsg")
	}
	m, _ = step(t, m, msg)

	if m.preview.img.rendering {
		t.Error("still rendering after ImageRenderedMsg")
	}
	if m.preview.img.err != nil {
		t.Fatalf("render failed: %v", m.preview.img.err)
	}
	if len(m.preview.img.rows) == 0 {
		t.Fatal("no rows were rendered")
	}
	if m.preview.img.cols <= 0 {
		t.Error("cols was not recorded")
	}
}

// Non-image payloads must behave exactly as before.
func TestPreviewNonImageUnaffected(t *testing.T) {
	m := previewTestModel(t)

	t.Run("text", func(t *testing.T) {
		mt := m
		mt.preview.open("notes.txt")
		mt, cmd := step(t, mt, ObjectPreviewLoadedMsg{Content: []byte("line one\nline two"), ContentType: "text/plain"})
		if mt.preview.hasImage() {
			t.Error("text payload took the image path")
		}
		if cmd != nil {
			t.Error("text payload returned a render command")
		}
		if len(mt.preview.lines) != 2 {
			t.Errorf("lines = %d, want 2", len(mt.preview.lines))
		}
	})

	t.Run("binary", func(t *testing.T) {
		mb := m
		mb.preview.open("archive.zip")
		mb, cmd := step(t, mb, ObjectPreviewLoadedMsg{Content: []byte{0x50, 0x4b, 0x03, 0x04, 0x00}, ContentType: "application/zip"})
		if mb.preview.hasImage() {
			t.Error("zip payload took the image path")
		}
		if cmd != nil {
			t.Error("zip payload returned a render command")
		}
		if !mb.preview.binary {
			t.Error("zip payload was not flagged binary")
		}
	})
}

// WebP has an image/* kind but no decoder, so it must fall through to the
// binary note rather than producing an empty image popup.
func TestPreviewUndecodableImageFallsBack(t *testing.T) {
	m := previewTestModel(t)
	m.preview.open("photo.webp")
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 64)...)

	m, cmd := step(t, m, ObjectPreviewLoadedMsg{Content: webp, ContentType: "image/webp"})
	if m.preview.hasImage() {
		t.Error("undecodable webp took the image path")
	}
	if cmd != nil {
		t.Error("undecodable webp returned a render command")
	}
	if !m.preview.binary {
		t.Error("undecodable webp should fall back to the binary note")
	}
}

// A resize must re-rasterise from the bytes already in hand.
func TestPreviewImageResizeRerenders(t *testing.T) {
	m := previewTestModel(t)
	m.preview.open("cover.png")
	m, _ = step(t, m, ObjectPreviewLoadedMsg{Content: encodePNG(t, gradientImage(800, 600)), ContentType: "image/png"})
	if cmd := m.renderImage(); cmd != nil {
		m, _ = step(t, m, cmd().(ImageRenderedMsg))
	}
	wide := m.preview.img.cols

	m, cmd := step(t, m, tea.WindowSizeMsg{Width: 70, Height: 22})
	if cmd == nil {
		t.Fatal("resize did not trigger a re-render")
	}
	if !m.preview.img.rendering {
		t.Error("resize did not mark the image as rendering")
	}
	m, _ = step(t, m, cmd().(ImageRenderedMsg))

	if m.preview.img.cols >= wide {
		t.Errorf("after shrinking to 70 cols the image is %d cols, was %d", m.preview.img.cols, wide)
	}
	// The bytes must be reused, not refetched.
	if len(m.preview.img.data) == 0 {
		t.Error("source bytes were dropped, so the re-render must have refetched")
	}
}

// A 10 MB budget is a lot to leave in flight after the user has moved on.
func TestPreviewCloseCancelsFetch(t *testing.T) {
	m := previewTestModel(t)
	cancelled := false
	m.preview.open("cover.png")
	m.cancelPreview = func() { cancelled = true }

	m, _ = step(t, m, special(tea.KeyEsc))

	if !cancelled {
		t.Error("closing the popup did not cancel the in-flight fetch")
	}
	if m.cancelPreview != nil {
		t.Error("cancelPreview was not cleared")
	}
	if m.preview.active {
		t.Error("popup is still active after esc")
	}
}

func TestPreviewStartCancelsPreviousFetch(t *testing.T) {
	m := previewTestModel(t)
	cancelled := false
	m.cancelPreview = func() { cancelled = true }

	m, cmd := step(t, m, runeKey("p"))
	if !cancelled {
		t.Error("starting a new preview did not cancel the previous fetch")
	}
	if cmd == nil {
		t.Error("p did not start a fetch")
	}
	if m.cancelPreview == nil {
		t.Error("the new fetch registered no cancel func")
	}
}

// A cancelled fetch is the user's own doing; it must not surface as an error.
func TestPreviewCancelledFetchIsSilent(t *testing.T) {
	m := previewTestModel(t)
	m.preview.open("cover.png")

	m, _ = step(t, m, ObjectPreviewLoadedMsg{Err: context.Canceled})
	if m.preview.err != nil {
		t.Errorf("cancelled fetch surfaced as an error: %v", m.preview.err)
	}

	// A wrapped cancellation (as the AWS SDK returns it) counts too.
	m2 := previewTestModel(t)
	m2.preview.open("cover.png")
	m2, _ = step(t, m2, ObjectPreviewLoadedMsg{Err: fmtWrap(context.Canceled)})
	if m2.preview.err != nil {
		t.Errorf("wrapped cancellation surfaced as an error: %v", m2.preview.err)
	}

	// A genuine failure still shows.
	m3 := previewTestModel(t)
	m3.preview.open("cover.png")
	m3, _ = step(t, m3, ObjectPreviewLoadedMsg{Err: errors.New("AccessDenied")})
	if m3.preview.err == nil {
		t.Error("a real fetch error was swallowed")
	}
}

func fmtWrap(err error) error {
	return &wrapped{err}
}

type wrapped struct{ err error }

func (w *wrapped) Error() string { return "operation error S3: GetObject, " + w.err.Error() }
func (w *wrapped) Unwrap() error { return w.err }

func TestIsImageKind(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"cover.png", true},
		{"photo.JPG", true}, // itemExt lowercases
		{"anim.gif", true},
		{"logo.svg", true},  // image/* kind, though decodeImage cannot read it
		{"shot.webp", true}, // ditto
		{"notes.txt", false},
		{"session.als", false},
		{"archive.zip", false},
		{"noextension", false},
	}
	for _, tc := range tests {
		item := s3client.S3Item{Name: tc.name, Key: "p/" + tc.name}
		if got := isImageKind(item); got != tc.want {
			t.Errorf("isImageKind(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
	if isImageKind(s3client.S3Item{Name: "images/", IsDir: true}) {
		t.Error("isImageKind said true for a directory")
	}
}

// The caption tells the user which backend they are looking at.
func TestImageCaption(t *testing.T) {
	p := preview{img: &previewImage{srcW: 1920, srcH: 1080, format: "jpeg", backend: backendHalfblock}}
	if got := p.imageCaption(); !strings.Contains(got, "1920×1080") || !strings.Contains(got, "JPEG") || !strings.Contains(got, "halfblocks") {
		t.Errorf("caption = %q", got)
	}
	p.img.backend = backendKitty
	if got := p.imageCaption(); !strings.Contains(got, "kitty graphics") {
		t.Errorf("kitty caption = %q", got)
	}
}
