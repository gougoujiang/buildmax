package cli

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

// slashRewindPanelChromeLines is what the rewind panel spends on everything
// that is not a row: the title, the blank line under it, the consequence block,
// and the key hints.
const (
	slashRewindPanelChromeLines           = 9
	slashRewindInlinePanelMaxContentLines = 14
)

type slashRewindState struct {
	LoadError string
	Empty     bool
	// Points are the messages this session can return to, most recent first —
	// rewinding a little is far more common than rewinding to the beginning.
	Points   []agentapp.RewindPoint
	Selected int
	Offset   int

	// preview is what rewinding to the highlighted point would leave in place,
	// recomputed as the selection moves. It is shown before the user commits,
	// because a choice made without knowing what it leaves behind is exactly
	// what §8.1 of the session storage design says rewind must not hide.
	preview    session.AbandonedWork
	previewErr string
}

var slashRewindTitleStyle = slashPanelTitleStyle

func openSlashRewind(m *Model) (tea.Model, tea.Cmd) {
	st := &slashRewindState{}
	if m.opts.Session == nil {
		st.LoadError = "no session is open"
		m.slashRewind = st
		return m.openPanel(st)
	}
	points := agentapp.RewindPoints(m.opts.Session)
	// The newest point is the current head, and rewinding to where you already
	// are is not a move. Offering it would be offering a no-op.
	if len(points) > 0 {
		points = points[:len(points)-1]
	}
	reverse(points)
	if len(points) == 0 {
		st.Empty = true
	} else {
		st.Points = points
		st.refreshPreview(m)
	}
	m.slashRewind = st
	return m.openPanel(st)
}

func reverse(points []agentapp.RewindPoint) {
	for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
		points[i], points[j] = points[j], points[i]
	}
}

// refreshPreview recomputes the consequence for the highlighted point.
func (p *slashRewindState) refreshPreview(m *Model) {
	p.preview, p.previewErr = session.AbandonedWork{}, ""
	if len(p.Points) == 0 || m.opts.Session == nil {
		return
	}
	got, err := m.opts.Session.AbandonedBy(p.Points[p.Selected].ItemID)
	if err != nil {
		p.previewErr = err.Error()
		return
	}
	p.preview = got
}

func (p *slashRewindState) FooterHint() string { return "esc: close rewind panel" }

func (p *slashRewindState) OnClose(m *Model) { m.slashRewind = nil }

func (p *slashRewindState) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	switch msg.Code {
	case tea.KeyEscape:
		_, cmd := m.closeActivePanel()
		return true, cmd
	case tea.KeyUp:
		if p.Selected > 0 {
			p.Selected--
			p.refreshPreview(m)
			scrollRewindIntoView(m, p)
		}
		return true, nil
	case tea.KeyDown:
		if p.Selected < len(p.Points)-1 {
			p.Selected++
			p.refreshPreview(m)
			scrollRewindIntoView(m, p)
		}
		return true, nil
	case tea.KeyEnter:
		return true, confirmSlashRewind(m)
	}
	return false, nil
}

// confirmSlashRewind performs the rewind and reports what it left behind.
func confirmSlashRewind(m *Model) tea.Cmd {
	p := m.slashRewind
	if p == nil || len(p.Points) == 0 || m.opts.Session == nil {
		_, cmd := m.closeActivePanel()
		return cmd
	}
	target := p.Points[p.Selected]
	abandoned, err := m.opts.Session.Rewind(target.ItemID)
	if err != nil {
		p.LoadError = "rewind failed: " + err.Error()
		return nil
	}
	_, closeCmd := m.closeActivePanel()
	if m.opts.App != nil {
		if st, err := m.opts.App.EstimateRunStatus(m.opts.Session); err == nil {
			m.runStatus = st
		}
	}
	m.focusInput = true

	banner := messageBarStyle.Render("─── Rewound to: " + rewindLabel(target, 60) + " ───")
	return tea.Sequence(
		tea.Println(banner+"\n\n"+renderAbandoned(abandoned)),
		closeCmd,
	)
}

