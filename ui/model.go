package ui

import (
	"context"
	"fmt"
	"os/exec"
	"path"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jackjakarta/anchr/config"
	"github.com/jackjakarta/anchr/s3client"
)

type focus int

const (
	focusSidebar focus = iota
	focusBrowser
)

type Model struct {
	sidebar sidebar
	browser browser
	focus   focus
	clients []*s3client.Client
	configs []config.BucketConfig
	width   int
	height  int
	status  string
}

func NewModel(cfg *config.Config, clients []*s3client.Client) Model {
	names := make([]string, len(cfg.Buckets))
	for i, b := range cfg.Buckets {
		names[i] = b.Name
	}

	return Model{
		sidebar: newSidebar(names),
		browser: newBrowser(),
		focus:   focusSidebar,
		clients: clients,
		configs: cfg.Buckets,
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

	case DownloadPathChosenMsg:
		if msg.Cancelled {
			return m, nil
		}
		if msg.Err != nil {
			m.status = fmt.Sprintf("Download failed: %s", msg.Err)
			return m, nil
		}
		m.browser.downloading = true
		return m, m.downloadFile(msg.ClientIdx, msg.Key, msg.DestPath)

	case FileDownloadedMsg:
		m.browser.downloading = false
		if msg.Err != nil {
			m.status = fmt.Sprintf("Download failed: %s", msg.Err)
		} else {
			m.status = fmt.Sprintf("Downloaded to %s", msg.DestPath)
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

	switch {
	case key.Matches(msg, keys.Quit):
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

func (m Model) startDownload() (tea.Model, tea.Cmd) {
	item, ok := m.browser.selectedItem()
	if !ok || item.IsDir || item.Name == "../" {
		return m, nil
	}
	m.status = ""
	return m, chooseDownloadDest(m.sidebar.cursor, item.Key, path.Base(item.Key))
}

// chooseDownloadDest opens the native macOS save panel via osascript and
// reports the chosen destination path (or a user cancellation).
func chooseDownloadDest(clientIdx int, key, defaultName string) tea.Cmd {
	return func() tea.Msg {
		script := fmt.Sprintf(
			`POSIX path of (choose file name with prompt "Save file as:" `+
				`default name %q default location (path to downloads folder))`,
			defaultName,
		)
		out, err := exec.Command("osascript", "-e", script).Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok && strings.Contains(string(ee.Stderr), "-128") {
				return DownloadPathChosenMsg{Cancelled: true}
			}
			return DownloadPathChosenMsg{Err: err}
		}
		dest := strings.TrimRight(string(out), "\r\n")
		if dest == "" {
			return DownloadPathChosenMsg{Cancelled: true}
		}
		return DownloadPathChosenMsg{ClientIdx: clientIdx, Key: key, DestPath: dest}
	}
}

func (m Model) downloadFile(clientIdx int, key, destPath string) tea.Cmd {
	client := m.clients[clientIdx]
	return func() tea.Msg {
		err := client.DownloadObject(context.Background(), key, destPath)
		return FileDownloadedMsg{DestPath: destPath, Err: err}
	}
}

func (m *Model) updateLayout() {
	// Reserve 2 rows for title bar and status bar
	contentHeight := m.height - 2
	if contentHeight < 1 {
		contentHeight = 1
	}
	m.sidebar.height = contentHeight
	m.browser.height = contentHeight
	m.browser.width = m.width - sidebarWidth - 1 // -1 for border
	if m.browser.width < 10 {
		m.browser.width = 10
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var sb strings.Builder

	// Title bar
	title := titleStyle.Width(m.width).Render("S3 Browser")
	sb.WriteString(title)
	sb.WriteString("\n")

	// Content area
	contentHeight := m.height - 2
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Sidebar
	sideView := m.sidebar.View()
	sideView = sidebarStyle.
		Width(sidebarWidth).
		Height(contentHeight).
		Render(sideView)

	// Browser
	browseView := m.browser.View()
	browseView = lipgloss.NewStyle().
		Width(m.width - sidebarWidth - 2).
		Height(contentHeight).
		Render(browseView)

	content := lipgloss.JoinHorizontal(lipgloss.Top, sideView, browseView)
	sb.WriteString(content)
	sb.WriteString("\n")

	// Status bar
	var statusText string
	switch {
	case m.browser.downloading:
		statusText = " Downloading..."
	case m.status != "":
		statusText = " " + m.status
	default:
		statusText = " ↑↓/jk: navigate  enter/l: open  esc/h: back  D: download  tab/←→: switch pane  q: quit"
	}
	status := statusBarStyle.Width(m.width).Render(statusText)
	sb.WriteString(status)

	return sb.String()
}
