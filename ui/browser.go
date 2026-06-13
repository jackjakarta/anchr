package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/jackjakarta/anchr/s3client"
)

type sortKey int

const (
	sortByName sortKey = iota
	sortBySize
	sortByModified
)

// File-list column widths (cells). Fixed columns plus a flexible NAME column.
// rows and the column header share these so everything aligns.
const (
	colAccent = 1  // left accent bar (▌ on cursor, else space)
	colIcon   = 2  // type icon + trailing gap
	colKind   = 6  // KIND badge (centered; max label 4 + Padding(0,1))
	colSize   = 10 // SIZE (right-aligned)
	colDate   = 6  // DATE (right-aligned, "Jan _2")
	colGaps   = 3  // single-cell gaps before KIND, SIZE, DATE
	// total non-name overhead used by nameWidth()
	colFixed = colAccent + colIcon + colKind + colSize + colDate + colGaps
)

type browser struct {
	items       []s3client.S3Item
	cursor      int
	bucket      string
	prefix      string
	prefixStack []string
	focused     bool
	loading     bool
	downloading bool
	err         error
	spinner     spinner.Model
	width       int
	height      int
	offset      int
	sortBy      sortKey
	sortReverse bool
	tickCount   int // advanced on every spinner tick; animates the transfer bar
}

func newBrowser() browser {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = spinnerStyle
	return browser{
		spinner: sp,
	}
}

func (b *browser) setItems(result *s3client.ListResult) {
	b.items = result.Items
	b.bucket = result.Bucket
	b.prefix = result.Prefix
	b.cursor = 0
	b.offset = 0
	b.loading = false
	b.err = nil
	b.applySort()
}

func (b *browser) setError(err error) {
	b.err = err
	b.loading = false
	b.items = nil
}

func (b *browser) cursorUp() {
	if b.cursor > 0 {
		b.cursor--
		b.ensureVisible()
	}
}

func (b *browser) cursorDown() {
	max := b.itemCount() - 1
	if b.cursor < max {
		b.cursor++
		b.ensureVisible()
	}
}

func (b *browser) itemCount() int {
	n := len(b.items)
	if b.canGoBack() {
		n++ // "../" entry
	}
	return n
}

func (b *browser) canGoBack() bool {
	return b.prefix != "" || len(b.prefixStack) > 0
}

// fileCount is the number of non-directory objects in the current listing.
// Backs the sidebar count badge and the status bar's item total.
func (b browser) fileCount() int {
	n := 0
	for _, it := range b.items {
		if !it.IsDir {
			n++
		}
	}
	return n
}

// totalSize sums the byte sizes of the file objects in the current listing.
func (b browser) totalSize() int64 {
	var sum int64
	for _, it := range b.items {
		if !it.IsDir {
			sum += it.Size
		}
	}
	return sum
}

// statusPosition reports the 1-based index of the selected item within items
// (the synthetic "../" yields 0) and the total row count. Used by the status
// bar's "item N of M".
func (b browser) statusPosition() (pos, total int) {
	total = len(b.items)
	sel, ok := b.selectedItem()
	if !ok || sel.Name == "../" {
		return 0, total
	}
	for i, it := range b.items {
		if it.Key == sel.Key {
			return i + 1, total
		}
	}
	return 0, total
}

func (b *browser) selectedItem() (s3client.S3Item, bool) {
	if len(b.items) == 0 && !b.canGoBack() {
		return s3client.S3Item{}, false
	}
	idx := b.cursor
	if b.canGoBack() {
		if idx == 0 {
			return s3client.S3Item{Name: "../", IsDir: true}, true
		}
		idx--
	}
	if idx >= len(b.items) {
		return s3client.S3Item{}, false
	}
	return b.items[idx], true
}

func (b *browser) applySort() {
	sort.SliceStable(b.items, func(i, j int) bool {
		a, c := b.items[i], b.items[j]
		if a.IsDir != c.IsDir {
			return a.IsDir // directories always first
		}
		var less bool
		switch b.sortBy {
		case sortBySize:
			less = a.Size < c.Size
		case sortByModified:
			less = a.LastModified.Before(c.LastModified)
		default: // sortByName
			less = a.Name < c.Name
		}
		if b.sortReverse {
			return !less
		}
		return less
	})
}

func (b *browser) cycleSort() {
	sel, _ := b.selectedItem()
	b.sortBy = (b.sortBy + 1) % 3
	b.applySort()
	b.restoreCursor(sel.Key)
}

