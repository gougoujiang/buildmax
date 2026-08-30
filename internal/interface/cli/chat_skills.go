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

// slashSkillsPanelChromeLines are the panel's own lines: box border, title,
// the search bar and its blank line, the "… N more" row, and the key hint.
// The list budget counts lines, not entries, because each skill takes a name
// line and a path line.
const slashSkillsPanelChromeLines = 9

// slashSkillsState is the /skills system panel above the input (not part of chat session).
type slashSkillsState struct {
	Entries  []tools.SkillEntry // full list, load order
	Filtered []tools.SkillEntry // Entries after Query
	Selected int
	Offset   int
	Query    string
}

// skillEntries resolves the skill listing this run would use. Extracted so
// openSlashSkills and dispatchSlashCommand's skill fallback never disagree on
// what counts as a loaded skill.
func (m *Model) skillEntries() []tools.SkillEntry {
	if m.opts.App != nil {
		return m.opts.App.SkillEntries()
	}
	// No runtime to borrow a snapshot from, so scan now: the listing must
	// match what a run would load, plugins included.
	plugins := config.DiscoverPlugins().Loadable()
	return tools.ResolveSkills(config.SkillSources(workspaceRoot(m.opts.Workspace), plugins)).Entries
}

// findSkill looks up a skill by exact name from the current listing.
func (m *Model) findSkill(name string) (tools.SkillEntry, bool) {
	for _, e := range m.skillEntries() {
		if e.Name == name {
			return e, true
		}
	}
	return tools.SkillEntry{}, false
}

func openSlashSkills(m *Model) (tea.Model, tea.Cmd) {
	st := &slashSkillsState{Entries: m.skillEntries()}
	applySkillFilter(st)
	m.slashSkills = st
	return m.openPanel(st)
}

// applySkillFilter rebuilds Filtered from Entries using the current Query.
func applySkillFilter(st *slashSkillsState) {
	if st.Query == "" {
		st.Filtered = st.Entries
		return
	}
	q := strings.ToLower(st.Query)
	filtered := make([]tools.SkillEntry, 0, len(st.Entries))
	for _, e := range st.Entries {
		if strings.Contains(strings.ToLower(e.Name), q) || strings.Contains(strings.ToLower(e.Description), q) {
			filtered = append(filtered, e)
		}
	}
	st.Filtered = filtered
}

// setSkillFilter updates the search query and rebuilds the filtered list.
func setSkillFilter(m *Model, query string) {
	if m.slashSkills == nil {
		return
	}
	m.slashSkills.Query = query
	applySkillFilter(m.slashSkills)
	m.slashSkills.Selected = 0
	m.slashSkills.Offset = 0
}

// skillEntryBudget is how many skill entries fit on screen. Each entry is two
// rendered lines (name+description, then path), so the line budget halves.
func (m *Model) skillEntryBudget() int {
	lines := m.panelListBudget(slashMCPInlinePanelMaxContentLines, slashSkillsPanelChromeLines)
	return max(1, lines/2)
}

// scrollSkillIntoView adjusts Offset so that Selected is within the visible window.
func scrollSkillIntoView(m *Model, st *slashSkillsState) {
	rows := m.skillEntryBudget()
	if st.Selected < st.Offset {
		st.Offset = st.Selected
	} else if st.Selected >= st.Offset+rows {
		st.Offset = st.Selected - rows + 1
	}
}

func (p *slashSkillsState) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	n := len(p.Filtered)
	switch msg.Code {
	case tea.KeyUp:
		if n > 0 && p.Selected > 0 {
			p.Selected--
			scrollSkillIntoView(m, p)
		}
		return true, nil
	case tea.KeyDown:
		if n > 0 && p.Selected < n-1 {
			p.Selected++
			scrollSkillIntoView(m, p)
		}
		return true, nil
	case tea.KeyEnter:
		return true, confirmSlashSkillUse(m)
	case tea.KeyEscape:
		_, cmd := m.closeActivePanel()
		return true, cmd
	case tea.KeyBackspace:
		q := []rune(p.Query)
		if len(q) > 0 {
			setSkillFilter(m, string(q[:len(q)-1]))
		}
		return true, nil
	}
	s := msg.String()
	if len([]rune(s)) == 1 && []rune(s)[0] >= 32 {
		setSkillFilter(m, p.Query+s)
		return true, nil
	}
	return true, nil
}

// confirmSlashSkillUse fills the input with the selected skill's slash form
// and closes the panel. It does not send: the user adds args and presses
// Enter themselves, matching Desktop's SkillsPopup exactly.
func confirmSlashSkillUse(m *Model) tea.Cmd {
	if m.slashSkills == nil || len(m.slashSkills.Filtered) == 0 {
		_, cmd := m.closeActivePanel()
		return cmd
	}
	entry := m.slashSkills.Filtered[m.slashSkills.Selected]
	restorePrompt(m, "/"+entry.Name)
	_, cmd := m.closeActivePanel()
	return cmd
}

func (p *slashSkillsState) FooterHint() string { return "esc: close skills panel" }

func (p *slashSkillsState) OnClose(m *Model) { m.slashSkills = nil }

func (p *slashSkillsState) keyHints() string {
	return "↑↓ select · enter: use skill · esc: close"
}

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
	queryDisplay := p.Query + "█"
	if p.Query == "" {
		queryDisplay = "█"
	}
	b.WriteString("/ " + queryDisplay)
	b.WriteString("\n\n")
	if len(p.Filtered) == 0 {
		b.WriteString("No skills match.")
		return strings.TrimRight(b.String(), "\n") + "\n\n" + p.keyHints()
	}
	rows := m.skillEntryBudget()
	end := min(p.Offset+rows, len(p.Filtered))
	for i := p.Offset; i < end; i++ {
		e := &p.Filtered[i]
		cursor := "  "
		if i == p.Selected {
			cursor = "› "
		}
		desc := truncateRunes(e.Description, slashSkillsDescriptionMaxRunes)
		main := truncateRunes(cursor+e.Name+" - "+desc, maxLineWidth)
		b.WriteString(main)
		b.WriteByte('\n')
		pathVis := max(0, maxLineWidth-4)
		pathLine := slashPopupLineStyle.Render("    " + truncateRunes(e.Path, pathVis))
		b.WriteString(pathLine)
		b.WriteByte('\n')
	}
	if remaining := len(p.Filtered) - end; remaining > 0 {
		fmt.Fprintf(&b, "… %d more\n", remaining)
	}
	out := strings.TrimRight(b.String(), "\n")
	return out + "\n\n" + p.keyHints()
}
