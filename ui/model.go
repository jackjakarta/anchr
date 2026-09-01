package ui

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jackjakarta/anchr/config"
	"github.com/jackjakarta/anchr/s3client"
)

// presignExpiry is how long generated presigned GET URLs stay valid.
const presignExpiry = time.Hour

type focus int

const (
	focusSidebar focus = iota
	focusBrowser
)

// layoutState holds the responsive widths/flags resolved by updateLayout so
// View doesn't have to recompute the breakpoint math.
type layoutState struct {
	sidebarW    int
	previewW    int
	showPreview bool
}

type Model struct {
	sidebar       sidebar
	browser       browser
	preview       preview
	savePrompt    savePrompt
	focus         focus
	clients       []*s3client.Client
	configs       []config.BucketConfig
	width         int
	height        int
	status        string
	layout        layoutState
	openBucketIdx int // index of the bucket whose objects are loaded (-1 = none)

	// In-flight download. transfer is nil when idle; the dl* fields are the
	// latest sample from DownloadProgressMsg, kept here so View stays a pure
	// function of the model (no time.Since at render time).
	transfer  *transfer
	dlWritten int64
	dlTotal   int64
	dlRate    float64 // bytes/sec, averaged over the transfer so far
	dlETA     time.Duration
}

func NewModel(cfg *config.Config, clients []*s3client.Client) Model {
	names := make([]string, len(cfg.Buckets))
	for i, b := range cfg.Buckets {
		names[i] = b.Name
	}

	return Model{
		sidebar:       newSidebar(names),
		browser:       newBrowser(),
		savePrompt:    newSavePrompt(),
		focus:         focusSidebar,
		clients:       clients,
		configs:       cfg.Buckets,
		openBucketIdx: -1,
	}
}

