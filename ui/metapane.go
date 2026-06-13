package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/jackjakarta/anchr/s3client"
)

// renderMetaPane renders the always-on right-hand metadata pane. It is
// stateless: everything comes from the current selection. width/height are the
// pane's full dimensions; lines are padded to width with the dark bg so the
// panel reads as a solid column. The wrapper (metaPaneStyle) fills any
// remaining height.
func renderMetaPane(item s3client.S3Item, ok, loading bool, width, height int) string {
	iw := width - 2 // 1-cell inset on each side
	if iw < 6 {
		iw = 6
	}

	var lines []string
	add := func(l string) { lines = append(lines, l) }
	blank := func() { add(metaBlank.Width(width).Render("")) }

	switch {
	case loading:
		add(metaRow(width, metaLabelStyle.Render("PREVIEW"), ""))
		blank()
		add(metaRow(width, metaDimStyle.Render("Loading…"), ""))
		return pad(lines, width, height)

	case !ok:
		add(metaRow(width, metaLabelStyle.Render("PREVIEW"), ""))
		blank()
		add(metaRow(width, metaDimStyle.Render("No selection"), ""))
		return pad(lines, width, height)
	}

	k := kindFor(item)

	// Header: PREVIEW + storage-class pill (files with a storage class only).
	right := ""
	if !item.IsDir && item.Name != "../" && item.StorageClass != "" {
		right = storagePillStyle.Render(item.StorageClass)
	}
	add(metaRow(width, metaLabelStyle.Render("PREVIEW"), right))
	blank()

	// "../" parent row — minimal.
	if item.Name == "../" {
		add(metaRow(width, metaPaneIcon.Render("↰")+metaValueStyle.Render(" .."), ""))
		blank()
		add(metaRow(width, metaDimStyle.Render("Parent directory"), ""))
		return pad(lines, width, height)
	}

	// Title: icon + name.
	icon := darkBase.Foreground(k.Color).Render(k.Icon)
	title := truncate(item.Name, iw-2)
	add(metaRow(width, icon+metaPaneTitle.Render(" "+title), ""))
	blank()

	if item.IsDir {
		add(metaKV(width, iw, "Type", "directory"))
		return pad(lines, width, height)
	}

	// Content box (placeholder — the `p` popup reads the real bytes).
	box := metaContentBox.Width(iw - 4).Render("press p to preview")
	for _, bl := range strings.Split(box, "\n") {
		add(metaRow(width, bl, ""))
	}
	blank()

	// Metadata rows.
	add(metaKV(width, iw, "Type", fileType(item)))
	add(metaKV(width, iw, "Size", formatSize(item.Size)+" ("+groupDigits(item.Size)+" B)"))
	if !item.LastModified.IsZero() {
		add(metaKV(width, iw, "Modified", formatModified(item.LastModified)))
	}
	add(metaKVStyled(width, iw, "ETag", dashIfEmpty(item.ETag), metaETagStyle))
	add(metaKV(width, iw, "Storage", dashIfEmpty(item.StorageClass)))

	// Action hints, pinned to the bottom.
	btns := btnDownload.Render("D download") + metaBlank.Render(" ") + btnPresign.Render("u presign")
	for len(lines) < height-1 {
		blank()
	}
	add(metaRow(width, btns, ""))

	return pad(lines, width, height)
}

// metaRow lays out a left segment and an optional right segment within width,
// with a 1-cell dark inset on each side and a dark gap between.
func metaRow(width int, left, right string) string {
	used := lipgloss.Width(left) + lipgloss.Width(right)
	gap := width - 2 - used
	if gap < 0 {
		gap = 0
	}
	return metaBlank.Render(" ") + left +
		metaBlank.Render(strings.Repeat(" ", gap)) + right +
		metaBlank.Render(" ")
}

// metaKV renders a "Label  value" metadata row (value right-aligned).
func metaKV(width, iw int, label, value string) string {
	return metaKVStyled(width, iw, label, value, metaValueStyle)
}

func metaKVStyled(width, iw int, label, value string, valStyle lipgloss.Style) string {
	value = truncate(value, iw-lipgloss.Width(label)-1)
	return metaRow(width, metaLabelStyle.Render(label), valStyle.Render(value))
}

// pad fills the line slice to height with blank dark rows and joins them.
func pad(lines []string, width, height int) string {
	for len(lines) < height {
		lines = append(lines, metaBlank.Width(width).Render(""))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func formatModified(t time.Time) string {
	return t.Format("2006-01-02 15:04")
}

// groupDigits formats n with thousands separators, e.g. 66498560 → "66,498,560".
func groupDigits(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
