package ui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	sidebarStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderRight(true).
			BorderForeground(lipgloss.Color("240"))

	sidebarActiveItem = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("229"))

	sidebarItem = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	sidebarDimItem = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242"))

	sidebarDimActiveItem = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	browserHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("75"))

	browserItem = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	browserActiveItem = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("229"))

	browserDimItem = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242"))

	browserDimActiveItem = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	dirStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("117"))

	dirDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("60"))

	sizeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("248"))

	dateStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("248"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205"))

	emptyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true)
)

const sidebarWidth = 24
