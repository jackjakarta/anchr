package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/jackjakarta/anchr/s3client"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero", 0, "0 B"},
		{"bytes", 512, "512 B"},
		{"max bytes", 1023, "1023 B"},
		{"one kib", 1024, "1.0 KB"},
		{"one and a half kib", 1536, "1.5 KB"},
		{"max kib", (1 << 20) - 1, "1024.0 KB"},
		{"one mib", 1 << 20, "1.0 MB"},
		{"one gib", 1 << 30, "1.0 GB"},
		{"multi gib", 5 << 30, "5.0 GB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatSize(tt.bytes); got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{"two digit day", time.Date(2026, time.June, 13, 10, 30, 0, 0, time.UTC), "Jun 13"},
		{"single digit day is space padded", time.Date(2026, time.June, 5, 0, 0, 0, 0, time.UTC), "Jun  5"},
		{"january", time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC), "Jan  1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDate(tt.in); got != tt.want {
				t.Errorf("formatDate(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCanGoBack(t *testing.T) {
	tests := []struct {
		name string
		b    browser
		want bool
	}{
		{"root, empty stack", browser{prefix: "", prefixStack: nil}, false},
		{"nested prefix", browser{prefix: "a/", prefixStack: nil}, true},
		{"empty prefix but non-empty stack", browser{prefix: "", prefixStack: []string{""}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.b.canGoBack(); got != tt.want {
				t.Errorf("canGoBack() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectedItem(t *testing.T) {
	items := []s3client.S3Item{
		{Name: "alpha.txt"},
		{Name: "beta.txt"},
		{Name: "gamma.txt"},
	}

	tests := []struct {
		name     string
		b        browser
		wantName string
		wantDir  bool
		wantOK   bool
	}{
		{
			name:   "empty, cannot go back",
			b:      browser{items: nil, prefix: ""},
			wantOK: false,
		},
		{
			name:     "empty, can go back, cursor on ..",
			b:        browser{items: nil, prefix: "a/", cursor: 0},
			wantName: "../",
			wantDir:  true,
			wantOK:   true,
		},
		{
			name:     "no back, cursor in range",
			b:        browser{items: items, prefix: "", cursor: 1},
			wantName: "beta.txt",
			wantOK:   true,
		},
		{
			name:     "can go back, cursor 0 is ..",
			b:        browser{items: items, prefix: "a/", cursor: 0},
			wantName: "../",
			wantDir:  true,
			wantOK:   true,
		},
		{
			name:     "can go back, cursor 1 maps to items[0]",
			b:        browser{items: items, prefix: "a/", cursor: 1},
			wantName: "alpha.txt",
			wantOK:   true,
		},
		{
			name:     "can go back, cursor on last real row",
			b:        browser{items: items, prefix: "a/", cursor: len(items)},
			wantName: "gamma.txt",
			wantOK:   true,
		},
		{
			name:   "no back, cursor out of bounds",
			b:      browser{items: items, prefix: "", cursor: len(items)},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.b.selectedItem()
			if ok != tt.wantOK {
				t.Fatalf("selectedItem() ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.Name != tt.wantName {
				t.Errorf("selectedItem() name = %q, want %q", got.Name, tt.wantName)
			}
			if got.IsDir != tt.wantDir {
				t.Errorf("selectedItem() IsDir = %v, want %v", got.IsDir, tt.wantDir)
			}
		})
	}
}

func TestEnterFolder(t *testing.T) {
	b := browser{prefix: "a/", cursor: 5, offset: 3}
	b.enterFolder("a/b/")

	if b.prefix != "a/b/" {
		t.Errorf("prefix = %q, want %q", b.prefix, "a/b/")
	}
	if len(b.prefixStack) != 1 || b.prefixStack[0] != "a/" {
		t.Errorf("prefixStack = %v, want [%q]", b.prefixStack, "a/")
	}
	if b.cursor != 0 {
		t.Errorf("cursor = %d, want 0", b.cursor)
	}
	if b.offset != 0 {
		t.Errorf("offset = %d, want 0", b.offset)
	}
	if !b.loading {
		t.Errorf("loading = false, want true")
	}
}

func TestGoBack(t *testing.T) {
	t.Run("nowhere to go", func(t *testing.T) {
		b := browser{prefix: "", prefixStack: nil}
		got, ok := b.goBack()
		if ok {
			t.Errorf("goBack() ok = true, want false")
		}
		if got != "" {
			t.Errorf("goBack() = %q, want %q", got, "")
		}
		if b.prefix != "" {
			t.Errorf("prefix mutated to %q, want unchanged", b.prefix)
		}
	})

	t.Run("clear prefix to root", func(t *testing.T) {
		b := browser{prefix: "a/", prefixStack: nil, cursor: 4, offset: 2}
		got, ok := b.goBack()
		if !ok {
			t.Fatalf("goBack() ok = false, want true")
		}
		if got != "" {
			t.Errorf("goBack() = %q, want %q", got, "")
		}
		if b.prefix != "" {
			t.Errorf("prefix = %q, want %q", b.prefix, "")
		}
		if b.cursor != 0 || b.offset != 0 {
			t.Errorf("cursor/offset = %d/%d, want 0/0", b.cursor, b.offset)
		}
		if !b.loading {
			t.Errorf("loading = false, want true")
		}
	})

	t.Run("pop stack", func(t *testing.T) {
		b := browser{prefix: "a/b/", prefixStack: []string{"", "a/"}, cursor: 4, offset: 2}
		got, ok := b.goBack()
		if !ok {
			t.Fatalf("goBack() ok = false, want true")
		}
		if got != "a/" {
			t.Errorf("goBack() = %q, want %q", got, "a/")
		}
		if b.prefix != "a/" {
			t.Errorf("prefix = %q, want %q", b.prefix, "a/")
		}
		if len(b.prefixStack) != 1 || b.prefixStack[0] != "" {
			t.Errorf("prefixStack = %v, want [%q]", b.prefixStack, "")
		}
		if b.cursor != 0 || b.offset != 0 {
			t.Errorf("cursor/offset = %d/%d, want 0/0", b.cursor, b.offset)
		}
	})
}

func TestApplySort(t *testing.T) {
	// fixed timestamps so ordering is deterministic
	t1 := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)

	// mix of dirs and files, intentionally unsorted
	base := func() []s3client.S3Item {
		return []s3client.S3Item{
			{Name: "banana.txt", IsDir: false, Size: 300, LastModified: t2},
			{Name: "zebra", IsDir: true},
			{Name: "apple.txt", IsDir: false, Size: 100, LastModified: t3},
			{Name: "mango", IsDir: true},
			{Name: "cherry.txt", IsDir: false, Size: 200, LastModified: t1},
		}
	}

	names := func(items []s3client.S3Item) []string {
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = it.Name
		}
		return out
	}

	equal := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	// dirsFirst asserts every directory precedes every file.
	dirsFirst := func(t *testing.T, items []s3client.S3Item) {
		t.Helper()
		seenFile := false
		for _, it := range items {
			if !it.IsDir {
				seenFile = true
			} else if seenFile {
				t.Errorf("directory %q appears after a file; dirs must come first: %v", it.Name, names(items))
				return
			}
		}
	}

	tests := []struct {
		name    string
		sortBy  sortKey
		reverse bool
		want    []string
	}{
		// name distinguishes the two dirs, so they sort alphabetically.
		{"name asc", sortByName, false, []string{"mango", "zebra", "apple.txt", "banana.txt", "cherry.txt"}},
		{"name desc", sortByName, true, []string{"zebra", "mango", "cherry.txt", "banana.txt", "apple.txt"}},
		// dirs have equal Size/LastModified (both zero), so a stable sort keeps
		// their original input order (zebra before mango). Only files reorder.
		{"size asc", sortBySize, false, []string{"zebra", "mango", "apple.txt", "cherry.txt", "banana.txt"}},
		{"modified asc", sortByModified, false, []string{"zebra", "mango", "cherry.txt", "banana.txt", "apple.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := browser{items: base(), sortBy: tt.sortBy, sortReverse: tt.reverse}
			b.applySort()
			dirsFirst(t, b.items)
			if got := names(b.items); !equal(got, tt.want) {
				t.Errorf("applySort() order = %v, want %v", got, tt.want)
			}
		})
	}

	// the key invariant: dirs stay first even when the sort is reversed.
	t.Run("dirs first even when reversed", func(t *testing.T) {
		for _, k := range []sortKey{sortByName, sortBySize, sortByModified} {
			b := browser{items: base(), sortBy: k, sortReverse: true}
			b.applySort()
			dirsFirst(t, b.items)
		}
	})
}

func TestRenderItem(t *testing.T) {
	items := []s3client.S3Item{
		{Name: "alpha.txt", Size: 100},
		{Name: "docs", IsDir: true, Size: 5000}, // dirs ignore Size, show "-"
	}

	t.Run("synthetic .. at index 0 when can go back", func(t *testing.T) {
		b := browser{items: items, prefix: "a/"}
		got := b.renderItem(0, 30)
		if !strings.Contains(got, "../") {
			t.Errorf("renderItem(0) = %q, want it to contain %q", got, "../")
		}
	})

	t.Run("index 1 maps to items[0] when can go back", func(t *testing.T) {
		b := browser{items: items, prefix: "a/"}
		got := b.renderItem(1, 30)
		if !strings.Contains(got, "alpha.txt") {
			t.Errorf("renderItem(1) = %q, want it to contain %q", got, "alpha.txt")
		}
	})

	t.Run("long name is truncated", func(t *testing.T) {
		long := "this-is-a-very-long-object-name.txt"
		b := browser{items: []s3client.S3Item{{Name: long}}, prefix: ""}
		nameW := 12
		got := b.renderItem(0, nameW)
		wantTrunc := long[:nameW-3] + "..." // "this-is-a..."
		if !strings.Contains(got, wantTrunc) {
			t.Errorf("renderItem = %q, want it to contain truncated %q", got, wantTrunc)
		}
		if strings.Contains(got, long) {
			t.Errorf("renderItem = %q, should not contain the full untruncated name", got)
		}
	})

	t.Run("directory shows dash for size, not a byte size", func(t *testing.T) {
		b := browser{items: items, prefix: ""}
		got := b.renderItem(1, 30) // items[1] is the "docs" dir
		if !strings.Contains(got, "docs") {
			t.Fatalf("renderItem = %q, want it to contain %q", got, "docs")
		}
		if !strings.Contains(got, "-") {
			t.Errorf("renderItem = %q, want dir size column to contain %q", got, "-")
		}
		if strings.Contains(got, "KB") || strings.Contains(got, "5000") {
			t.Errorf("renderItem = %q, dir should not render a byte size", got)
		}
	})
}
