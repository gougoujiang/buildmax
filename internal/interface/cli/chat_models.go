package cli

import (
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/agentapp"

	tea "charm.land/bubbletea/v2"
)

const slashModelInlinePanelMaxContentLines = 12

// slashModelState is the /model panel above the input.
type slashModelState struct {
	Current   string
	Selected  int
	Note      string
	LoadError string
	Entries   []agentapp.ModelConfig
}

func runSlashModel(m *Model, args []string) (tea.Model, tea.Cmd) {
	selector := strings.TrimSpace(strings.Join(args, " "))
	return openSlashModel(m, "", selector)
}

func openSlashModel(m *Model, note string, selector string) (tea.Model, tea.Cmd) {
	current, entries, loadErr := loadSlashModelEntries(m)
	selected := selectedModelIndex(entries, current, selector)
	st := &slashModelState{
		Current:   current,
		Selected:  selected,
		Note:      note,
		LoadError: loadErr,
		Entries:   entries,
	}
	m.slashModel = st
	return m.openPanel(st)
}

func selectedModelIndex(entries []agentapp.ModelConfig, current string, selector string) int {
	for i, entry := range entries {
		if selector != "" && (entry.Name == selector || entry.ProviderModel == selector) {
			return i
		}
	}
	for i, entry := range entries {
		if entry.Name == current || entry.ProviderModel == current {
			return i
		}
	}
	return 0
}

func (p *slashModelState) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if p.LoadError != "" || len(p.Entries) == 0 {
		if msg.Code == tea.KeyEscape {
			_, cmd := m.closeActivePanel()
			return true, cmd
		}
		return false, nil
	}
	switch msg.Code {
	case tea.KeyUp:
		if p.Selected > 0 {
			p.Selected--
		} else {
			p.Selected = len(p.Entries) - 1
		}
		return true, nil
	case tea.KeyDown:
		p.Selected = (p.Selected + 1) % len(p.Entries)
		return true, nil
	case tea.KeyEnter:
		return true, p.confirm(m)
	case tea.KeyEscape:
		_, cmd := m.closeActivePanel()
		return true, cmd
	}
	return false, nil
}

func (p *slashModelState) confirm(m *Model) tea.Cmd {
	if p.Selected < 0 || p.Selected >= len(p.Entries) {
		p.Selected = 0
	}
	cfg := p.Entries[p.Selected]
	if m.opts.Session != nil {
		m.opts.Session.SetModel(cfg.Name)
	}
	m.opts.ModelName = cfg.Name
	if m.opts.App != nil && m.opts.Session != nil {
		if st, err := m.opts.App.EstimateRunStatus(m.opts.Session); err == nil {
			m.runStatus = updateRunStatusContext(m.runStatus, st)
		}
	}
	_, cmd := m.closeActivePanel()
	return cmd
}

func (p *slashModelState) FooterHint() string { return "esc: close model panel" }

func (p *slashModelState) OnClose(m *Model) { m.slashModel = nil }

func loadSlashModelEntries(m *Model) (current string, entries []agentapp.ModelConfig, loadErr string) {
	if m != nil && m.opts.Session != nil {
		current = m.opts.Session.ModelName(m.opts.ModelName)
	} else if m != nil {
		current = m.opts.ModelName
	}
	if m == nil || m.opts.App == nil {
		return current, nil, "agent app is not initialized"
	}
	return current, m.opts.App.ModelConfigs(), ""
}

// buildSlashModelContent is exported (lowercase but referenced by tests) for
// inline rendering checks.
func (m *Model) buildSlashModelContent(maxLineWidth int) string {
	if m.slashModel == nil {
		return ""
	}
	return m.slashModel.Render(m, maxLineWidth)
}

func (p *slashModelState) Render(_ *Model, maxLineWidth int) string {
	var b strings.Builder
	b.WriteString(slashPanelTitleStyle.Render("Models"))
	b.WriteString("\n\n")
	if p.Current != "" {
		b.WriteString("Current: ")
		b.WriteString(truncateRunes(p.Current, maxLineWidth-9))
		b.WriteString("\n\n")
	}
	if p.Note != "" {
		b.WriteString(truncateRunes(p.Note, maxLineWidth))
		b.WriteString("\n\n")
	}
	if p.LoadError != "" {
		b.WriteString("error: ")
		b.WriteString(truncateRunes(p.LoadError, maxLineWidth-8))
		out := strings.TrimRight(b.String(), "\n")
		return out + "\n\nesc: close"
	}
	if len(p.Entries) == 0 {
		b.WriteString("No models configured.\n\n")
		b.WriteString("Add models in settings.yaml or set BUILDMAX_MODEL / BUILDMAX_API_KEY for the fallback model.")
		out := strings.TrimRight(b.String(), "\n")
		return out + "\n\nesc: close"
	}
	linesOut := 0
	for i, entry := range p.Entries {
		if linesOut >= slashModelInlinePanelMaxContentLines {
			remaining := len(p.Entries) - i
			if remaining > 0 {
				b.WriteString(fmt.Sprintf("… %d more\n", remaining))
			}
			break
		}
		prefix := "  "
		if i == p.Selected {
			prefix = "› "
		}
		currentMark := "  "
		if entry.Name == p.Current || entry.ProviderModel == p.Current {
			currentMark = "* "
		}
		line := prefix + currentMark + entry.Name
		if entry.ProviderModel != "" && entry.ProviderModel != entry.Name {
			line += " -> " + entry.ProviderModel
		}
		b.WriteString(truncateRunes(line, maxLineWidth))
		b.WriteByte('\n')
		linesOut++
	}
	out := strings.TrimRight(b.String(), "\n")
	return out + "\n\n↑↓ select · enter switch · esc: close"
}
