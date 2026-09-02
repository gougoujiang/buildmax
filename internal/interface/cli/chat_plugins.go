package cli

import (
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"

	tea "charm.land/bubbletea/v2"
)

const slashPluginsDescriptionMaxRunes = 80
const slashPluginsInlinePanelMaxLines = 14

// slashPluginsPanelChromeLines are the panel's own lines: box border, title,
// the directory line, the "… N more" row, and the key hint.
const slashPluginsPanelChromeLines = 8

// slashPluginsPanel implements slashPanel for the /plugins overlay. It reports
// what this project's runtime resolved, the same snapshot the agent runs with,
// rather than scanning the directory again.
type slashPluginsPanel struct {
	Dir     string
	Plugins []config.DiscoveredPlugin
}

func openSlashPlugins(m *Model) (tea.Model, tea.Cmd) {
	p := &slashPluginsPanel{}
	if m.opts.App != nil {
		snapshot := m.opts.App.Plugins()
		p.Dir = snapshot.Discovery.Dir
		p.Plugins = snapshot.Discovery.Plugins
	}
	return m.openPanel(p)
}

func (p *slashPluginsPanel) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if msg.Code == tea.KeyEscape {
		_, cmd := m.closeActivePanel()
		return true, cmd
	}
	return false, nil
}

func (p *slashPluginsPanel) FooterHint() string { return "esc: close plugins panel" }

func (p *slashPluginsPanel) OnClose(_ *Model) {}

// slashPluginState mirrors the states the Desktop plugin surface reports: a
// policy refusal and a defect are different from being switched off.
func slashPluginState(p config.DiscoveredPlugin) string {
	switch {
	case p.PolicyRefused:
		return "refused"
	case coreplugin.HasErrors(p.Findings):
		return "error"
	case p.State.Disabled:
		return "disabled"
	default:
		return "active"
	}
}

func (p *slashPluginsPanel) Render(m *Model, maxLineWidth int) string {
	var b strings.Builder
	b.WriteString(slashPanelTitleStyle.Render("Plugins"))
	b.WriteString("\n\n")
	if len(p.Plugins) == 0 {
		b.WriteString("No plugins installed.\n\n")
		if p.Dir != "" {
			b.WriteString(truncateRunes("Install into "+p.Dir, maxLineWidth))
		} else {
			b.WriteString("Install into ~/.buildmax/plugins/<name>/.")
		}
		return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
	}
	budget := m.panelListBudget(slashPluginsInlinePanelMaxLines, slashPluginsPanelChromeLines)
	shown := 0
	for _, pl := range p.Plugins {
		if shown+1 > budget {
			if rem := len(p.Plugins) - shown; rem > 0 {
				fmt.Fprintf(&b, "… %d more\n", rem)
			}
			break
		}
		label := pl.Name() + " [" + slashPluginState(pl) + "]"
		desc := truncateRunes(pl.Manifest.Description, slashPluginsDescriptionMaxRunes)
		line := label
		if desc != "" {
			line += " — " + desc
		}
		b.WriteString(truncateRunes(line, maxLineWidth))
		b.WriteByte('\n')
		shown++
	}
	return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
}