func (b *browser) toggleReverse() {
	sel, _ := b.selectedItem()
	b.sortReverse = !b.sortReverse
	b.applySort()
	b.restoreCursor(sel.Key)
}

func (b *browser) restoreCursor(key string) {
	b.cursor = 0
	b.offset = 0
	if key == "" { // was on "../" (synthetic, empty Key)
		return
	}
	base := 0
	if b.canGoBack() {
		base = 1 // account for the "../" entry at index 0
	}
	for i, it := range b.items {
		if it.Key == key {
			b.cursor = base + i
			break
		}
	}
	b.ensureVisible()
}

func (b *browser) enterFolder(prefix string) {
	b.prefixStack = append(b.prefixStack, b.prefix)
	b.prefix = prefix
	b.loading = true
	b.cursor = 0
	b.offset = 0
}

func (b *browser) goBack() (string, bool) {
	if len(b.prefixStack) == 0 {
		if b.prefix != "" {
			prev := b.prefix
			b.prefix = ""
			b.loading = true
			b.cursor = 0
			b.offset = 0
			_ = prev
			return "", true
		}
		return "", false
	}
	prev := b.prefixStack[len(b.prefixStack)-1]
	b.prefixStack = b.prefixStack[:len(b.prefixStack)-1]
	b.prefix = prev
	b.loading = true
	b.cursor = 0
	b.offset = 0
	return prev, true
}

func (b *browser) ensureVisible() {
	headerRows := 1 // column header
	visible := b.height - headerRows
	if visible < 1 {
		visible = 1
	}
	if b.cursor < b.offset {
		b.offset = b.cursor
	}
	if b.cursor >= b.offset+visible {
		b.offset = b.cursor - visible + 1
	}
}