func (m Model) Init() tea.Cmd {
	return m.browser.spinner.Tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		return m, nil

	case spinner.TickMsg:
		m.browser.tickCount++ // drives the indeterminate transfer bar
		var cmd tea.Cmd
		m.browser.spinner, cmd = m.browser.spinner.Update(msg)
		return m, cmd

	case ObjectsLoadedMsg:
		if msg.Err != nil {
			m.browser.setError(msg.Err)
		} else {
			m.browser.setItems(msg.Result)
		}
		return m, nil

	case DownloadProgressMsg:
		if m.transfer == nil {
			return m, nil // a sample that outlived its download
		}
		m.dlWritten, m.dlTotal = msg.Written, msg.Total
		if elapsed := time.Since(m.transfer.started).Seconds(); elapsed > 0 && msg.Written > 0 {
			m.dlRate = float64(msg.Written) / elapsed
			if msg.Total > msg.Written && m.dlRate > 0 {
				m.dlETA = time.Duration(float64(msg.Total-msg.Written) / m.dlRate * float64(time.Second))
			} else {
				m.dlETA = 0
			}
		}
		return m, pollProgress(m.transfer)

	case FileDownloadedMsg:
		m.transfer = nil
		m.dlWritten, m.dlTotal, m.dlRate, m.dlETA = 0, 0, 0, 0
		m.updateLayout() // give the content row back
		switch {
		case msg.Cancelled:
			m.status = "Download cancelled"
		case msg.Err != nil:
			m.status = fmt.Sprintf("Download failed: %s", msg.Err)
		default:
			m.status = fmt.Sprintf("Downloaded to %s", msg.DestPath)
		}
		return m, nil

	case PresignedURLGeneratedMsg:
		if msg.Err != nil {
			m.status = fmt.Sprintf("Presign failed: %s", msg.Err)
			return m, nil
		}
		if err := clipboard.WriteAll(msg.URL); err != nil {
			m.status = fmt.Sprintf("Copy failed: %s", err)
			return m, nil
		}
		m.status = "Presigned URL copied to clipboard (valid 1h)"
		return m, nil

	case ObjectPreviewLoadedMsg:
		if msg.Err != nil {
			m.preview.setError(msg.Err)
		} else {
			m.preview.setContent(msg.Content, msg.ContentType)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any keypress dismisses a lingering status message.
	m.status = ""

	// While the preview popup is open it captures all keys so they don't leak
	// to the browser underneath.
	if m.preview.active {
		switch {
		case msg.String() == "ctrl+c":
			return m, tea.Quit
		case key.Matches(msg, keys.Up):
			m.preview.scrollUp()
		case key.Matches(msg, keys.Down):
			m.preview.scrollDown(m.contentHeight())
		case key.Matches(msg, keys.Back), key.Matches(msg, keys.Preview), key.Matches(msg, keys.Quit):
			m.preview.close()
		}
		return m, nil
	}

	// The save prompt captures every key the same way, so a path can contain
	// s/y/D/q without firing the browser actions underneath.
	if m.savePrompt.active {
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			m.savePrompt.close()
			return m, nil
		case tea.KeyEnter:
			dest, ok := m.savePrompt.resolve()
			if !ok {
				return m, nil // error or overwrite confirmation; stay open
			}
			clientIdx, key := m.savePrompt.clientIdx, m.savePrompt.key
			m.savePrompt.close()
			return m.startTransfer(clientIdx, key, dest)
		}
		return m, m.savePrompt.update(msg)
	}

	// While the `/` input has focus it captures every key, so nothing leaks to
	// the browser actions underneath — s, y, D, q and h/l would all fire
	// mid-word otherwise.
	if m.browser.filtering {
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyRunes, tea.KeySpace:
			// A lone space arrives as KeySpace, not KeyRunes, but still carries
			// Runes; alt-modified runes are not filter input.
			if !msg.Alt {
				m.browser.filterAppend(string(msg.Runes))
			}
		case tea.KeyBackspace:
			m.browser.filterBackspace()
		case tea.KeyCtrlU:
			m.browser.setFilter("")
		case tea.KeyEsc:
			m.browser.cancelFilter()
		case tea.KeyEnter:
			m.browser.commitFilter()
		case tea.KeyUp:
			m.browser.cursorUp()
		case tea.KeyDown:
			m.browser.cursorDown()
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, keys.CancelDL):
		if m.transfer != nil {
			m.transfer.abort()
		}
		return m, nil

	case key.Matches(msg, keys.Quit):
		if m.transfer != nil {
			// Give DownloadObject's cleanup a chance to remove the .part file
			// before the process goes away.
			m.transfer.abort()
		}
		return m, tea.Quit

	case key.Matches(msg, keys.Tab):
		if m.focus == focusSidebar {
			m.focus = focusBrowser
			m.sidebar.focused = false
			m.browser.focused = true
		} else {
			m.focus = focusSidebar
			m.sidebar.focused = true
			m.browser.focused = false
		}
		return m, nil

	case key.Matches(msg, keys.Left):
		m.focus = focusSidebar
		m.sidebar.focused = true
		m.browser.focused = false
		return m, nil

	case key.Matches(msg, keys.Right):
		m.focus = focusBrowser
		m.sidebar.focused = false
		m.browser.focused = true
		return m, nil

	case key.Matches(msg, keys.Up):
		if m.focus == focusSidebar {
			m.sidebar.cursorUp()
		} else {
			m.browser.cursorUp()
		}
		return m, nil

	case key.Matches(msg, keys.Down):
		if m.focus == focusSidebar {
			m.sidebar.cursorDown()
		} else {
			m.browser.cursorDown()
		}
		return m, nil

	case key.Matches(msg, keys.Enter):
		if m.focus == focusSidebar {
			return m.selectBucket()
		}
		return m.openItem()

	case msg.Type == tea.KeyEsc && m.focus == focusBrowser && m.browser.filter != "":
		// First esc drops the filter, a second one navigates up. keys.Back also
		// binds "h" and backspace, which must keep going up unconditionally.
		m.browser.clearFilter()
		return m, nil

	case key.Matches(msg, keys.Back):
		if m.focus == focusBrowser {
			return m.goBack()
		}
		return m, nil

	case key.Matches(msg, keys.Download):
		if m.focus == focusBrowser {
			return m.startDownload()
		}
		return m, nil

	case key.Matches(msg, keys.CopyKey):
		if m.focus == focusBrowser {
			return m.copyToClipboard(false)
		}
		return m, nil

	case key.Matches(msg, keys.CopyURI):
		if m.focus == focusBrowser {
			return m.copyToClipboard(true)
		}
		return m, nil

	case key.Matches(msg, keys.PresignURL):
		if m.focus == focusBrowser {
			return m.startPresign()
		}
		return m, nil

	case key.Matches(msg, keys.Preview):
		if m.focus == focusBrowser {
			return m.startPreview()
		}
		return m, nil

	case key.Matches(msg, keys.Filter):
		if m.focus == focusBrowser {
			m.browser.startFilter()
		}
		return m, nil

	case key.Matches(msg, keys.Sort):
		if m.focus == focusBrowser {
			m.browser.cycleSort()
		}
		return m, nil

	case key.Matches(msg, keys.SortReverse):
		if m.focus == focusBrowser {
			m.browser.toggleReverse()
		}
		return m, nil
	}

	return m, nil
}

