package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"buildmax/internal/config"
	"buildmax/internal/mcpservers"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const mcpProbeTimeout = 45 * time.Second

const mcpInlinePanelMaxContentLines = 14

// mcpOverlayState is the /mcp system panel above the input (not part of chat session).
type mcpOverlayState struct {
	Loading   bool
	LoadError string
	Empty     bool
	Rows      []mcpservers.MCPServerProbeRow
}

type mcpProbeDoneMsg struct {
	LoadError string
	Rows      []mcpservers.MCPServerProbeRow
	Empty     bool
}

var mcpOverlayTitleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lightSkyBlue)

// mcpInlineBoxStyle is the bordered box for MCP and shared slash popup framing.
var mcpInlineBoxStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.RoundedBorder()).
	BorderForeground(lightSkyBlue).
	Padding(0, 1)

func runMCPProbeCmd(workspace string) tea.Cmd {
	ch := make(chan tea.Msg, 1)
	go func() {
		cfg, err := config.LoadMCPConfigForWorkspace(workspace)
		if err != nil {
			ch <- mcpProbeDoneMsg{LoadError: err.Error()}
			return
		}
		if cfg == nil || len(cfg.MCPServers) == 0 {
			ch <- mcpProbeDoneMsg{Empty: true}
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), mcpProbeTimeout)
		defer cancel()
		rows := mcpservers.ProbeMCPServers(ctx, cfg, nil)
		ch <- mcpProbeDoneMsg{Rows: rows}
	}()
	return func() tea.Msg { return <-ch }
}

func handleMCPProbeDone(m *Model, msg mcpProbeDoneMsg) (tea.Model, tea.Cmd) {
	if m.mcpOverlay == nil {
		return m, nil
	}
	m.mcpOverlay.Loading = false
	if msg.LoadError != "" {
		m.mcpOverlay.LoadError = msg.LoadError
		m.mcpOverlay.Empty = false
		m.mcpOverlay.Rows = nil
		return m, nil
	}
	if msg.Empty {
		m.mcpOverlay.LoadError = ""
		m.mcpOverlay.Empty = true
		m.mcpOverlay.Rows = nil
		return m, nil
	}
	m.mcpOverlay.LoadError = ""
	m.mcpOverlay.Empty = false
	m.mcpOverlay.Rows = msg.Rows
	return m, nil
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-3]) + "..."
}

// renderMCPInlinePanel returns the MCP status block above the input, or "" if closed.
func (m *Model) renderMCPInlinePanel() string {
	st := m.mcpOverlay
	if st == nil {
		return ""
	}
	boxW := m.width - 2
	if boxW < 12 {
		boxW = 12
	}
	inner := m.buildMCPOverlayContent(boxW)
	boxed := mcpInlineBoxStyle.Width(boxW).Render(inner)
	return boxed
}

func (m *Model) buildMCPOverlayContent(maxLineWidth int) string {
	st := m.mcpOverlay
	if st == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(mcpOverlayTitleStyle.Render("MCP servers"))
	b.WriteString("\n\n")
	if st.Loading {
		b.WriteString("Probing MCP servers…")
		return b.String()
	}
	if st.LoadError != "" {
		b.WriteString("error: ")
		b.WriteString(truncateRunes(st.LoadError, maxLineWidth-8))
		b.WriteString("\n\nCheck mcp.json (BUILDMAX_MCP_CONFIG, ~/.buildmax/mcp.json, or workspace .buildmax/mcp.json).")
		return b.String()
	}
	if st.Empty {
		b.WriteString("No MCP servers configured.\n\n")
		b.WriteString("Add mcp.json: BUILDMAX_MCP_CONFIG, ~/.buildmax/mcp.json, or <workspace>/.buildmax/mcp.json")
		return b.String()
	}
	b.WriteString(fmt.Sprintf("%-16s  %-6s  %s\n", "id", "type", "status"))
	linesOut := 0
	for _, row := range st.Rows {
		if linesOut >= mcpInlinePanelMaxContentLines {
			remaining := len(st.Rows) - linesOut
			if remaining > 0 {
				b.WriteString(fmt.Sprintf("… %d more\n", remaining))
			}
			break
		}
		status := "connected"
		if row.OK {
			if row.ToolCount != 1 {
				status = fmt.Sprintf("connected (%d tools)", row.ToolCount)
			} else {
				status = "connected (1 tool)"
			}
		} else if row.Err != nil {
			status = "error: " + truncateRunes(row.Err.Error(), max(20, maxLineWidth-30))
		} else {
			status = "error"
		}
		line := fmt.Sprintf("%-16s  %-6s  %s", truncateRunes(row.ID, 14), row.Type, status)
		b.WriteString(line)
		b.WriteByte('\n')
		linesOut++
	}
	out := strings.TrimRight(b.String(), "\n")
	return out + "\n\nesc: close"
}

func closeMCPOverlay(m *Model) (tea.Model, tea.Cmd) {
	m.mcpOverlay = nil
	m.focusInput = true
	return m, tea.Batch(textarea.Blink, m.inputBlock.Focus())
}

// dispatchSlashCommand runs a resolved system command (no session append).
func dispatchSlashCommand(m *Model, cmd string) (tea.Model, tea.Cmd) {
	switch cmd {
	case "/mcp":
		m.err = ""
		m.mcpOverlay = &mcpOverlayState{Loading: true}
		return m, runMCPProbeCmd(m.opts.Workspace)
	default:
		m.err = "unknown command " + cmd + " (try /mcp)"
		return m, nil
	}
}
