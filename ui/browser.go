package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/jackjakarta/anchr/s3client"
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

	// Path header
	path := b.bucket
	if b.prefix != "" {
		path += " / " + b.prefix
	}
	if len(path) > b.width-2 {
		path = "..." + path[len(path)-b.width+5:]
	}
	sb.WriteString(browserHeader.Render(path))
	sb.WriteString("\n")

	if b.loading {
		sb.WriteString(fmt.Sprintf("\n  %s Loading...", b.spinner.View()))
		return sb.String()
	}

	if b.err != nil {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render(fmt.Sprintf("  Error: %s", b.err)))
		return sb.String()
	}

	// Column header
	nameW := b.nameWidth()
	header := fmt.Sprintf("  %-*s  %8s  %6s", nameW, "NAME", "SIZE", "DATE")
	sb.WriteString(lipglossRender(header, browserDimItem))
	sb.WriteString("\n")

	if b.itemCount() == 0 {
		sb.WriteString(emptyStyle.Render("  (empty)"))
		return sb.String()
	}

	// Items
	headerRows := 2 // path + column header
	visible := b.height - headerRows
	if visible < 1 {
		visible = 1
	}
	end := b.offset + visible
	if end > b.itemCount() {
		end = b.itemCount()
	}

	for i := b.offset; i < end; i++ {
		line := b.renderItem(i, nameW)
		sb.WriteString(line)
		if i < end-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (b browser) renderItem(index, nameW int) string {
	isBack := false
	var item s3client.S3Item

	adjustedIndex := index
	if b.canGoBack() {
		if index == 0 {
			isBack = true
		} else {
			adjustedIndex = index - 1
		}
	}

	if isBack {
		item = s3client.S3Item{Name: "../", IsDir: true}
	} else {
		item = b.items[adjustedIndex]
	}

	isCursor := index == b.cursor

	name := item.Name
	if len(name) > nameW {
		name = name[:nameW-3] + "..."
	}

	var sizeStr, dateStr string
	if item.IsDir {
		sizeStr = "-"
		dateStr = ""
		if !isBack {
			if !item.LastModified.IsZero() {
				dateStr = formatDate(item.LastModified)
			}
		}
	} else {
		sizeStr = formatSize(item.Size)
		if !item.LastModified.IsZero() {
			dateStr = formatDate(item.LastModified)
		}
	}

	line := fmt.Sprintf("  %-*s  %8s  %6s", nameW, name, sizeStr, dateStr)

	if item.IsDir {
		if isCursor {
			if b.focused {
				return browserActiveItem.Render(line)
			}
			return browserDimActiveItem.Render(line)
		}
		if b.focused {
			return dirStyle.Render(line)
		}
		return dirDimStyle.Render(line)
	}

	if isCursor {
		if b.focused {
			return browserActiveItem.Render(line)
		}
		return browserDimActiveItem.Render(line)
	}
	if b.focused {
		return browserItem.Render(line)
	}
	return browserDimItem.Render(line)
}

func (b browser) nameWidth() int {
	// total width minus padding/columns: "  " + "  " + 8 + "  " + 6
	w := b.width - 22
	if w < 10 {
		w = 10
	}
	return w
}

func lipglossRender(s string, style lipgloss.Style) string {
	return style.Render(s)
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
