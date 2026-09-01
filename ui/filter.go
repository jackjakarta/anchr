package ui

import (
	"unicode"

	"github.com/jackjakarta/anchr/s3client"
)

// The `/` filter narrows the current listing in memory — ListObjects has
// already drained every page for the prefix, so filtering never touches the
// network. browser.items keeps the full listing and browser.matches holds the
// narrowed projection, which is what lets `esc` restore everything instantly.

// matchIndex reports the rune index at which query occurs within name under
// smart-case rules — an all-lowercase query matches case-insensitively, a query
// containing any uppercase rune matches case-sensitively (the ripgrep/vim
// convention) — or -1 when there is no match. An empty query matches at 0.
//
// Folding is done rune-by-rune with unicode.ToLower, which maps 1:1 on runes,
// so the returned index stays valid against []rune(name) and can be handed
// straight to renderNameCell for highlighting.
func matchIndex(name, query string) int {
	if query == "" {
		return 0
	}
	nr, qr := []rune(name), []rune(query)
	if !hasUpperRune(qr) {
		nr, qr = foldRunes(nr), foldRunes(qr)
	}
	if len(qr) > len(nr) {
		return -1
	}
outer:
	for i := 0; i+len(qr) <= len(nr); i++ {
		for j, q := range qr {
			if nr[i+j] != q {
				continue outer
			}
		}
		return i
	}
	return -1
}

// hasUpperRune reports whether rs contains an uppercase rune — the smart-case
// switch between an insensitive and a sensitive match.
func hasUpperRune(rs []rune) bool {
	for _, r := range rs {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

// foldRunes returns a lowercased copy of rs. unicode.ToLower is 1:1 on runes,
// so indices are preserved.
func foldRunes(rs []rune) []rune {
	out := make([]rune, len(rs))
	for i, r := range rs {
		out[i] = unicode.ToLower(r)
	}
	return out
}

// rows is the slice the list renders and indexes from: the full listing, or
// the filtered projection while a filter is active. Every consumer of the
// listing (itemCount, selectedItem, renderItem, totalSize, statusPosition,
// restoreCursor) goes through here — fileCount deliberately does not, since
// the sidebar badge describes the bucket rather than the filtered view.
func (b browser) rows() []s3client.S3Item {
	if b.filter == "" {
		return b.items
	}
	return b.matches
}

// matchCount reports the matched and total object counts for the status-bar
// filter indicator.
func (b browser) matchCount() (matched, total int) {
	return len(b.rows()), len(b.items)
}

// applyFilter rebuilds matches from items. It must be called whenever the
// query changes *or* the sort order does — applySort reorders items in place,
// which leaves the previously built matches stale.
func (b *browser) applyFilter() {
	if b.filter == "" {
		b.matches = nil
		return
	}
	m := make([]s3client.S3Item, 0, len(b.items))
	for _, it := range b.items {
		if matchIndex(it.Name, b.filter) >= 0 {
			m = append(m, it)
		}
	}
	b.matches = m
}

// setFilter replaces the query, re-narrows the list, and keeps the cursor on
// the previously selected object. restoreCursor falls back to the first row
// when that object no longer matches.
func (b *browser) setFilter(q string) {
	sel, _ := b.selectedItem()
	b.filter = q
	b.applyFilter()
	b.restoreCursor(sel.Key)

	// restoreCursor falls back to row 0, which is "../" inside a folder. Skip
	// past it onto the first match so D/p/y act on a real object the moment the
	// filter is committed, instead of no-opping on the parent row.
	if b.filter != "" && b.cursor == 0 && b.canGoBack() && len(b.matches) > 0 {
		b.cursor = 1
		b.ensureVisible()
	}
}

func (b *browser) filterAppend(s string) { b.setFilter(b.filter + s) }

func (b *browser) filterBackspace() {
	r := []rune(b.filter)
	if len(r) == 0 {
		return
	}
	b.setFilter(string(r[:len(r)-1]))
}

// startFilter focuses the `/` input, keeping any committed query so `/`
// reopens and extends the last filter rather than discarding it.
func (b *browser) startFilter() { b.filtering = true }

// commitFilter leaves the input but keeps the filter applied, so the browser
// action keys (D, p, y, s…) work again on the narrowed list.
func (b *browser) commitFilter() { b.filtering = false }

// cancelFilter aborts the input and restores the full listing (`esc` while
// typing).
func (b *browser) cancelFilter() {
	b.filtering = false
	b.setFilter("")
}

// clearFilter drops a committed filter — the first `esc` in normal mode, which
// takes precedence over navigating up a level.
func (b *browser) clearFilter() { b.setFilter("") }
