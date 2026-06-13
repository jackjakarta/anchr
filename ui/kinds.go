package ui

import (
	"mime"
	"path"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/jackjakarta/anchr/s3client"
)

// fileKind describes how an object renders: its leading icon, accent color,
// the short KIND badge label, and a coarse mime/type string. It is the single
// source of truth for both the file-list row and the preview pane's "Type".
type fileKind struct {
	Icon  string
	Color lipgloss.Color
	Label string // KIND badge text, e.g. "WAV"
	Mime  string // type string for the preview pane, e.g. "audio/wav"
}

// kindByExt maps a lowercased extension (no dot) to its fileKind. Per the V3
// design: audio ♪, Ableton ◈, image ▨, text/structured ≡.
var kindByExt = map[string]fileKind{
	// audio
	"mp3":  {"♪", cPurple, "MP3", "audio/mpeg"},
	"wav":  {"♪", cOrange, "WAV", "audio/wav"},
	"flac": {"♪", cOrange, "FLAC", "audio/flac"},
	"ogg":  {"♪", cOrange, "OGG", "audio/ogg"},
	"m4a":  {"♪", cOrange, "M4A", "audio/mp4"},
	"aiff": {"♪", cOrange, "AIFF", "audio/aiff"},
	// Ableton / project files
	"als": {"◈", cAquaBri, "ALS", "application/x-ableton"},
	// image
	"png":  {"▨", cGreen, "PNG", "image/png"},
	"jpg":  {"▨", cGreen, "JPG", "image/jpeg"},
	"jpeg": {"▨", cGreen, "JPG", "image/jpeg"},
	"gif":  {"▨", cGreen, "GIF", "image/gif"},
	"webp": {"▨", cGreen, "WEBP", "image/webp"},
	"svg":  {"▨", cGreen, "SVG", "image/svg+xml"},
	// text
	"txt": {"≡", cFgMut, "TXT", "text/plain"},
	"md":  {"≡", cFgMut, "MD", "text/markdown"},
	"log": {"≡", cFgMut, "LOG", "text/plain"},
	// structured / config
	"json": {"≡", cFgMut, "JSON", "application/json"},
	"yaml": {"≡", cFgMut, "YAML", "application/yaml"},
	"yml":  {"≡", cFgMut, "YAML", "application/yaml"},
	"toml": {"≡", cFgMut, "TOML", "application/toml"},
	"csv":  {"≡", cFgMut, "CSV", "text/csv"},
	// documents / archives
	"pdf": {"≡", cFgMut, "PDF", "application/pdf"},
	"zip": {"≡", cFgMut, "ZIP", "application/zip"},
	"tar": {"≡", cFgMut, "TAR", "application/x-tar"},
	"gz":  {"≡", cFgMut, "GZ", "application/gzip"},
}

var (
	kindDir     = fileKind{"▤", cAqua, "DIR", "directory"}
	kindDefault = fileKind{"≡", cFgMut, "FILE", ""}
)

// kindFor returns the fileKind for an item: directories short-circuit,
// otherwise the extension after the last '.' drives the lookup.
func kindFor(item s3client.S3Item) fileKind {
	if item.IsDir {
		return kindDir
	}
	if ext := itemExt(item.Name); ext != "" {
		if k, ok := kindByExt[ext]; ok {
			return k
		}
	}
	return kindDefault
}

// fileType is the preview pane's "Type" value: the curated mime when the kind
// is known, else the stdlib mime table (broad), else a generic "<ext> file".
func fileType(item s3client.S3Item) string {
	if item.IsDir {
		return "directory"
	}
	if k := kindFor(item); k.Mime != "" {
		return k.Mime
	}
	if t := mime.TypeByExtension(path.Ext(item.Name)); t != "" {
		if i := strings.IndexByte(t, ';'); i >= 0 { // strip "; charset=utf-8"
			t = t[:i]
		}
		return t
	}
	if ext := itemExt(item.Name); ext != "" {
		return ext + " file"
	}
	return "file"
}

// itemExt returns the lowercased extension (without the dot), or "".
func itemExt(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 && i < len(name)-1 {
		return strings.ToLower(name[i+1:])
	}
	return ""
}