func (m Model) selectBucket() (tea.Model, tea.Cmd) {
	if len(m.clients) == 0 {
		return m, nil
	}
	idx := m.sidebar.cursor
	client := m.clients[idx]
	prefix := client.InitialPrefix()

	m.openBucketIdx = idx
	m.browser.bucket = m.configs[idx].Bucket
	m.browser.prefix = prefix
	m.browser.prefixStack = nil
	m.browser.loading = true
	m.browser.err = nil
	m.browser.items = nil
	m.browser.cursor = 0
	m.browser.offset = 0

	return m, m.loadObjects(idx, prefix)
}

func (m Model) openItem() (tea.Model, tea.Cmd) {
	item, ok := m.browser.selectedItem()
	if !ok {
		return m, nil
	}

	if item.Name == "../" {
		return m.goBack()
	}

	if item.IsDir {
		m.browser.enterFolder(item.Key)
		idx := m.sidebar.cursor
		return m, m.loadObjects(idx, item.Key)
	}

	return m, nil
}

func (m Model) goBack() (tea.Model, tea.Cmd) {
	prefix, ok := m.browser.goBack()
	if !ok {
		return m, nil
	}
	idx := m.sidebar.cursor
	return m, m.loadObjects(idx, prefix)
}

func (m Model) loadObjects(clientIdx int, prefix string) tea.Cmd {
	client := m.clients[clientIdx]
	return func() tea.Msg {
		result, err := client.ListObjects(context.Background(), prefix)
		if err != nil {
			return ObjectsLoadedMsg{Err: err}
		}
		return ObjectsLoadedMsg{Result: result}
	}
}

// startDownload opens the save-path prompt for the selected object. The prompt
// is in-TUI on every platform — there is no native save panel any more.
func (m Model) startDownload() (tea.Model, tea.Cmd) {
	item, ok := m.browser.selectedItem()
	if !ok || item.IsDir || item.Name == "../" {
		return m, nil
	}
	if m.transfer != nil {
		m.status = "A download is already in progress"
		return m, nil
	}
	m.status = ""
	m.savePrompt.open(m.sidebar.cursor, item.Key, path.Base(item.Key))
	return m, nil
}

// startTransfer kicks off the download itself plus the progress poll.
func (m Model) startTransfer(clientIdx int, key, destPath string) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	t := newTransfer(path.Base(key), cancel)
	m.transfer = t
	m.dlWritten, m.dlTotal, m.dlRate, m.dlETA = 0, 0, 0, 0
	m.updateLayout() // the transfer bar steals a content row
	return m, tea.Batch(m.downloadFile(ctx, clientIdx, key, destPath, t), pollProgress(t))
}

func (m Model) downloadFile(ctx context.Context, clientIdx int, key, destPath string, t *transfer) tea.Cmd {
	client := m.clients[clientIdx]
	return func() tea.Msg {
		err := client.DownloadObject(ctx, key, destPath, t.note)
		if err != nil && t.cancelled.Load() {
			return FileDownloadedMsg{DestPath: destPath, Cancelled: true}
		}
		return FileDownloadedMsg{DestPath: destPath, Err: err}
	}
}