func (b browser) View() string {
	if b.bucket == "" && !b.loading {
		return emptyStyle.Render("  Select a bucket to browse")
	}

	var sb strings.Builder

	if b.loading {
		sb.WriteString(fmt.Sprintf("  %s Loading…", b.spinner.View()))
		return sb.String()
	}

	if b.err != nil {
		sb.WriteString(errorStyle.Render(fmt.Sprintf("  Error: %s", b.err)))
		return sb.String()
	}

	// Column header
	nameW := b.nameWidth()
	sb.WriteString(b.renderHeader(nameW))
	sb.WriteString("\n")

	if b.itemCount() == 0 {
		sb.WriteString(emptyStyle.Render("  (empty)"))
		return sb.String()
	}

	headerRows := 1 // column header
	visible := b.height - headerRows
	if visible < 1 {
		visible = 1
	}
	end := b.offset + visible
	if end > b.itemCount() {
		end = b.itemCount()
	}

	for i := b.offset; i < end; i++ {
		sb.WriteString(b.renderItem(i, nameW))
		if i < end-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// renderHeader draws the column header row: blank icon column, NAME (with the
// active sort arrow), KIND, SIZE, DATE — matching the row column widths.
func (b browser) renderHeader(nameW int) string {
	arrow := "▲"
	if b.sortReverse {
		arrow = "▼"
	}
	nameArrow, sizeArrow, dateArrow := "", "", ""
	switch b.sortBy {
	case sortBySize:
		sizeArrow = arrow
	case sortByModified:
		dateArrow = arrow
	default:
		nameArrow = arrow
	}

	var sb strings.Builder
	sb.WriteString(colHeader.Render(strings.Repeat(" ", colAccent+colIcon))) // accent + icon cols
	sb.WriteString(headerField(nameArrow, nameW, false, "NAME"))
	sb.WriteString(colHeader.Render(" "))
	sb.WriteString(colHeader.Width(colKind).Align(lipgloss.Center).Render("KIND"))
	sb.WriteString(colHeader.Render(" "))
	sb.WriteString(headerField(sizeArrow, colSize, true, "SIZE"))
	sb.WriteString(colHeader.Render(" "))
	sb.WriteString(headerField(dateArrow, colDate, true, "DATE"))
	return sb.String()
}

// headerField renders one header label in width w, left- or right-aligned,
// with the sort arrow (if any) colored yellow and the rest dim.
func headerField(arrow string, w int, right bool, label string) string {
	text := label
	if arrow != "" {
		text = label + " " + arrow
	}
	pad := w - lipgloss.Width(text)
	if pad < 0 {
		pad = 0
	}
	var sb strings.Builder
	if right {
		sb.WriteString(colHeader.Render(strings.Repeat(" ", pad)))
	}
	sb.WriteString(colHeader.Render(label))
	if arrow != "" {
		sb.WriteString(colHeader.Render(" "))
		sb.WriteString(sortArrow.Render(arrow))
	}
	if !right {
		sb.WriteString(colHeader.Render(strings.Repeat(" ", pad)))
	}
	return sb.String()
}

func (b browser) renderItem(index, nameW int) string {
	isBack := false
	adjustedIndex := index
	if b.canGoBack() {
		if index == 0 {
			isBack = true
		} else {
			adjustedIndex = index - 1
		}
	}

	var item s3client.S3Item
	if isBack {
		item = s3client.S3Item{Name: "../", IsDir: true}
	} else {
		item = b.items[adjustedIndex]
	}

	isCursor := index == b.cursor
	k := kindFor(item)

	// Resolve the per-row palette. Non-cursor rows sit on the app bg; the cursor
	// row sits on the elevated bg with a yellow (focused) or gray (unfocused)
	// accent. Every cell style derives from base so it carries the row bg.
	base := rowBlank
	accent := cYellow
	if isCursor {
		base = elevBase
		if !b.focused {
			accent = cFgMut
		}
	}

	// Column contents -------------------------------------------------
	dash := item.IsDir || isBack
	var sizeStr, dateStr string
	if item.IsDir {
		sizeStr = "—"
		if !isBack && !item.LastModified.IsZero() {
			dateStr = formatDate(item.LastModified)
		}
	} else {
		sizeStr = formatSize(item.Size)
		if !item.LastModified.IsZero() {
			dateStr = formatDate(item.LastModified)
		}
	}

	name := truncate(item.Name, nameW)
	icon := k.Icon
	if isBack {
		icon = "↰"
	}

	// Per-cell styles (all carry the row bg) --------------------------
	iconStyle := base.Foreground(k.Color)
	nameStyle := base.Foreground(cFg)
	sizeStyle := base.Foreground(cFgMut)
	dateStyle := base.Foreground(cFgMut)
	dashStyle := base.Foreground(cBordDim)
	var badgeStr string

	switch {
	case isCursor:
		iconStyle = base.Foreground(accent)
		nameStyle = base.Foreground(accent).Bold(b.focused)
		sizeStyle = base.Foreground(accent)
		dateStyle = base.Foreground(accent).Faint(true)
		if !isBack {
			badgeStr = badge(k.Label, cOnAcc, accent) // filled with the accent
		}
	case isBack:
		iconStyle = base.Foreground(cFgDim)
		nameStyle = base.Foreground(cFgDim)
	case item.IsDir:
		nameStyle = base.Foreground(cAqua)
		badgeStr = badge(k.Label, k.Color, cElevBg)
	default:
		badgeStr = badge(k.Label, k.Color, cElevBg)
	}

	// Assemble fixed-width, bg-styled fields (no bare spacers) ---------
	var sb strings.Builder
	if isCursor {
		sb.WriteString(base.Foreground(accent).Render(accentGlyph))
	} else {
		sb.WriteString(base.Render(" "))
	}
	sb.WriteString(iconStyle.Width(colIcon).Render(icon))
	sb.WriteString(nameStyle.Width(nameW).Render(name))
	sb.WriteString(base.Render(" "))
	sb.WriteString(kindCell(base, badgeStr))
	sb.WriteString(base.Render(" "))
	if dash {
		sb.WriteString(dashStyle.Width(colSize).Align(lipgloss.Right).Render(sizeStr))
	} else {
		sb.WriteString(sizeStyle.Width(colSize).Align(lipgloss.Right).Render(sizeStr))
	}
	sb.WriteString(base.Render(" "))
	sb.WriteString(dateStyle.Width(colDate).Align(lipgloss.Right).Render(dateStr))

	return sb.String()
}

// kindCell centers a (possibly empty) badge within the KIND column, padding the
// remainder with the row background.
func kindCell(base lipgloss.Style, badgeStr string) string {
	bw := lipgloss.Width(badgeStr)
	if bw > colKind {
		bw = colKind
	}
	lp := (colKind - bw) / 2
	rp := colKind - bw - lp
	return base.Render(strings.Repeat(" ", lp)) + badgeStr + base.Render(strings.Repeat(" ", rp))
}

func (b browser) nameWidth() int {
	w := b.width - colFixed
	if w < 6 {
		w = 6
	}
	return w
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatDate(t time.Time) string {
	return t.Format("Jan _2")
}
