package ui

import "github.com/charmbracelet/lipgloss"

// ── Gruvbox (dark) palette ──────────────────────────────────────────
// Reference these named colors everywhere instead of inlining hex. On
// non-truecolor terminals lipgloss downsamples each to the nearest
// 256/16-color and strips color entirely off a TTY — no manual handling.
var (
	cAppBg   = lipgloss.Color("#282828") // app background (file list)
	cDarkBg  = lipgloss.Color("#1d2021") // darker bg (top/status bars, preview)
	cPanelBg = lipgloss.Color("#242424") // sidebar / transfer bar
	cElevBg  = lipgloss.Color("#3c3836") // selected / elevated row + chips
	cBorder  = lipgloss.Color("#3c3836") // default border
	cBordDim = lipgloss.Color("#504945") // dim border / breadcrumb separators

	cFg     = lipgloss.Color("#ebdbb2") // primary fg
	cFgBri  = lipgloss.Color("#fbf1c7") // bright fg
	cFgMut  = lipgloss.Color("#a89984") // muted fg
	cFgDim  = lipgloss.Color("#7c6f64") // dim fg
	cFgDimr = lipgloss.Color("#665c54") // dimmer fg (idle icons)
	cOnAcc  = lipgloss.Color("#282828") // dark text on an accent fill

	cYellow  = lipgloss.Color("#fabd2f")
	cGreen   = lipgloss.Color("#b8bb26")
	cAqua    = lipgloss.Color("#83a598")
	cAquaBri = lipgloss.Color("#8ec07c")
	cOrange  = lipgloss.Color("#fe8019")
	cPurple  = lipgloss.Color("#d3869b")
	cRed     = lipgloss.Color("#fb4934")
)

// ── Layout constants ────────────────────────────────────────────────
const (
	sidebarWidth       = 22 // default sidebar column width
	sidebarWidthNarrow = 16 // sidebar width on narrow terminals
	previewPaneWidth   = 32 // metadata pane column width
	hidePreviewBelow   = 90 // hide the preview pane below this terminal width
	narrowSidebarBelow = 70 // narrow the sidebar below this terminal width

	accentGlyph = "▌" // 1-cell left accent bar (CSS 3px accent → 1 cell)
)

// ── Region bases ────────────────────────────────────────────────────
// Each pane paints its own background; because inner styled segments emit
// their own SGR reset, the background does NOT bleed through from a wrapper.
// So every leaf style derives from its region base to carry the bg, and rows
// are assembled from bg-styled fixed-width fields with no bare spacers.
var (
	appBase   = lipgloss.NewStyle().Background(cAppBg)
	panelBase = lipgloss.NewStyle().Background(cPanelBg)
	darkBase  = lipgloss.NewStyle().Background(cDarkBg)
	elevBase  = lipgloss.NewStyle().Background(cElevBg)
)

// ── Top bar ─────────────────────────────────────────────────────────
var (
	topBarStyle = darkBase // wrapper: Width(m.width) at call site

	pillStyle = lipgloss.NewStyle().
			Bold(true).Foreground(cOnAcc).Background(cYellow).Padding(0, 1)

	crumbBucket  = darkBase.Foreground(cAqua).Bold(true)
	crumbFolder  = darkBase.Foreground(cFgMut)
	crumbHashed  = darkBase.Foreground(cFgDim)
	crumbCurrent = darkBase.Foreground(cFg).Bold(true)
	crumbSep     = darkBase.Foreground(cBordDim)

	topDot  = darkBase.Foreground(cGreen) // ● connection indicator
	topInfo = darkBase.Foreground(cFgDim) // region text
)

// ── Sidebar ─────────────────────────────────────────────────────────
var (
	sidebarStyle  = panelBase // wrapper
	sidebarHeader = panelBase.Foreground(cFgDim)
)

