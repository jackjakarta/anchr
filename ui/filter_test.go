package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/jackjakarta/anchr/s3client"
)

func TestMatchIndex(t *testing.T) {
	tests := []struct {
		name  string
		obj   string
		query string
		want  int
	}{
		{"empty query matches at 0", "notes.txt", "", 0},
		{"lowercase query is case-insensitive", "POWxSdmT-Bassomatic.wav", "pow", 0},
		{"lowercase query matches lowercase", "session.als", "ses", 0},
		{"uppercase query is case-sensitive", "session.als", "SES", -1},
		{"uppercase query matches exactly", "POWxSdmT-Bassomatic.wav", "POW", 0},
		{"mixed case query stays sensitive", "mixdown_final.mp3", "Mix", -1},
		{"match past the start", "cover.png", "png", 6},
		{"no match", "cover.png", "wav", -1},
		{"query longer than name", "a.txt", "toolongquery", -1},
		{"must be contiguous, not a subsequence", "mixdown_final.mp3", "mixfin", -1},
		// Rune indices, not byte offsets: é is two bytes, so a byte-based
		// search would report 6 here and mis-place the highlight.
		{"index is a rune index", "café-münchen.txt", "münchen", 5},
		{"folds non-ascii case", "Ärger.txt", "är", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchIndex(tt.obj, tt.query); got != tt.want {
				t.Errorf("matchIndex(%q, %q) = %d, want %d", tt.obj, tt.query, got, tt.want)
			}
		})
	}
}

func logItems() []s3client.S3Item {
	return []s3client.S3Item{
		{Key: "p/app.log", Name: "app.log", Size: 300},
		{Key: "p/notes.txt", Name: "notes.txt", Size: 100},
		{Key: "p/sys.log", Name: "sys.log", Size: 200},
	}
}

func TestApplyFilterAndRows(t *testing.T) {
	b := browser{items: logItems()}

	if got := len(b.rows()); got != 3 {
		t.Errorf("rows() with no filter = %d, want the full listing of 3", got)
	}

	b.setFilter("log")
	if got := len(b.rows()); got != 2 {
		t.Fatalf("rows() after /log = %d, want 2", got)
	}
	if matched, total := b.matchCount(); matched != 2 || total != 3 {
		t.Errorf("matchCount() = (%d,%d), want (2,3)", matched, total)
	}
	// items is never narrowed, so clearing restores without a refetch.
	if len(b.items) != 3 {
		t.Errorf("items was mutated to %d entries, want the full 3 kept", len(b.items))
	}
	b.setFilter("")
	if got := len(b.rows()); got != 3 || b.matches != nil {
		t.Errorf("clearing left rows()=%d matches=%v, want 3 and nil", got, b.matches)
	}
}

func TestFilterFiltersDirectoriesToo(t *testing.T) {
	b := browser{items: []s3client.S3Item{
		{Key: "p/logs/", Name: "logs/", IsDir: true},
		{Key: "p/stems/", Name: "stems/", IsDir: true},
		{Key: "p/app.log", Name: "app.log"},
	}}
	b.setFilter("log")
	rows := b.rows()
	if len(rows) != 2 || rows[0].Name != "logs/" || rows[1].Name != "app.log" {
		t.Errorf("rows() = %v, want [logs/ app.log]", rows)
	}
}

func TestFilterCursorFollowsSelection(t *testing.T) {
	b := browser{items: logItems()}
	b.cursor = 2 // sys.log

	b.setFilter("log")
	if sel, _ := b.selectedItem(); sel.Name != "sys.log" {
		t.Errorf("selection = %q, want sys.log to survive the narrowing", sel.Name)
	}

	// Narrowing until the selection stops matching clamps to the first row.
	b.setFilter("app")
	if sel, _ := b.selectedItem(); sel.Name != "app.log" {
		t.Errorf("selection = %q, want a clamp to app.log", sel.Name)
	}
}

func TestFilterCursorWithParentRow(t *testing.T) {
	b := browser{prefix: "p/", items: logItems()}
	b.cursor = 3 // "../" holds row 0, so items[2] = sys.log
	if sel, _ := b.selectedItem(); sel.Name != "sys.log" {
		t.Fatalf("precondition: selection = %q, want sys.log", sel.Name)
	}

	b.setFilter("log")
	// rows() is [app.log sys.log]; sys.log is index 1, +1 for the "../" row.
	if b.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (match index 1 + the .. offset)", b.cursor)
	}
	if sel, _ := b.selectedItem(); sel.Name != "sys.log" {
		t.Errorf("selection = %q, want sys.log", sel.Name)
	}
	if got := b.itemCount(); got != 3 {
		t.Errorf("itemCount() = %d, want 3 (2 matches + \"../\")", got)
	}
}

// TestFilterLandsOnFirstMatch covers the case where the previous selection did
// not survive narrowing: the cursor must skip the synthetic "../" row so the
// object action keys have a real object to act on.
func TestFilterLandsOnFirstMatch(t *testing.T) {
	b := browser{prefix: "p/", items: logItems()}
	b.cursor = 0 // on "../"

	b.setFilter("log")
	if b.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (first match, past \"../\")", b.cursor)
	}
	sel, ok := b.selectedItem()
	if !ok || sel.Name != "app.log" {
		t.Errorf("selection = %q (ok=%v), want app.log", sel.Name, ok)
	}

	// With nothing matching there is no row to advance to; "../" stays selected.
	b.setFilter("zzz")
	if b.cursor != 0 {
		t.Errorf("cursor = %d with no matches, want 0", b.cursor)
	}

	// At the bucket root there is no "../" row to skip.
	r := browser{items: logItems()}
	r.setFilter("sys")
	if r.cursor != 0 {
		t.Errorf("cursor = %d at root, want 0", r.cursor)
	}
	if sel, _ := r.selectedItem(); sel.Name != "sys.log" {
		t.Errorf("selection = %q at root, want sys.log", sel.Name)
	}
}