// renderAbandoned is what the user is told after a rewind.
//
// It names the tools whose effects are still in place, because the conversation
// no longer mentions them and the workspace still contains them. Saying nothing
// would leave the user believing a rewind undid the run.
func renderAbandoned(a session.AbandonedWork) string {
	if a.Undoable() {
		return "Nothing outside the conversation ran in the part that was rewound, so there is nothing left over."
	}
	var b strings.Builder
	b.WriteString("These ran before the rewind and their effects are still in place:\n")
	for _, e := range a.Effects {
		name := e.ToolName
		if name == "" {
			name = "(unknown tool)"
		}
		if e.Returned {
			fmt.Fprintf(&b, "  • %s\n", name)
			continue
		}
		// The worse case: it entered the tool and never reported back, so what
		// it changed was never recorded either.
		fmt.Fprintf(&b, "  • %s — interrupted, outcome unknown\n", name)
	}
	b.WriteString("\nRewinding moves the conversation. It does not undo files, commands, or network calls.")
	return b.String()
}

func (p *slashRewindState) Render(m *Model, maxLineWidth int) string {
	var b strings.Builder
	b.WriteString(slashRewindTitleStyle.Render("Rewind"))
	b.WriteString("\n\n")
	if p.LoadError != "" {
		b.WriteString("error: ")
		b.WriteString(truncateRunes(p.LoadError, maxLineWidth-8))
		return b.String() + "\n\nesc: close"
	}
	if p.Empty || len(p.Points) == 0 {
		b.WriteString("Nothing to rewind to yet.")
		return b.String() + "\n\nesc: close"
	}

	end := p.Offset + m.rewindRowBudget(p)
	if end > len(p.Points) {
		end = len(p.Points)
	}
	for i := p.Offset; i < end; i++ {
		cursor := "  "
		if i == p.Selected {
			cursor = "› "
		}
		b.WriteString(truncateRunes(cursor+rewindLabel(p.Points[i], maxLineWidth-2), maxLineWidth))
		b.WriteByte('\n')
	}
	if remaining := len(p.Points) - end; remaining > 0 {
		fmt.Fprintf(&b, "  … %d more\n", remaining)
	}

	b.WriteString("\n")
	b.WriteString(truncateRunes(previewLine(p), maxLineWidth))
	b.WriteString("\n\n↑/↓: choose · enter: rewind · esc: close")
	return b.String()
}

// previewLine is the one-line consequence shown under the list.
func previewLine(p *slashRewindState) string {
	if p.previewErr != "" {
		return "could not check what this would leave: " + p.previewErr
	}
	msgs := "message"
	if p.preview.Messages != 1 {
		msgs = "messages"
	}
	if p.preview.Undoable() {
		return fmt.Sprintf("drops %d %s · nothing outside the conversation ran", p.preview.Messages, msgs)
	}
	names := make([]string, 0, len(p.preview.Effects))
	for _, e := range p.preview.Effects {
		name := e.ToolName
		if name == "" {
			name = "?"
		}
		if !e.Returned {
			name += "(interrupted)"
		}
		names = append(names, name)
	}
	return fmt.Sprintf("drops %d %s · leaves in place: %s", p.preview.Messages, msgs, strings.Join(names, ", "))
}

// rewindLabel is one row: who spoke, and enough of what they said to recognise
// the moment.
func rewindLabel(p agentapp.RewindPoint, width int) string {
	who := "you"
	switch {
	case p.Role == "assistant":
		who = "agent"
	case p.Source != "":
		// A background event travels as a user message but the user did not
		// say it, and labelling it "you" would misattribute it.
		who = "event"
	}
	text := strings.TrimSpace(strings.ReplaceAll(p.Content, "\n", " "))
	if text == "" {
		text = "(no text)"
	}
	label := who + ": " + text
	if width > 0 {
		label = truncateRunes(label, width)
	}
	return label
}

// rewindRowBudget is how many rows fit once the panel chrome is accounted for.
func (m *Model) rewindRowBudget(p *slashRewindState) int {
	budget := m.panelListBudget(slashRewindInlinePanelMaxContentLines, slashRewindPanelChromeLines)
	if budget < 1 {
		budget = 1
	}
	if budget > len(p.Points) {
		budget = len(p.Points)
	}
	return budget
}

func scrollRewindIntoView(m *Model, p *slashRewindState) {
	budget := m.rewindRowBudget(p)
	if p.Selected < p.Offset {
		p.Offset = p.Selected
	}
	if p.Selected >= p.Offset+budget {
		p.Offset = p.Selected - budget + 1
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
}
