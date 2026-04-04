package tui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// builtinSlashCommands is sorted; add new system commands here for completion.
var builtinSlashCommands = []string{
	"/mcp",
	"/session",
}

type slashPopupState struct {
	matches  []string
	selected int
}

var slashPopupTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lightSkyBlue)
var slashPopupLineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#B8D4E3"))
var slashPopupSelectedStyle = lipgloss.NewStyle().Foreground(lightSkyBlue).Bold(true)

const slashPopupMaxLines = 6

// syncSlashPopupFromInput updates or clears the slash completion popup from the current input (first line only).
func (m *Model) syncSlashPopupFromInput() {
	raw := m.inputBlock.Value()
	first := raw
	if idx := strings.IndexByte(raw, '\n'); idx >= 0 {
		first = raw[:idx]
	}
	first = strings.TrimSpace(first)
	if !strings.HasPrefix(first, "/") {
		m.slashPopup = nil
		return
	}
	if strings.Contains(first, " ") {
		m.slashPopup = nil
		return
	}
	prefix := first
	var matches []string
	for _, c := range builtinSlashCommands {
		if strings.HasPrefix(c, prefix) {
			matches = append(matches, c)
		}
	}
	if len(matches) == 0 {
		m.slashPopup = nil
		return
	}
	sel := 0
	if m.slashPopup != nil && m.slashPopup.selected < len(m.slashPopup.matches) {
		prev := m.slashPopup.matches[m.slashPopup.selected]
		if i := slices.Index(matches, prev); i >= 0 {
			sel = i
		}
	}
	m.slashPopup = &slashPopupState{matches: matches, selected: sel}
}

func (m *Model) renderSlashPopupPanel() string {
	if m.slashPopup == nil || len(m.slashPopup.matches) == 0 {
		return ""
	}
	boxW := m.width - 2
	if boxW < 12 {
		boxW = 12
	}
	var b strings.Builder
	b.WriteString(slashPopupTitleStyle.Render("Commands"))
	b.WriteByte('\n')
	all := m.slashPopup.matches
	start := 0
	show := all
	if len(all) > slashPopupMaxLines {
		start = m.slashPopup.selected - slashPopupMaxLines/2
		if start < 0 {
			start = 0
		}
		if start+slashPopupMaxLines > len(all) {
			start = len(all) - slashPopupMaxLines
		}
		show = all[start : start+slashPopupMaxLines]
	}
	for i, name := range show {
		global := start + i
		prefix := "  "
		st := slashPopupLineStyle
		if global == m.slashPopup.selected {
			prefix = "› "
			st = slashPopupSelectedStyle
		}
		b.WriteString(st.Render(prefix + name))
		b.WriteByte('\n')
	}
	if len(m.slashPopup.matches) > slashPopupMaxLines {
		b.WriteString(slashPopupLineStyle.Render("  …"))
		b.WriteByte('\n')
	}
	b.WriteString(slashPopupLineStyle.Render("↑↓ select · enter run · esc dismiss"))
	inner := strings.TrimRight(b.String(), "\n")
	return mcpInlineBoxStyle.Width(boxW).Render(inner)
}

// dispatchSlashCommand runs a resolved system command (no session append).
func dispatchSlashCommand(m *Model, cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "/mcp":
		m.err = ""
		m.mcpOverlay = &mcpOverlayState{Loading: true}
		return m, runMCPProbeCmd(m.opts.Workspace)
	case "/session":
		return openSessionListOverlay(m)
	default:
		m.err = "unknown command " + cmd + " (try /mcp, /session)"
		return m, nil
	}
}
