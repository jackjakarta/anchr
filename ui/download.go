package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// progressInterval is how often the UI samples an in-flight download's counters.
const progressInterval = 100 * time.Millisecond

// transferTrackW is the width (cells) of the transfer bar's progress track.
const transferTrackW = 16

// transfer is the live state of an in-flight download. The download goroutine
// only ever stores into the atomics and the poll tick only ever loads them, so
// they are the whole synchronisation story.
//
// atomic.Int64 embeds noCopy and Model is copied by value on every Update, so
// this is always held by pointer (a value field would fail go vet).
type transfer struct {
	written   atomic.Int64
	total     atomic.Int64
	cancelled atomic.Bool
	name      string // base name of the object, for the bar
	started   time.Time
	cancel    context.CancelFunc
}

func newTransfer(name string, cancel context.CancelFunc) *transfer {
	return &transfer{name: name, started: time.Now(), cancel: cancel}
}

// note is the s3client.ProgressFunc handed to DownloadObject. It runs on the
// download goroutine, hence atomics only.
func (t *transfer) note(written, total int64) {
	t.written.Store(written)
	t.total.Store(total)
}

// abort cancels the in-flight request. The download cmd then returns a
// FileDownloadedMsg whose Cancelled flag is read back off this struct, so the
// status line says "cancelled" instead of "context canceled".
func (t *transfer) abort() {
	t.cancelled.Store(true)
	if t.cancel != nil {
		t.cancel()
	}
}

// pollProgress samples the counters once, on a timer. Update re-arms it while
// the download is still running.
func pollProgress(t *transfer) tea.Cmd {
	return tea.Tick(progressInterval, func(time.Time) tea.Msg {
		return DownloadProgressMsg{Written: t.written.Load(), Total: t.total.Load()}
	})
}

// ── Destination paths ───────────────────────────────────────────────

// defaultDownloadPath is what the save prompt is prefilled with:
// $XDG_DOWNLOAD_DIR, else ~/Downloads when it exists, else the home directory,
// else the bare name (relative to the working directory).
func defaultDownloadPath(name string) string {
	if dir := os.Getenv("XDG_DOWNLOAD_DIR"); dir != "" {
		return filepath.Join(dir, name)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return name
	}
	if dl := filepath.Join(home, "Downloads"); isDir(dl) {
		return filepath.Join(dl, name)
	}
	return filepath.Join(home, name)
}

// expandPath cleans a user-typed path and expands a leading ~. $VAR is left
// alone on purpose — a literal $ is legal in a filename.
func expandPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errors.New("path required")
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return filepath.Clean(p), nil
}

// resolveDest turns the typed path into the file to write. A path naming an
// existing directory gets the object's name appended; a missing parent
// directory is an error (reported inline, so no download is started); an
// existing file asks for confirmation before it is overwritten.
func resolveDest(p, name string) (dest string, needsConfirm bool, err error) {
	dest, err = expandPath(p)
	if err != nil {
		return "", false, err
	}
	if isDir(dest) {
		dest = filepath.Join(dest, name)
	}
	if dir := filepath.Dir(dest); !isDir(dir) {
		return "", false, fmt.Errorf("no such directory: %s", dir)
	}
	if fi, err := os.Stat(dest); err == nil && !fi.IsDir() {
		return dest, true, nil
	}
	return dest, false, nil
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// ── Progress math & formatting ──────────────────────────────────────

// percentOf is the completed percentage, floored so it only reads 100 when the
// transfer really is done.
func percentOf(written, total int64) int {
	if total <= 0 || written <= 0 {
		return 0
	}
	if written >= total {
		return 100
	}
	return int(written * 100 / total)
}

// fillCells is how many of the track's cells are painted as filled.
func fillCells(written, total int64, trackW int) int {
	if trackW <= 0 || total <= 0 || written <= 0 {
		return 0
	}
	if written >= total {
		return trackW
	}
	n := int(float64(written)/float64(total)*float64(trackW) + 0.5)
	if n >= trackW {
		n = trackW - 1 // a full track means finished, so hold one cell back
	}
	return n
}

// formatETA renders a remaining duration compactly ("—" when unknown).
func formatETA(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	s := int(d.Round(time.Second).Seconds())
	switch {
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm%02ds", s/60, s%60)
	default:
		return fmt.Sprintf("%dh%02dm", s/3600, (s%3600)/60)
	}
}

// ── The transfer bar ────────────────────────────────────────────────

// renderTransferBar draws the download bar shown while a transfer is in flight:
// a determinate track with percentage, throughput and ETA, falling back to the
// old sweeping block when the store didn't report a Content-Length.
//
// Segment order is deliberate — fitBar truncates from the right, so on a narrow
// terminal the least important information is what disappears first.
func (m Model) renderTransferBar() string {
	t := m.transfer
	if t == nil {
		return fitBar(transferBarStyle, "", m.width)
	}

	name := t.name
	if name == "" {
		name = "download"
	}

	var track, tail string
	switch {
	case t.cancelled.Load():
		track = m.sweepTrack(transferTrackW)
		tail = transferText.Render("Cancelling…")
	case m.dlTotal > 0:
		track = fillTrack(fillCells(m.dlWritten, m.dlTotal, transferTrackW), transferTrackW)
		tail = transferPct.Render(fmt.Sprintf("%3d%%", percentOf(m.dlWritten, m.dlTotal)))
		if m.dlRate > 0 {
			tail += transferBarStyle.Render("  ") + transferMeta.Render(formatSize(int64(m.dlRate))+"/s")
		}
		if m.dlETA > 0 {
			tail += transferBarStyle.Render("  ") + transferMeta.Render("ETA "+formatETA(m.dlETA))
		}
	default:
		track = m.sweepTrack(transferTrackW)
		tail = transferText.Render("Downloading…")
	}

	bar := transferBarStyle.Render(" ") + transferIcon.Render("⬇") + transferBarStyle.Render(" ") +
		transferName.Render(truncate(name, 28)) + transferBarStyle.Render("  ") +
		track + transferBarStyle.Render("  ") + tail + transferBarStyle.Render("  ") +
		transferHint.Render("^x cancel")
	return fitBar(transferBarStyle, bar, m.width)
}

// fillTrack paints the first filled cells of a trackW-wide track.
func fillTrack(filled, trackW int) string {
	var sb strings.Builder
	for i := 0; i < trackW; i++ {
		if i < filled {
			sb.WriteString(transferBlock.Render("█"))
		} else {
			sb.WriteString(transferTrack.Render("░"))
		}
	}
	return sb.String()
}

// sweepTrack is the indeterminate fallback: a block window sweeping across the
// track, advanced by the global spinner tick.
func (m Model) sweepTrack(trackW int) string {
	const blockW = 4
	pos := 0
	if steps := trackW - blockW + 1; steps > 0 {
		pos = m.browser.tickCount % steps
	}
	var sb strings.Builder
	for i := 0; i < trackW; i++ {
		if i >= pos && i < pos+blockW {
			sb.WriteString(transferBlock.Render("█"))
		} else {
			sb.WriteString(transferTrack.Render("░"))
		}
	}
	return sb.String()
}
