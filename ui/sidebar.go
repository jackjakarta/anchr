package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type sidebar struct {
	items   []string
	cursor  int
	focused bool
	width   int
	height  int
	offset  int
}

func newSidebar(names []string) sidebar {
	return sidebar{
		items:   names,
		focused: true,
	}
}

func (s *sidebar) cursorUp() {
	if s.cursor > 0 {
		s.cursor--
		s.ensureVisible()
	}
}

func (s *sidebar) cursorDown() {
	if s.cursor < len(s.items)-1 {
		s.cursor++
		s.ensureVisible()
	}
}

func (s *sidebar) ensureVisible() {
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	visible := s.visibleRows()
	if visible > 0 && s.cursor >= s.offset+visible {
		s.offset = s.cursor - visible + 1
	}
}

// visibleRows is how many bucket rows fit below the "BUCKETS" header.
func (s *sidebar) visibleRows() int {
	v := s.height - 1 // header row
	if v < 1 {
		v = 1
	}
	return v
}

// View renders the sidebar. openIdx is the index of the bucket whose objects
// are currently loaded (-1 if none); openCount is that bucket's file count,
// shown as a badge on its row only.
func (s sidebar) View(openIdx, openCount int) string {
	w := s.width
	if w < 8 {
		w = sidebarWidth
	}

	if len(s.items) == 0 {
		return sidebarHeader.Width(w).Render(" No buckets")
	}

	var b strings.Builder
	b.WriteString(sidebarHeader.Width(w).Render(" BUCKETS"))
	b.WriteString("\n")

	visible := s.visibleRows()
	end := s.offset + visible
	if end > len(s.items) {
		end = len(s.items)
	}
	for i := s.offset; i < end; i++ {
		b.WriteString(s.renderBucket(i, openIdx, openCount, w))
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (s sidebar) renderBucket(i, openIdx, openCount, w int) string {
	isCursor := i == s.cursor

	base := panelBase
	accent := cYellow
	if isCursor {
		base = elevBase
		if !s.focused {
			accent = cFgMut
		}
	}

	var iconStyle, nameStyle, countStyle lipgloss.Style
	if isCursor {
		iconStyle = base.Foreground(accent)
		countStyle = base.Foreground(accent)
		if s.focused {
			nameStyle = base.Foreground(cFgBri).Bold(true)
		} else {
			nameStyle = base.Foreground(cFg)
		}
	} else {
		iconStyle = base.Foreground(cFgDimr)
		nameStyle = base.Foreground(cFgMut)
		countStyle = base.Foreground(cFgDimr)
	}

	countStr := ""
	if i == openIdx {
		countStr = formatCount(openCount)
	}

	// Column budget: accent(1) + icon(2) + name(flex) + [gap + count].
	avail := w - colAccent - colIcon
	if avail < 1 {
		avail = 1
	}

	var b strings.Builder
	if isCursor {
		b.WriteString(base.Foreground(accent).Render(accentGlyph))
	} else {
		b.WriteString(base.Render(" "))
	}
	b.WriteString(iconStyle.Width(colIcon).Render("▤"))

	if countStr != "" {
		cw := lipgloss.Width(countStr)
		nameMax := avail - cw - 1 // 1-cell gap before the count
		if nameMax < 1 {
			nameMax = 1
		}
		nm := truncate(s.items[i], nameMax)
		gap := avail - lipgloss.Width(nm) - cw
		if gap < 1 {
			gap = 1
		}
		b.WriteString(nameStyle.Render(nm))
		b.WriteString(base.Render(strings.Repeat(" ", gap)))
		b.WriteString(countStyle.Render(countStr))
	} else {
		b.WriteString(nameStyle.Width(avail).Render(truncate(s.items[i], avail)))
	}

	return b.String()
}

// formatCount renders an object count compactly: 128, 2.4k, 5.1M.
func formatCount(n int) string {
	switch {
	case n >= 1_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e6), ".0") + "M"
	case n >= 1_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e3), ".0") + "k"
	default:
		return fmt.Sprintf("%d", n)
	}
}
