package cli

import (
	"fmt"
	"strings"

	mcp "github.com/gougoujiang/buildmax/internal/infra/mcp"

	tea "charm.land/bubbletea/v2"
)

const slashMCPInlinePanelMaxContentLines = 14

// slashMCPState is the /mcp system panel above the input (not part of chat session).
type slashMCPState struct {
	LoadError string
	Servers   []mcp.MCPServerStatus
}

func openSlashMCP(m *Model) (tea.Model, tea.Cmd) {
	st := &slashMCPState{}
	if m.opts.App != nil {
		status := m.opts.App.MCPStatus()
		st.LoadError = status.LoadError
		st.Servers = status.Servers
	} else {
		st.LoadError = "agent app is not initialized"
	}
	m.slashMCP = st
	return m.openPanel(st)
}

func (p *slashMCPState) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if msg.Code == tea.KeyEscape {
		_, cmd := m.closeActivePanel()
		return true, cmd
	}
	return false, nil
}

func (p *slashMCPState) FooterHint() string { return "esc: close MCP panel" }

func (p *slashMCPState) OnClose(m *Model) { m.slashMCP = nil }

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

func (p *slashMCPState) Render(_ *Model, maxLineWidth int) string {
	var b strings.Builder
	b.WriteString(slashPanelTitleStyle.Render("MCP servers"))
	b.WriteString("\n\n")
	if p.LoadError != "" {
		b.WriteString("error: ")
		b.WriteString(truncateRunes(p.LoadError, maxLineWidth-8))
		b.WriteString("\n\nCheck mcp.json in the workspace (.buildmax/mcp.json) or BuildMax home (~/.buildmax/mcp.json).")
		return b.String()
	}
	if len(p.Servers) == 0 {
		b.WriteString("No MCP servers configured.\n\n")
		b.WriteString("Add mcp.json in BuildMax home (~/.buildmax/mcp.json) and/or <workspace>/.buildmax/mcp.json. Workspace entries override global ones with the same server id.")
		return b.String()
	}
	fmt.Fprintf(&b, "%-16s  %-6s  %s\n", "id", "type", "status")
	linesOut := 0
	for _, row := range p.Servers {
		if linesOut >= slashMCPInlinePanelMaxContentLines {
			remaining := len(p.Servers) - linesOut
			if remaining > 0 {
				fmt.Fprintf(&b, "… %d more\n", remaining)
			}
			break
		}
		var status string
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
