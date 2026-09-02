package cli

import (
	"fmt"
	"strings"

	tools "github.com/gougoujiang/buildmax/internal/tool"

	tea "charm.land/bubbletea/v2"
)

const slashAgentsDescriptionMaxRunes = 100
const slashAgentsInlinePanelMaxLines = 14

// slashAgentsPanelChromeLines are the panel's own lines: box border, title, the
// "… N more" row, and the key hint.
const slashAgentsPanelChromeLines = 7

// slashAgentEntry is one agent type the Task tool can delegate to.
type slashAgentEntry struct {
	Name        string
	Description string
	Builtin     bool
}

// slashAgentsPanel implements slashPanel for the /agents overlay: the agent
// types the Task tool can delegate to, builtin ones first and then the
// workspace and home definitions.
type slashAgentsPanel struct {
	Entries []slashAgentEntry
}

func openSlashAgents(m *Model) (tea.Model, tea.Cmd) {
	var entries []slashAgentEntry
	for _, def := range tools.BuiltinSubAgentDefs() {
		entries = append(entries, slashAgentEntry{Name: def.Name, Description: def.Description, Builtin: true})
	}
	if m.opts.App != nil {
		for _, def := range m.opts.App.AgentDefs() {
			entries = append(entries, slashAgentEntry{Name: def.Name, Description: def.Description})
		}
	}
	return m.openPanel(&slashAgentsPanel{Entries: entries})
}

func (p *slashAgentsPanel) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if msg.Code == tea.KeyEscape {
		_, cmd := m.closeActivePanel()
		return true, cmd
	}
	return false, nil
}

func (p *slashAgentsPanel) FooterHint() string { return "esc: close agents panel" }

func (p *slashAgentsPanel) OnClose(_ *Model) {}

func (p *slashAgentsPanel) Render(m *Model, maxLineWidth int) string {
	var b strings.Builder
	b.WriteString(slashPanelTitleStyle.Render("Agents"))
	b.WriteString("\n\n")
	if len(p.Entries) == 0 {
		b.WriteString("No agents available.")
		return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
	}
	budget := m.panelListBudget(slashAgentsInlinePanelMaxLines, slashAgentsPanelChromeLines)
	shown := 0
	for _, e := range p.Entries {
		if shown+1 > budget {
			if rem := len(p.Entries) - shown; rem > 0 {
				fmt.Fprintf(&b, "… %d more\n", rem)
			}
			break
		}
		label := e.Name
		if e.Builtin {
			label += " [builtin]"
		}
		desc := truncateRunes(e.Description, slashAgentsDescriptionMaxRunes)
		b.WriteString(truncateRunes(label+" — "+desc, maxLineWidth))
		b.WriteByte('\n')
		shown++
	}
	return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
}