// copyToClipboard yanks the selected object's key (or s3://bucket/key when uri
// is true) to the system clipboard.
func (m Model) copyToClipboard(uri bool) (tea.Model, tea.Cmd) {
	item, ok := m.browser.selectedItem()
	if !ok || item.IsDir || item.Name == "../" {
		return m, nil
	}
	text, label := item.Key, "key"
	if uri {
		text = fmt.Sprintf("s3://%s/%s", m.browser.bucket, item.Key)
		label = "S3 URI"
	}
	if err := clipboard.WriteAll(text); err != nil {
		m.status = fmt.Sprintf("Copy failed: %s", err)
		return m, nil
	}
	m.status = fmt.Sprintf("Copied %s to clipboard", label)
	return m, nil
}

func (m Model) startPresign() (tea.Model, tea.Cmd) {
	item, ok := m.browser.selectedItem()
	if !ok || item.IsDir || item.Name == "../" {
		return m, nil
	}
	m.status = "Generating presigned URL..."
	return m, m.presignURL(m.sidebar.cursor, item.Key)
}

func (m Model) presignURL(clientIdx int, key string) tea.Cmd {
	client := m.clients[clientIdx]
	return func() tea.Msg {
		url, err := client.PresignGetObject(context.Background(), key, presignExpiry)
		return PresignedURLGeneratedMsg{URL: url, Err: err}
	}
}

func (m Model) startPreview() (tea.Model, tea.Cmd) {
	item, ok := m.browser.selectedItem()
	if !ok || item.IsDir || item.Name == "../" {
		return m, nil
	}
	m.preview.open(item.Name)
	return m, m.loadPreview(m.sidebar.cursor, item.Key)
}

func (m Model) loadPreview(clientIdx int, key string) tea.Cmd {
	client := m.clients[clientIdx]
	return func() tea.Msg {
		content, contentType, err := client.PreviewObject(context.Background(), key, previewMaxBytes)
		return ObjectPreviewLoadedMsg{Content: content, ContentType: contentType, Err: err}
	}
}

// transferBarVisible reports whether the transfer bar row is currently shown
// (a download is in flight).
func (m Model) transferBarVisible() bool {
	return m.transfer != nil
}

// contentHeight is the screen height minus the top bar, status bar, and (when a
// download is in flight) the transfer bar.
func (m Model) contentHeight() int {
	chrome := 2 // top bar + status bar
	if m.transferBarVisible() {
		chrome++
	}
	h := m.height - chrome
	if h < 1 {
		h = 1
	}
	return h
}

// updateLayout resolves responsive widths into m.layout and propagates the
// content height/width to the sub-views.
//
//	width >= 90        three columns (sidebar | list | preview)
//	70 <= width < 90   preview hidden
//	width < 70         preview hidden + narrow sidebar
func (m *Model) updateLayout() {
	ch := m.contentHeight()

	sideW := sidebarWidth
	if m.width < narrowSidebarBelow {
		sideW = sidebarWidthNarrow
	}
	previewW := previewPaneWidth
	showPreview := m.width >= hidePreviewBelow
	if !showPreview {
		previewW = 0
	}

	listW := m.width - sideW - previewW
	if listW < 10 {
		listW = 10
	}

	m.layout = layoutState{sidebarW: sideW, previewW: previewW, showPreview: showPreview}
	m.sidebar.width = sideW
	m.sidebar.height = ch
	m.browser.width = listW
	m.browser.height = ch
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var sb strings.Builder
	ch := m.contentHeight()

	sb.WriteString(m.renderTopBar())
	sb.WriteString("\n")

	switch {
	case m.savePrompt.active:
		// The save prompt overlays the three-pane area, like the preview popup.
		sb.WriteString(m.savePrompt.View(m.width, ch))
	case m.preview.active:
		// The content popup overlays the three-pane area while open.
		sb.WriteString(m.preview.View(m.width, ch, m.browser.spinner.View()))
	default:
		sb.WriteString(m.renderContent(ch))
	}
	sb.WriteString("\n")

	if m.transferBarVisible() {
		sb.WriteString(m.renderTransferBar())
		sb.WriteString("\n")
	}

	sb.WriteString(m.renderStatusBar())

	return sb.String()
}

