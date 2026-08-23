package cli

import (
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
	tools "github.com/gougoujiang/buildmax/internal/tool"

	tea "charm.land/bubbletea/v2"
)

// slashSkillsDescriptionMaxRunes caps how much of a skill description is shown in /skills.
const slashSkillsDescriptionMaxRunes = 128

// slashSkillsPanelChromeLines are the panel's own lines: box border, title, the
// "… N more" row, and the key hint. The list budget counts lines, not entries,
// because each skill takes a name line and a path line.
const slashSkillsPanelChromeLines = 7

// slashSkillsState is the /skills system panel above the input (not part of chat session).
type slashSkillsState struct {
	Entries []tools.SkillEntry
}

func openSlashSkills(m *Model) (tea.Model, tea.Cmd) {
	var entries []tools.SkillEntry
	if m.opts.App != nil {
		entries = m.opts.App.SkillEntries()
	} else {
		// No runtime to borrow a snapshot from, so scan now: the listing must
		// match what a run would load, plugins included.
		plugins := config.DiscoverPlugins().Loadable()
		entries = tools.ResolveSkills(config.SkillSources(m.opts.Workspace, plugins)).Entries
	}
	st := &slashSkillsState{Entries: entries}
	m.slashSkills = st
	return m.openPanel(st)
}

func (p *slashSkillsState) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if msg.Code == tea.KeyEscape {
		_, cmd := m.closeActivePanel()
		return true, cmd
	}
	return false, nil
}

func (p *slashSkillsState) FooterHint() string { return "esc: close skills panel" }

func (p *slashSkillsState) OnClose(m *Model) { m.slashSkills = nil }

func (p *slashSkillsState) Render(m *Model, maxLineWidth int) string {
	var b strings.Builder
	b.WriteString(slashPanelTitleStyle.Render("Skills"))
	b.WriteString("\n\n")
	if len(p.Entries) == 0 {
		b.WriteString("No skills found.\n\n")
		b.WriteString("Add folders with SKILL.md under <workspace>/.buildmax/skills or under your BuildMax data directory (e.g. ~/.buildmax/skills when BUILDMAX_HOME is unset).")
		out := strings.TrimRight(b.String(), "\n")
		return out + "\n\nesc: close"
	}
	budget := m.panelListBudget(slashMCPInlinePanelMaxContentLines, slashSkillsPanelChromeLines)
	linesOut := 0
	shown := 0
	for _, e := range p.Entries {
		if linesOut+2 > budget {
			rem := len(p.Entries) - shown
			if rem > 0 {
				fmt.Fprintf(&b, "… %d more\n", rem)
			}
			break
		}
		desc := truncateRunes(e.Description, slashSkillsDescriptionMaxRunes)
		main := truncateRunes(e.Name+" - "+desc, maxLineWidth)
		b.WriteString(main)
		b.WriteByte('\n')
		linesOut++
		pathVis := max(0, maxLineWidth-2)
		pathLine := slashPopupLineStyle.Render("  " + truncateRunes(e.Path, pathVis))
		b.WriteString(pathLine)
		b.WriteByte('\n')
		linesOut++
		shown++
	}
	out := strings.TrimRight(b.String(), "\n")
	return out + "\n\nesc: close"
}