func TestFilterClearedOnSetItems(t *testing.T) {
	b := browser{items: logItems()}
	b.setFilter("log")
	b.filtering = true

	b.setItems(&s3client.ListResult{
		Bucket: "bkt",
		Prefix: "q/",
		Items:  []s3client.S3Item{{Key: "q/x.txt", Name: "x.txt"}},
	})

	if b.filter != "" || b.filtering || b.matches != nil {
		t.Errorf("navigation left filter state behind: filter=%q filtering=%v matches=%v",
			b.filter, b.filtering, b.matches)
	}
	if got := len(b.rows()); got != 1 {
		t.Errorf("rows() = %d, want the new listing of 1", got)
	}
}

// TestSortWithFilterActive guards the stale-matches bug: applySort reorders
// items in place, so matches must be rebuilt after every re-sort.
func TestSortWithFilterActive(t *testing.T) {
	b := browser{items: logItems()}
	b.applySort() // by name: app.log, notes.txt, sys.log
	b.setFilter("log")
	b.cursor = 1 // sys.log, of [app.log sys.log]
	if sel, _ := b.selectedItem(); sel.Name != "sys.log" {
		t.Fatalf("precondition: selection = %q, want sys.log", sel.Name)
	}

	b.cycleSort() // sortByName -> sortBySize
	if b.sortBy != sortBySize {
		t.Fatalf("sortBy = %v, want sortBySize", b.sortBy)
	}

	rows := b.rows()
	if len(rows) != 2 {
		t.Fatalf("rows() after re-sort = %d, want the filter still applied (2)", len(rows))
	}
	// Ascending by size the matches are sys.log (200) then app.log (300).
	if rows[0].Name != "sys.log" || rows[1].Name != "app.log" {
		t.Errorf("rows() = [%s %s], want [sys.log app.log]", rows[0].Name, rows[1].Name)
	}
	if sel, _ := b.selectedItem(); sel.Name != "sys.log" {
		t.Errorf("selection = %q, want sys.log to survive the re-sort", sel.Name)
	}
}

func TestFilterModeTransitions(t *testing.T) {
	b := browser{items: logItems()}

	b.startFilter()
	b.filterAppend("l")
	b.filterAppend("o")
	b.filterAppend("g")
	if !b.filtering || b.filter != "log" {
		t.Fatalf("after typing: filtering=%v filter=%q, want true and \"log\"", b.filtering, b.filter)
	}

	b.filterBackspace()
	if b.filter != "lo" {
		t.Errorf("after backspace: filter = %q, want \"lo\"", b.filter)
	}

	// enter keeps the filter but releases the keys back to the browser.
	b.commitFilter()
	if b.filtering || b.filter != "lo" {
		t.Errorf("after commit: filtering=%v filter=%q, want false and \"lo\"", b.filtering, b.filter)
	}

	// esc in normal mode drops a committed filter.
	b.clearFilter()
	if b.filter != "" || len(b.rows()) != 3 {
		t.Errorf("after clear: filter=%q rows=%d, want \"\" and 3", b.filter, len(b.rows()))
	}

	// esc while typing aborts both the input and the query.
	b.startFilter()
	b.filterAppend("log")
	b.cancelFilter()
	if b.filtering || b.filter != "" || len(b.rows()) != 3 {
		t.Errorf("after cancel: filtering=%v filter=%q rows=%d, want false, \"\" and 3",
			b.filtering, b.filter, len(b.rows()))
	}

	// Backspace on an empty query is a no-op, not a panic.
	b.filterBackspace()
	if b.filter != "" {
		t.Errorf("backspace on empty filter = %q, want \"\"", b.filter)
	}
}

// TestRenderNameCellWidth is the guard on the highlight rendering: the NAME
// cell must occupy exactly w cells however the match falls, or the file list
// overflows its column and the whole grid shifts.
func TestRenderNameCellWidth(t *testing.T) {
	base := rowBlank
	nameStyle := base.Foreground(cFg)

	tests := []struct {
		name      string
		obj       string
		hi, hiLen int
		w         int
	}{
		{"no highlight", "mixdown_final.mp3", -1, 0, 24},
		{"match at start", "mixdown_final.mp3", 0, 3, 24},
		{"match in the middle", "mixdown_final.mp3", 8, 5, 24},
		{"match at the end", "mixdown_final.mp3", 14, 3, 24},
		{"truncated, match survives", "mixdown_final.mp3", 0, 3, 8},
		{"truncated, match cut away", "mixdown_final.mp3", 14, 3, 8},
		{"match spans the truncation point", "mixdown_final.mp3", 6, 6, 9},
		{"name shorter than the cell", "a.txt", 0, 1, 20},
		{"name exactly fills the cell", "a.txt", 0, 5, 5},
		{"single-cell column", "a.txt", 0, 1, 1},
		{"zero-length highlight", "a.txt", 2, 0, 12},
		{"non-ascii name", "café-münchen.txt", 5, 7, 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderNameCell(base, nameStyle, tt.obj, tt.hi, tt.hiLen, tt.w)
			if w := lipgloss.Width(got); w != tt.w {
				t.Errorf("renderNameCell width = %d, want %d: %q", w, tt.w, got)
			}
		})
	}
}
