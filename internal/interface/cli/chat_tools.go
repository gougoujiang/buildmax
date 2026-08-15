package cli

import (
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/agentapp"

	tea "charm.land/bubbletea/v2"
)

const slashToolsDescriptionMaxRunes = 100
const slashToolsInlinePanelMaxLines = 14

// slashToolsPanel implements slashPanel for the /tools overlay.
type slashToolsPanel struct {
	Entries []agentapp.ToolEntry
}

func openSlashTools(m *Model) (tea.Model, tea.Cmd) {
	var entries []agentapp.ToolEntry
	if m.opts.App != nil {
		entries = m.opts.App.ToolEntries()
	}
	return m.openPanel(&slashToolsPanel{Entries: entries})
}

func (p *slashToolsPanel) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if msg.Code == tea.KeyEscape {
		_, cmd := m.closeActivePanel()
		return true, cmd
	}
	return false, nil
}

func (p *slashToolsPanel) FooterHint() string { return "esc: close tools panel" }

func (p *slashToolsPanel) OnClose(_ *Model) {}

func (p *slashToolsPanel) Render(_ *Model, maxLineWidth int) string {
	var b strings.Builder
	b.WriteString(slashPanelTitleStyle.Render("Tools"))
	b.WriteString("\n\n")
	if len(p.Entries) == 0 {
		b.WriteString("No tools available.")
		return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
	}
	linesOut := 0
	shown := 0
	for _, e := range p.Entries {
		if linesOut+2 > slashToolsInlinePanelMaxLines {
			rem := len(p.Entries) - shown
			if rem > 0 {
				fmt.Fprintf(&b, "… %d more\n", rem)
			}
			break
		}
		desc := truncateRunes(e.Description, slashToolsDescriptionMaxRunes)
		main := truncateRunes(e.Name+" — "+desc, maxLineWidth)
		b.WriteString(main)
		b.WriteByte('\n')
		linesOut++
		shown++
	}
	return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
}
