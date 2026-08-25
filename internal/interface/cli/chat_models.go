package cli

import (
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/agentapp"

	tea "charm.land/bubbletea/v2"
)

const slashModelInlinePanelMaxContentLines = 12

// slashModelPanelChromeLines are the panel's own lines: box border, title, the
// current-model line, the "… N more" row, and the key hints.
const slashModelPanelChromeLines = 9

// slashModelState is the /model panel above the input.
type slashModelState struct {
	Current  string
	Selected int
	// Offset is the first entry the panel draws. A list longer than the
	// terminal allows is a window onto the entries, not the first N of them:
	// selection moves through all of them, so the window has to follow it or
	// the rows past the fold are selectable without ever being visible.
	Offset    int
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
	// The current model can be anywhere in the list, so the panel opens with it
	// on screen rather than showing the top and a cursor somewhere below it.
	scrollModelIntoView(m, st)
	m.slashModel = st
	return m.openPanel(st)
}

// modeLabel says where this session's prompts go. It is shown whichever mode is
// in effect, so the answer never depends on a label being absent.
func modeLabel(serverURL string) string {
	if serverURL == "" {
		return "local, straight to each provider"
	}
	host := strings.TrimPrefix(strings.TrimPrefix(serverURL, "https://"), "http://")
	return "all prompts go to " + host
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
		scrollModelIntoView(m, p)
		return true, nil
	case tea.KeyDown:
		p.Selected = (p.Selected + 1) % len(p.Entries)
		scrollModelIntoView(m, p)
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
		if st, err := m.opts.App.EstimateRunUsage(m.opts.Session); err == nil {
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

// modelRowBudget is how many entries the panel may draw, which depends on the
// terminal height and on whether a note is taking two of the lines.
func (m *Model) modelRowBudget(st *slashModelState) int {
	chrome := slashModelPanelChromeLines
	if st.Note != "" {
		chrome += 2
	}
	return m.panelListBudget(slashModelInlinePanelMaxContentLines, chrome)
}

// scrollModelIntoView moves the window the least it can to contain Selected.
func scrollModelIntoView(m *Model, st *slashModelState) {
	rows := m.modelRowBudget(st)
	if st.Selected < st.Offset {
		st.Offset = st.Selected
	} else if st.Selected >= st.Offset+rows {
		st.Offset = st.Selected - rows + 1
	}
	clampModelOffset(st, rows)
}

// clampModelOffset keeps the window inside the list. The budget shrinks when
// the terminal does, under a panel that is already open and already scrolled,
// and a stale offset would then leave rows blank at the bottom.
func clampModelOffset(st *slashModelState, rows int) {
	if last := len(st.Entries) - rows; st.Offset > last {
		st.Offset = last
	}
	if st.Offset < 0 {
		st.Offset = 0
	}
}

func (p *slashModelState) Render(m *Model, maxLineWidth int) string {
	var b strings.Builder
	// The mode rides on the title rather than taking a line of its own: in
	// either mode every model in the list sends prompts to the same place, and
	// the panel has to fit a short terminal.
	title := "Models"
	if m != nil && m.opts.App != nil {
		title += " — " + modeLabel(m.opts.App.ManagedServerURL())
	}
	b.WriteString(slashPanelTitleStyle.Render(truncateRunes(title, maxLineWidth)))
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
		b.WriteString("Add a models: entry to settings.yaml.")
		out := strings.TrimRight(b.String(), "\n")
		return out + "\n\nesc: close"
	}
	rows := m.modelRowBudget(p)
	clampModelOffset(p, rows)
	end := min(p.Offset+rows, len(p.Entries))
	for i := p.Offset; i < end; i++ {
		entry := p.Entries[i]
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
	}
	if remaining := len(p.Entries) - end; remaining > 0 {
		fmt.Fprintf(&b, "… %d more\n", remaining)
	}
	out := strings.TrimRight(b.String(), "\n")
	return out + "\n\n↑↓ select · enter switch · esc: close"
}
