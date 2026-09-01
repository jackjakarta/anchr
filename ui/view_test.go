package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/jackjakarta/anchr/config"
	"github.com/jackjakarta/anchr/s3client"
)

// testModel builds a fully-populated Model (no live S3) for rendering tests.
func testModel() Model {
	mod := time.Date(2026, time.May, 24, 14, 32, 0, 0, time.UTC)
	m := Model{
		sidebar: newSidebar([]string{"el-chat", "mixbuddy-dev", "hotel-app-public", "anchr-backups"}),
		browser: newBrowser(),
		configs: []config.BucketConfig{
			{Name: "el-chat", Region: "us-east-1"},
			{Name: "mixbuddy-dev", Region: "us-east-1"},
			{Name: "hotel-app-public", Region: "eu-central-1"},
			{Name: "anchr-backups", Region: "us-east-1"},
		},
		openBucketIdx: 1,
		focus:         focusBrowser,
	}
	m.browser.focused = true
	m.browser.bucket = "mixbuddy-dev"
	m.browser.prefix = "projects/b7e550f5-0a7e-4a5a-a2da-4ed60a523f59/files/"
	m.browser.items = []s3client.S3Item{
		{Key: m.browser.prefix + "stems/", Name: "stems/", IsDir: true, LastModified: mod},
		{Key: m.browser.prefix + "mixdown_final.mp3", Name: "mixdown_final.mp3", Size: 5 << 20, LastModified: mod, ETag: "9f2ac41", StorageClass: "STANDARD"},
		{Key: m.browser.prefix + "POWxSdmT-Bassomatic.wav", Name: "POWxSdmT-Bassomatic.wav", Size: 63 << 20, LastModified: mod, ETag: "9f2ac41", StorageClass: "STANDARD"},
		{Key: m.browser.prefix + "session.als", Name: "session.als", Size: 842 << 10, LastModified: mod, ETag: "abc", StorageClass: "STANDARD"},
		{Key: m.browser.prefix + "cover.png", Name: "cover.png", Size: 1 << 20, LastModified: mod, ETag: "def", StorageClass: "STANDARD"},
		{Key: m.browser.prefix + "notes.txt", Name: "notes.txt", Size: 3 << 10, LastModified: mod, ETag: "ghi", StorageClass: "STANDARD"},
	}
	m.browser.applySort()
	return m
}

// assertGrid checks that View() renders exactly height rows, each exactly width
// display cells — i.e. nothing wrapped or overflowed.
func assertGrid(t *testing.T, m Model) {
	t.Helper()
	m.updateLayout()
	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Fatalf("View() at %dx%d produced %d lines, want %d", m.width, m.height, len(lines), m.height)
	}
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != m.width {
			t.Errorf("line %d width = %d, want %d (overflow/short row): %q", i, w, m.width, ln)
		}
	}
}

func TestViewGridInvariants(t *testing.T) {
	sizes := []struct{ w, h int }{
		{120, 38}, // wide: three panes
		{100, 30}, // three panes, smaller
		{85, 24},  // preview hidden
		{60, 20},  // narrow sidebar, preview hidden
	}
	for _, s := range sizes {
		m := testModel()
		m.width, m.height = s.w, s.h
		t.Run("focus-browser", func(t *testing.T) { assertGrid(t, m) })

		t.Run("downloading", func(t *testing.T) {
			md := m
			md.browser.downloading = true
			md.downloadingName = "mixdown_final.mp3"
			assertGrid(t, md)
		})

		t.Run("cursor-on-dotdot", func(t *testing.T) {
			mc := m
			mc.browser.cursor = 0 // synthetic "../"
			assertGrid(t, mc)
		})

		// The `/` filter states. These exercise renderNameCell, whose split NAME
		// cell must still occupy exactly nameW cells for the grid to hold.
		t.Run("filter-typing", func(t *testing.T) {
			mf := m
			mf.browser.startFilter()
			mf.browser.setFilter("mix")
			assertGrid(t, mf)
		})

		t.Run("filter-committed-cursor-on-match", func(t *testing.T) {
			mf := m
			mf.browser.setFilter("a")
			mf.browser.commitFilter()
			mf.browser.cursor = 1 // first match, so the cursor row carries a highlight
			assertGrid(t, mf)
		})

		t.Run("filter-match-truncated", func(t *testing.T) {
			// The match sits at the tail of the longest name, so at narrow widths
			// the highlight is clipped by truncate().
			mf := m
			mf.browser.setFilter("wav")
			mf.browser.commitFilter()
			mf.browser.cursor = 1
			assertGrid(t, mf)
		})

		t.Run("filter-no-matches", func(t *testing.T) {
			mf := m
			mf.browser.startFilter()
			mf.browser.setFilter("zzzznope")
			assertGrid(t, mf)
		})

		t.Run("filter-no-matches-at-root", func(t *testing.T) {
			// No "../" row, so the pane holds only the header and the note.
			mf := m
			mf.browser.prefix = ""
			mf.browser.setFilter("zzzznope")
			assertGrid(t, mf)
		})
	}
}

func TestViewStatesGrid(t *testing.T) {
	t.Run("no bucket selected", func(t *testing.T) {
		m := Model{sidebar: newSidebar([]string{"a", "b"}), browser: newBrowser(), openBucketIdx: -1}
		m.width, m.height = 110, 30
		assertGrid(t, m)
	})

	t.Run("loading", func(t *testing.T) {
		m := testModel()
		m.browser.loading = true
		m.browser.items = nil
		m.width, m.height = 110, 30
		assertGrid(t, m)
	})

	t.Run("error", func(t *testing.T) {
		m := testModel()
		m.browser.err = errTest
		m.browser.items = nil
		m.width, m.height = 110, 30
		assertGrid(t, m)
	})

	t.Run("empty listing", func(t *testing.T) {
		m := testModel()
		m.browser.items = nil
		m.browser.prefix = "" // root, nothing to list, no "../"
		m.width, m.height = 110, 30
		assertGrid(t, m)
	})
}

type stringErr string

func (e stringErr) Error() string { return string(e) }

var errTest = stringErr("boom")