// ── File list ───────────────────────────────────────────────────────
var (
	listStyle = appBase // wrapper

	colHeader = appBase.Foreground(cFgDim)
	sortArrow = appBase.Foreground(cYellow)

	rowBlank = appBase // base for row cells; per-cell fg derived at call site
)

// ── Preview / metadata pane ─────────────────────────────────────────
var (
	metaPaneStyle = darkBase // wrapper; panes separate by bg contrast, no border

	metaLabelStyle = darkBase.Foreground(cFgDim)
	metaPaneTitle  = darkBase.Foreground(cYellow).Bold(true)
	metaPaneIcon   = darkBase.Foreground(cYellow)
	metaValueStyle = darkBase.Foreground(cFg)
	metaETagStyle  = darkBase.Foreground(cFgMut)
	metaDimStyle   = darkBase.Foreground(cFgDim)
	metaBlank      = darkBase

	storagePillStyle = lipgloss.NewStyle().
				Bold(true).Foreground(cOnAcc).Background(cAqua).Padding(0, 1)

	metaContentBox = lipgloss.NewStyle().
			Background(cAppBg).
			Foreground(cFgDim).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(cBorder).
			Padding(0, 1)

	btnDownload = lipgloss.NewStyle().Background(cElevBg).Foreground(cYellow).Padding(0, 1)
	btnPresign  = lipgloss.NewStyle().Background(cElevBg).Foreground(cAqua).Padding(0, 1)
)

// ── Transfer bar (indeterminate) ────────────────────────────────────
var (
	transferBarStyle = panelBase // wrapper
	transferIcon     = panelBase.Foreground(cAquaBri)
	transferName     = panelBase.Foreground(cFgMut)
	transferTrack    = panelBase.Foreground(cFgDimr) // ░
	transferBlock    = panelBase.Foreground(cYellow) // █
	transferText     = panelBase.Foreground(cFgDim)
)

// ── Status bar ──────────────────────────────────────────────────────
var (
	statusBarStyle = darkBase // wrapper
	statusLabel    = darkBase.Foreground(cFgDim)
	statusRight    = darkBase.Foreground(cFgMut)

	keyNav    = darkBase.Foreground(cAqua).Bold(true)    // ↑↓ ⏎ esc
	keyAction = darkBase.Foreground(cYellow).Bold(true)  // D p
	keyYank   = darkBase.Foreground(cAquaBri).Bold(true) // y/Y u
	keyQuit   = darkBase.Foreground(cRed).Bold(true)     // q
)

// ── Misc / states ───────────────────────────────────────────────────
var (
	errorStyle   = appBase.Foreground(cRed).Bold(true)
	spinnerStyle = lipgloss.NewStyle().Foreground(cPurple)
	emptyStyle   = appBase.Foreground(cFgDim).Italic(true)
)

// ── Popup preview (the `p` content overlay) ─────────────────────────
var (
	previewBox = lipgloss.NewStyle().
			Background(cDarkBg).
			Foreground(cFg).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(cBorder).
			Padding(0, 1)

	previewTitle = darkBase.Foreground(cYellow).Bold(true)
	previewMeta  = darkBase.Foreground(cFgDim)
)

// ── Render helpers ──────────────────────────────────────────────────

// badge renders an inline KIND chip: bold label with one bg-colored space of
// padding on each side. Do NOT use Inline(true) — in lipgloss v1.1.0 it skips
// padding. Width is len(label)+2.
func badge(label string, fg, bg lipgloss.Color) string {
	return lipgloss.NewStyle().
		Bold(true).Foreground(fg).Background(bg).Padding(0, 1).
		Render(label)
}

// pill renders the top-bar "anchr" pill.
func pill(label string) string { return pillStyle.Render(label) }

// keyChip renders one status-bar hint: a colored KEY glyph + a dim label.
func keyChip(keyStyle lipgloss.Style, glyph, label string) string {
	return keyStyle.Render(glyph) + statusLabel.Render(" "+label)
}
