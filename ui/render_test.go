package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/jackjakarta/anchr/s3client"
)

func TestKindFor(t *testing.T) {
	tests := []struct {
		name      string
		item      s3client.S3Item
		wantLabel string
		wantMime  string
	}{
		{"dir", s3client.S3Item{Name: "stems", IsDir: true}, "DIR", "directory"},
		{"wav", s3client.S3Item{Name: "a.wav"}, "WAV", "audio/wav"},
		{"mp3 upper ext", s3client.S3Item{Name: "A.MP3"}, "MP3", "audio/mpeg"},
		{"png", s3client.S3Item{Name: "cover.png"}, "PNG", "image/png"},
		{"unknown ext", s3client.S3Item{Name: "x.qqq"}, "FILE", ""},
		{"no ext", s3client.S3Item{Name: "README"}, "FILE", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := kindFor(tt.item)
			if k.Label != tt.wantLabel {
				t.Errorf("kindFor(%q).Label = %q, want %q", tt.item.Name, k.Label, tt.wantLabel)
			}
			if k.Mime != tt.wantMime {
				t.Errorf("kindFor(%q).Mime = %q, want %q", tt.item.Name, k.Mime, tt.wantMime)
			}
		})
	}
}

func TestFileType(t *testing.T) {
	tests := []struct {
		name string
		item s3client.S3Item
		want string
	}{
		{"dir", s3client.S3Item{Name: "d", IsDir: true}, "directory"},
		{"curated wav", s3client.S3Item{Name: "a.wav"}, "audio/wav"},
		{"markdown fallback", s3client.S3Item{Name: "readme.md"}, "text/markdown"},
		{"no ext", s3client.S3Item{Name: "LICENSE"}, "file"},
		{"unknown ext", s3client.S3Item{Name: "a.qqq"}, "qqq file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fileType(tt.item); got != tt.want {
				t.Errorf("fileType(%q) = %q, want %q", tt.item.Name, got, tt.want)
			}
		})
	}
}

func TestBrowserCountsAndPosition(t *testing.T) {
	t1 := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	items := []s3client.S3Item{
		{Key: "p/dir/", Name: "dir", IsDir: true},
		{Key: "p/a.txt", Name: "a.txt", Size: 100, LastModified: t1},
		{Key: "p/b.txt", Name: "b.txt", Size: 200, LastModified: t1},
	}

	b := browser{items: items}
	if got := b.fileCount(); got != 2 {
		t.Errorf("fileCount() = %d, want 2 (dirs excluded)", got)
	}
	if got := b.totalSize(); got != 300 {
		t.Errorf("totalSize() = %d, want 300", got)
	}

	// Cursor on the second file (index 2 = items[2]) at root (no "../").
	b.cursor = 2
	pos, total := b.statusPosition()
	if pos != 3 || total != 3 {
		t.Errorf("statusPosition() = (%d,%d), want (3,3)", pos, total)
	}

	// With a synthetic "../" present, cursor 0 selects ".." → pos 0.
	bb := browser{items: items, prefix: "p/"}
	pos, total = bb.statusPosition()
	if pos != 0 || total != 3 {
		t.Errorf("statusPosition() on .. = (%d,%d), want (0,3)", pos, total)
	}
}

func TestFormatCount(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1k"},
		{2400, "2.4k"},
		{1_000_000, "1M"},
		{5_100_000, "5.1M"},
	}
	for _, tt := range tests {
		if got := formatCount(tt.in); got != tt.want {
			t.Errorf("formatCount(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGroupDigits(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{42, "42"},
		{1000, "1,000"},
		{66498560, "66,498,560"},
	}
	for _, tt := range tests {
		if got := groupDigits(tt.in); got != tt.want {
			t.Errorf("groupDigits(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatModified(t *testing.T) {
	in := time.Date(2026, time.May, 24, 14, 32, 0, 0, time.UTC)
	if got := formatModified(in); got != "2026-05-24 14:32" {
		t.Errorf("formatModified = %q, want %q", got, "2026-05-24 14:32")
	}
}

func TestMaybeEllipsize(t *testing.T) {
	short := "projects"
	if got := maybeEllipsize(short); got != short {
		t.Errorf("maybeEllipsize(%q) = %q, want unchanged", short, got)
	}
	long := "b7e550f5-0a7e-4a5a-a2da-4ed60a523f59"
	got := maybeEllipsize(long)
	if !strings.Contains(got, "…") || len([]rune(got)) >= len([]rune(long)) {
		t.Errorf("maybeEllipsize(%q) = %q, want it shortened with an ellipsis", long, got)
	}
}

func TestRenderBreadcrumb(t *testing.T) {
	t.Run("no bucket", func(t *testing.T) {
		m := Model{}
		got := m.renderBreadcrumb(40)
		if !strings.Contains(got, "no bucket") {
			t.Errorf("renderBreadcrumb = %q, want it to mention no bucket", got)
		}
	})

	t.Run("fits: shows all segments", func(t *testing.T) {
		m := Model{browser: browser{bucket: "mixbuddy-dev", prefix: "projects/files/"}}
		got := m.renderBreadcrumb(60)
		for _, want := range []string{"mixbuddy-dev", "projects", "files"} {
			if !strings.Contains(got, want) {
				t.Errorf("renderBreadcrumb = %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("overflow: collapses middle to ellipsis", func(t *testing.T) {
		m := Model{browser: browser{bucket: "mixbuddy-dev", prefix: "projects/sub/deeper/files/"}}
		got := m.renderBreadcrumb(28)
		if !strings.Contains(got, "…") {
			t.Errorf("renderBreadcrumb = %q, want a collapsed ellipsis", got)
		}
		if !strings.Contains(got, "mixbuddy-dev") || !strings.Contains(got, "files") {
			t.Errorf("renderBreadcrumb = %q, want bucket + current segment kept", got)
		}
	})
}