// renderContent joins the sidebar, file list, and (when wide enough) the
// metadata pane into the main content area.
func (m Model) renderContent(ch int) string {
	openCount := 0
	if m.openBucketIdx >= 0 {
		openCount = m.browser.fileCount()
	}
	sideView := sidebarStyle.Width(m.layout.sidebarW).Height(ch).MaxWidth(m.layout.sidebarW).
		Render(m.sidebar.View(m.openBucketIdx, openCount))
	listView := listStyle.Width(m.browser.width).Height(ch).MaxWidth(m.browser.width).
		Render(m.browser.View())

	if !m.layout.showPreview {
		return lipgloss.JoinHorizontal(lipgloss.Top, sideView, listView)
	}

	item, ok := m.browser.selectedItem()
	pane := renderMetaPane(item, ok, m.browser.loading, m.layout.previewW, ch)
	previewView := metaPaneStyle.Width(m.layout.previewW).Height(ch).MaxWidth(m.layout.previewW).Render(pane)
	return lipgloss.JoinHorizontal(lipgloss.Top, sideView, listView, previewView)
}

// renderTopBar draws the "anchr" pill, the breadcrumb, and the region cluster.
func (m Model) renderTopBar() string {
	p := pill("anchr")

	right := ""
	if m.openBucketIdx >= 0 && m.openBucketIdx < len(m.configs) {
		if region := m.configs[m.openBucketIdx].Region; region != "" {
			right = topDot.Render("● ") + topInfo.Render(region)
		}
	}

	avail := m.width - lipgloss.Width(p) - lipgloss.Width(right) - 3 // 1 pad + 1 space + 1 pad
	if avail < 1 {
		avail = 1
	}
	crumb := m.renderBreadcrumb(avail)
	if cw := lipgloss.Width(crumb); cw < avail {
		crumb += darkBase.Render(strings.Repeat(" ", avail-cw))
	}

	bar := darkBase.Render(" ") + p + darkBase.Render(" ") + crumb + right + darkBase.Render(" ")
	return fitBar(topBarStyle, bar, m.width)
}

// fitBar forces a single-line bar to exactly width cells: pad short bars with
// the bar's background, truncate long ones. Using lipgloss Width() would
// instead soft-wrap an overflowing bar onto a second line.
func fitBar(style lipgloss.Style, bar string, width int) string {
	if w := lipgloss.Width(bar); w < width {
		return bar + style.Render(strings.Repeat(" ", width-w))
	}
	return style.MaxWidth(width).Render(bar)
}

// renderBreadcrumb builds the bucket/path breadcrumb, collapsing the middle and
// truncating the tail to fit maxW. Truncation decisions are made on plain text
// (rune widths) and styles are applied to the surviving pieces.
func (m Model) renderBreadcrumb(maxW int) string {
	if m.browser.bucket == "" {
		return crumbHashed.Render(truncate("no bucket selected", maxW))
	}

	segs := []string{m.browser.bucket}
	for _, p := range strings.Split(strings.TrimSuffix(m.browser.prefix, "/"), "/") {
		if p != "" {
			segs = append(segs, maybeEllipsize(p))
		}
	}

	plainW := func(ss []string) int {
		w := 0
		for _, s := range ss {
			w += lipgloss.Width(s)
		}
		if len(ss) > 1 {
			w += (len(ss) - 1) * 3 // " / " separators
		}
		return w
	}
	styleSeg := func(i, n int, s string) string {
		switch {
		case i == 0:
			return crumbBucket.Render(s)
		case i == n-1:
			return crumbCurrent.Render(s)
		default:
			return crumbFolder.Render(s)
		}
	}
	sep := crumbSep.Render(" / ")

	if plainW(segs) <= maxW {
		parts := make([]string, len(segs))
		for i, s := range segs {
			parts[i] = styleSeg(i, len(segs), s)
		}
		return strings.Join(parts, sep)
	}

	// Collapse the middle to an ellipsis, keeping bucket + current.
	last := segs[len(segs)-1]
	prefix := crumbBucket.Render(segs[0]) + sep + crumbHashed.Render("…") + sep
	budget := maxW - lipgloss.Width(segs[0]) - 3 - 1 - 3 // bucket + " / " + "…" + " / "
	if budget < 1 {
		return crumbBucket.Render(truncate(segs[0], maxW))
	}
	return prefix + crumbCurrent.Render(truncate(last, budget))
}

