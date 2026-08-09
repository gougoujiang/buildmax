package cli

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

// builtinSlashCommands is sorted; add new system commands here for completion.
var builtinSlashCommands = []string{
	"/diff",
	"/mcp",
	"/model",
	"/sessions",
	"/skills",
	"/tools",
}

// slashPanel is the abstraction shared by all "/" overlays (sessions, models,
// tools, skills, mcp, diff). The Model holds at most one active panel; key
// dispatch, footer hints, and rendering all funnel through this interface so
// adding a new panel only requires implementing the methods and registering an
// open factory in dispatchSlashCommand.
type slashPanel interface {
	// Render returns the inner panel content; the caller wraps it in the
	// shared panel box style at maxWidth.
	Render(m *Model, maxWidth int) string
	// HandleKey processes a key while this panel is active. Returning
	// handled=true stops further key dispatch. Returning handled=false lets
	// the model fall through to its default handling.
	HandleKey(m *Model, msg tea.KeyPressMsg) (handled bool, cmd tea.Cmd)
	// FooterHint is appended to the second footer line (e.g. "esc: close tools panel").
	FooterHint() string
	// OnClose lets the panel nil its typed mirror field (m.slashFoo) so existing
	// callers / tests that look at the typed field see the panel is gone.
	OnClose(m *Model)
}

// openPanel installs the panel as the active one, blurs the input, and clears
// any prior footer error.
func (m *Model) openPanel(p slashPanel) (tea.Model, tea.Cmd) {
	m.err = ""
	m.activePanel = p
	m.focusInput = false
	m.inputBlock.Blur()
	return m, nil
}

// closeActivePanel clears the active panel and returns focus to the input.
func (m *Model) closeActivePanel() (tea.Model, tea.Cmd) {
	if m.activePanel != nil {
		m.activePanel.OnClose(m)
	}
	m.activePanel = nil
	m.focusInput = true
	return m, tea.Batch(textarea.Blink, m.inputBlock.Focus())
}

// renderActivePanel returns the boxed panel content, or "" if no panel is open.
func (m *Model) renderActivePanel() string {
	if m.activePanel == nil {
		return ""
	}
	boxW := m.width - 2
	if boxW < 12 {
		boxW = 12
	}
	return slashPanelBoxStyle.Width(boxW).Render(m.activePanel.Render(m, boxW))
}

// slashPopupState is the inline command-completion popup shown above the input
// while the user is typing "/...". It is not a slashPanel — it lives next to
// the input and is dismissed when the user presses enter or backspaces away.
type slashPopupState struct {
	matches  []string
	selected int
}

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
	return slashPanelBoxStyle.Width(boxW).Render(inner)
}

// dispatchSlashCommand runs a resolved system command (no session append).
func dispatchSlashCommand(m *Model, cmd string, args ...string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "/diff":
		return openSlashDiff(m)
	case "/mcp":
		return openSlashMCP(m)
	case "/model":
		return runSlashModel(m, args)
	case "/sessions":
		return openSlashSession(m)
	case "/skills":
		return openSlashSkills(m)
	case "/tools":
		return openSlashTools(m)
	default:
		m.err = "unknown command " + cmd + " (try /diff, /mcp, /model, /sessions, /skills, /tools)"
		return m, nil
	}
}
