package tui

import (
	"fmt"
	"strings"
	"time"

	"buildmax/internal/session"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

const sessionInlinePanelMaxContentLines = 18

// sessionOverlayState is the /session list panel above the input (not part of chat session).
type sessionOverlayState struct {
	LoadError string
	Empty     bool
	Lines     []string
}

var sessionOverlayTitleStyle = mcpOverlayTitleStyle

// openSessionListOverlay loads sessions.json, sorts newest-first, and shows a scrollable summary panel.
func openSessionListOverlay(m *Model) (tea.Model, tea.Cmd) {
	m.err = ""
	entries, err := session.LoadSessions(m.opts.SessionsDir)
	if err != nil {
		m.sessionOverlay = &sessionOverlayState{LoadError: err.Error()}
		return m, nil
	}
	session.SortByCreatedAtDesc(entries)
	if len(entries) == 0 {
		m.sessionOverlay = &sessionOverlayState{Empty: true}
		return m, nil
	}
	boxW := m.width - 2
	if boxW < 24 {
		boxW = 24
	}
	maxLine := boxW - 4
	if maxLine < 20 {
		maxLine = 20
	}
	lines := make([]string, 0, len(entries))
	for i := range entries {
		lines = append(lines, formatSessionListRow(i+1, maxLine, &entries[i]))
	}
	m.sessionOverlay = &sessionOverlayState{Lines: lines}
	return m, nil
}

func formatSessionListRow(index, maxWidth int, e *session.SessionItem) string {
	t, err := time.Parse(time.RFC3339, e.CreatedAt)
	timeStr := e.CreatedAt
	if err == nil {
		timeStr = t.Local().Format("2006-01-02 15:04")
	}
	title := e.Title
	if title == "" {
		title = "(no title)"
	}
	prefix := fmt.Sprintf("%2d. %s  ", index, timeStr)
	suffix := "  " + e.ID
	prefixW := len([]rune(prefix))
	suffixW := len([]rune(suffix))
	avail := maxWidth - prefixW - suffixW
	if avail < 4 {
		avail = 4
	}
	return prefix + truncateRunes(title, avail) + suffix
}

func (m *Model) renderSessionInlinePanel() string {
	st := m.sessionOverlay
	if st == nil {
		return ""
	}
	boxW := m.width - 2
	if boxW < 12 {
		boxW = 12
	}
	inner := m.buildSessionOverlayContent(boxW)
	return mcpInlineBoxStyle.Width(boxW).Render(inner)
}

func (m *Model) buildSessionOverlayContent(maxLineWidth int) string {
	st := m.sessionOverlay
	if st == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(sessionOverlayTitleStyle.Render("Sessions (newest first)"))
	b.WriteString("\n\n")
	if st.LoadError != "" {
		b.WriteString("error: ")
		b.WriteString(truncateRunes(st.LoadError, maxLineWidth-8))
		b.WriteString("\n\nCould not read sessions list under ")
		b.WriteString(truncateRunes(m.opts.SessionsDir, max(12, maxLineWidth-32)))
		b.WriteString(".")
		return b.String()
	}
	if st.Empty {
		b.WriteString("No saved sessions yet.\n\n")
		b.WriteString("Sessions appear here after you send messages in the TUI (see sessions.json in your sessions directory).")
		return b.String()
	}
	linesOut := 0
	for _, line := range st.Lines {
		if linesOut >= sessionInlinePanelMaxContentLines {
			remaining := len(st.Lines) - linesOut
			if remaining > 0 {
				b.WriteString(fmt.Sprintf("… %d more\n", remaining))
			}
			break
		}
		b.WriteString(truncateRunes(line, maxLineWidth))
		b.WriteByte('\n')
		linesOut++
	}
	out := strings.TrimRight(b.String(), "\n")
	return out + "\n\nesc: close · resume with: buildmax --resume <id>"
}

func closeSessionOverlay(m *Model) (tea.Model, tea.Cmd) {
	m.sessionOverlay = nil
	m.focusInput = true
	return m, tea.Batch(textarea.Blink, m.inputBlock.Focus())
}