// maybeEllipsize shortens hash-like path segments (e.g. UUIDs) for the crumb.
func maybeEllipsize(s string) string {
	r := []rune(s)
	if len(r) > 16 {
		return string(r[:6]) + "…" + string(r[len(r)-4:])
	}
	return s
}

// renderStatusBar draws the key hints (left) and "item N of M · size" (right).
func (m Model) renderStatusBar() string {
	var left string
	switch {
	case m.browser.filtering:
		left = m.renderFilterInput()
	case m.browser.filter != "":
		left = m.renderFilterChip() + statusLabel.Render(" ") + m.statusHints()
	case m.savePrompt.active:
		left = statusLabel.Render(" ") +
			keyChip(keyNav, "⏎", "save") + statusLabel.Render("  ") +
			keyChip(keyNav, "esc", "cancel") + statusLabel.Render("  ") +
			keyChip(keyAction, "^u", "clear")
	case m.preview.active:
		left = statusLabel.Render(" ") +
			keyChip(keyNav, "↑↓/jk", "scroll") + statusLabel.Render("  ") +
			keyChip(keyNav, "esc/q/p", "close")
	case m.status != "":
		left = statusLabel.Render(" " + m.status)
	default:
		left = m.statusHints()
	}

	right := ""
	if !m.preview.active && !m.savePrompt.active &&
		m.browser.bucket != "" && !m.browser.loading && m.browser.err == nil {
		pos, total := m.browser.statusPosition()
		right = statusRight.Render(
			fmt.Sprintf("item %d of %d · %s ", pos, total, formatSize(m.browser.totalSize())),
		)
	}

	// The right-side info is short and useful, so keep it and truncate the hints
	// when the two would collide (leaving at least a 1-cell gap).
	rightW := lipgloss.Width(right)
	leftMax := m.width - rightW - 1
	if leftMax < 0 {
		leftMax = 0
	}
	left = lipgloss.NewStyle().MaxWidth(leftMax).Render(left)
	gap := m.width - lipgloss.Width(left) - rightW
	if gap < 0 {
		gap = 0
	}
	bar := left + statusLabel.Render(strings.Repeat(" ", gap)) + right
	return fitBar(statusBarStyle, bar, m.width)
}

func (m Model) statusHints() string {
	chips := []string{
		keyChip(keyNav, "↑↓", "nav"),
		keyChip(keyNav, "⏎", "open"),
		keyChip(keyNav, "esc", "back"),
		keyChip(keyAction, "/", "filter"),
		keyChip(keyAction, "D", "download"),
		keyChip(keyAction, "p", "preview"),
		keyChip(keyYank, "y/Y", "copy"),
		keyChip(keyYank, "u", "presign"),
		keyChip(keyAction, "s/S", "sort"),
		keyChip(keyNav, "tab", "pane"),
		keyChip(keyQuit, "q", "quit"),
	}
	return statusLabel.Render(" ") + strings.Join(chips, statusLabel.Render("  "))
}

// renderFilterInput draws the live `/` input: the ⌕ glyph, the query, a block
// caret and the match count. It replaces the key hints while typing and flows
// through the same MaxWidth/fitBar truncation, so it cannot overflow the bar.
func (m Model) renderFilterInput() string {
	matched, total := m.browser.matchCount()
	return statusLabel.Render(" ") + filterIcon.Render("⌕ ") +
		filterQuery.Render(m.browser.filter) + filterCaret.Render("▏") +
		filterCount.Render(fmt.Sprintf("  %d/%d", matched, total))
}

// renderFilterChip draws the committed-filter indicator that sits ahead of the
// normal key hints, so an active filter is never invisible.
func (m Model) renderFilterChip() string {
	matched, total := m.browser.matchCount()
	return statusLabel.Render(" ") + filterIcon.Render("⌕ ") +
		filterQuery.Render(m.browser.filter) +
		filterCount.Render(fmt.Sprintf(" %d/%d", matched, total))
}
