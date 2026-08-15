package cli

import "charm.land/lipgloss/v2"

// lightSkyBlue is the primary theme accent color used across TUI components.
var lightSkyBlue = lipgloss.Color("#87CEFA")

// --- Input / footer ---

var inputBoxStyle = lipgloss.NewStyle().
	Border(lipgloss.NormalBorder(), true, false, true, false).
	BorderForeground(lightSkyBlue).
	Padding(0, 1)

var footerModelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E"))
var footerBranchStyle = lipgloss.NewStyle().Foreground(lightSkyBlue)

// --- Message rendering ---

var messageBarStyle = lipgloss.NewStyle().Foreground(lightSkyBlue)
var assistantGlyphStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA"))
var userMessageStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#B8D4E3"))
var toolGlyphSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E"))
var toolGlyphFailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
var toolGlyphPendingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))

// --- Approval panel ---

var approvalPanelStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#F59E0B")).
	Padding(0, 1)

var approvalSelectedStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#F59E0B")).
	Foreground(lipgloss.Color("#000000")).
	Bold(true).
	Padding(0, 2)

var approvalUnselectedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#9CA3AF")).
	Padding(0, 2)

// --- Slash panels ---

var slashPanelTitleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lightSkyBlue)

var slashPanelBoxStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.RoundedBorder()).
	BorderForeground(lightSkyBlue).
	Padding(0, 1)

var slashPopupTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lightSkyBlue)
var slashPopupLineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#B8D4E3"))
var slashPopupSelectedStyle = lipgloss.NewStyle().Foreground(lightSkyBlue).Bold(true)

// --- Diff panel ---

var (
	diffPaneBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, true, false, false).
				BorderForeground(lipgloss.Color("#374151")).
				PaddingRight(1)
	diffSelectedStyle = lipgloss.NewStyle().Foreground(lightSkyBlue).Bold(true)
	diffFocusedStyle  = lipgloss.NewStyle().BorderForeground(lightSkyBlue)
	diffPathStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB"))
	diffMetaStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	diffModStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#D97706"))
	diffAddStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E"))
	diffDelStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	diffRenStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4"))
	diffHunkStyle     = lipgloss.NewStyle().Foreground(lightSkyBlue)
	diffHeaderStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
)
