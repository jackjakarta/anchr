package ui

import (
	"fmt"
	"strings"
)

type sidebar struct {
	items   []string
	cursor  int
	focused bool
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

func (s *sidebar) visibleRows() int {
	return s.height
}

func (s sidebar) View() string {
	if len(s.items) == 0 {
		return emptyStyle.Render("  No buckets configured")
	}

	var b strings.Builder
	visible := s.visibleRows()
	end := s.offset + visible
	if end > len(s.items) {
		end = len(s.items)
	}

	for i := s.offset; i < end; i++ {
		name := s.items[i]
		if len(name) > sidebarWidth-4 {
			name = name[:sidebarWidth-7] + "..."
		}

		var line string
		if i == s.cursor {
			if s.focused {
				line = sidebarActiveItem.Render(fmt.Sprintf(" > %s", name))
			} else {
				line = sidebarDimActiveItem.Render(fmt.Sprintf(" > %s", name))
			}
		} else {
			if s.focused {
				line = sidebarItem.Render(fmt.Sprintf("   %s", name))
			} else {
				line = sidebarDimItem.Render(fmt.Sprintf("   %s", name))
			}
		}

		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}
