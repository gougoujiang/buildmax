package tui

import (
	"fmt"
	"strings"

	"buildmax/internal/config"
	tools "buildmax/internal/execution/agenttool"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// skillOverlayDescriptionMaxRunes caps how much of a skill description is shown in /skills.
const skillOverlayDescriptionMaxRunes = 128

// skillsOverlayState is the /skills system panel above the input (not part of chat session).
type skillsOverlayState struct {
	Entries []tools.SkillEntry
}

func openSkillsOverlay(m *Model) (tea.Model, tea.Cmd) {
	m.err = ""
	paths := config.SkillSearchPaths(m.opts.Workspace)
	m.skillsOverlay = &skillsOverlayState{
		Entries: tools.DiscoverSkillEntries(paths),
	}
	return m, nil
}

// renderSkillsInlinePanel returns the skills list block above the input, or "" if closed.
func (m *Model) renderSkillsInlinePanel() string {
	st := m.skillsOverlay
	if st == nil {
		return ""
	}
	boxW := m.width - 2
	if boxW < 12 {
		boxW = 12
	}
	inner := m.buildSkillsOverlayContent(boxW)
	return mcpInlineBoxStyle.Width(boxW).Render(inner)
}

func (m *Model) buildSkillsOverlayContent(maxLineWidth int) string {
	st := m.skillsOverlay
	if st == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(mcpOverlayTitleStyle.Render("Skills"))
	b.WriteString("\n\n")
	if len(st.Entries) == 0 {
		b.WriteString("No skills found.\n\n")
		b.WriteString("Add folders with SKILL.md under <workspace>/.buildmax/skills or under your BuildMax data directory (e.g. ~/.buildmax/skills when BUILDMAX_HOME is unset).")
		out := strings.TrimRight(b.String(), "\n")
		return out + "\n\nesc: close"
	}
	linesOut := 0
	shown := 0
	for _, e := range st.Entries {
		if linesOut+2 > mcpInlinePanelMaxContentLines {
			rem := len(st.Entries) - shown
			if rem > 0 {
				b.WriteString(fmt.Sprintf("… %d more\n", rem))
			}
			break
		}
		desc := truncateRunes(e.Description, skillOverlayDescriptionMaxRunes)
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

func closeSkillsOverlay(m *Model) (tea.Model, tea.Cmd) {
	m.skillsOverlay = nil
	m.focusInput = true
	return m, tea.Batch(textarea.Blink, m.inputBlock.Focus())
}
