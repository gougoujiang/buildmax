package cli

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

// The history-point panel backs both /rewind and /fork. Both ask the same
// question — which message in this conversation — and differ only in what they
// do with the answer, so the list, the navigation, and the consequence line are
// shared and the two modes supply the rest.
const (
	slashHistoryPanelChromeLines           = 9
	slashHistoryInlinePanelMaxContentLines = 14
)

type historyPointMode int

const (
	// modeRewind moves this conversation back to the chosen message.
	modeRewind historyPointMode = iota
	// modeFork copies the history up to the chosen message into a new session
	// and leaves this one untouched.
	modeFork
)

type slashHistoryPointState struct {
	Mode      historyPointMode
	LoadError string
	Empty     bool
	// Points are the messages this session offers, most recent first — working
	// near the end of a conversation is far more common than near its start.
	Points   []agentapp.RewindPoint
	Selected int
	Offset   int

	// affected is the work that happened after the highlighted point,
	// recomputed as the selection moves. Both modes need it and both mean
	// something different by it; see consequenceLine.
	affected    session.AbandonedWork
	affectedErr string
}

var slashHistoryTitleStyle = slashPanelTitleStyle

func openSlashRewind(m *Model) (tea.Model, tea.Cmd) { return openHistoryPanel(m, modeRewind) }

func openSlashFork(m *Model) (tea.Model, tea.Cmd) { return openHistoryPanel(m, modeFork) }

func openHistoryPanel(m *Model, mode historyPointMode) (tea.Model, tea.Cmd) {
	st := &slashHistoryPointState{Mode: mode}
	if m.opts.Session == nil {
		st.LoadError = "no session is open"
		m.slashHistory = st
		return m.openPanel(st)
	}
	points := agentapp.RewindPoints(m.opts.Session)
	if mode == modeRewind && len(points) > 0 {
		// Rewinding to the current head is not a move, so it is not offered.
		// Forking from it is: "branch off from where we are" is the common
		// case, not a no-op.
		points = points[:len(points)-1]
	}
	reversePoints(points)
	if len(points) == 0 {
		st.Empty = true
	} else {
		st.Points = points
		st.refreshAffected(m)
	}
	m.slashHistory = st
	return m.openPanel(st)
}

func reversePoints(points []agentapp.RewindPoint) {
	for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
		points[i], points[j] = points[j], points[i]
	}
}

// refreshAffected recomputes what happened after the highlighted point.
func (p *slashHistoryPointState) refreshAffected(m *Model) {
	p.affected, p.affectedErr = session.AbandonedWork{}, ""
	if len(p.Points) == 0 || m.opts.Session == nil {
		return
	}
	got, err := m.opts.Session.AbandonedBy(p.Points[p.Selected].ItemID)
	if err != nil {
		// The newest point has nothing after it, which is a legitimate fork
		// target rather than a failure worth reporting.
		if p.Mode == modeFork && p.Selected == 0 {
			return
		}
		p.affectedErr = err.Error()
		return
	}
	p.affected = got
}

func (p *slashHistoryPointState) FooterHint() string {
	if p.Mode == modeFork {
		return "esc: close fork panel"
	}
	return "esc: close rewind panel"
}

func (p *slashHistoryPointState) OnClose(m *Model) { m.slashHistory = nil }

func (p *slashHistoryPointState) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	switch msg.Code {
	case tea.KeyEscape:
		_, cmd := m.closeActivePanel()
		return true, cmd
	case tea.KeyUp:
		if p.Selected > 0 {
			p.Selected--
			p.refreshAffected(m)
			scrollHistoryIntoView(m, p)
		}
		return true, nil
	case tea.KeyDown:
		if p.Selected < len(p.Points)-1 {
			p.Selected++
			p.refreshAffected(m)
			scrollHistoryIntoView(m, p)
		}
		return true, nil
	case tea.KeyEnter:
		if p.Mode == modeFork {
			return true, confirmSlashFork(m)
		}
		return true, confirmSlashRewind(m)
	}
	return false, nil
}

// confirmSlashRewind moves the conversation and reports what it left behind.
func confirmSlashRewind(m *Model) tea.Cmd {
	p := m.slashHistory
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
	m.refreshRunStatus()
	m.focusInput = true

	banner := messageBarStyle.Render("─── Rewound to: " + historyPointLabel(target, 60) + " ───")
	return tea.Sequence(
		tea.Println(banner+"\n\n"+renderAbandoned(abandoned)),
		closeCmd,
	)
}

// confirmSlashFork branches a new session off the chosen point and switches to
// it, leaving the current one on disk exactly as it was.
func confirmSlashFork(m *Model) tea.Cmd {
	p := m.slashHistory
	if p == nil || len(p.Points) == 0 || m.opts.Session == nil {
		_, cmd := m.closeActivePanel()
		return cmd
	}
	target := p.Points[p.Selected]
	parent := m.opts.Session
	child, err := agentapp.NewSessionManager(m.opts.SessionsDir).
		Fork(parent, target.ItemID, m.opts.ModelName)
	if err != nil {
		p.LoadError = "fork failed: " + err.Error()
		return nil
	}
	// The parent goes before the child takes over: one writer per session, and
	// holding both would keep the parent locked for the rest of the process.
	// Releasing it does not depend on there being an app — that only decides
	// whether the SessionEnd hook also runs.
	if m.opts.App != nil {
		m.opts.App.CloseSession(parent)
	} else if cerr := parent.Close(); cerr != nil {
		slog.Warn("closing the forked-from session failed", "err", cerr)
	}
	m.opts.Session = child

	_, closeCmd := m.closeActivePanel()
	m.refreshRunStatus()
	m.focusInput = true

	banner := messageBarStyle.Render("─── Forked at: " + historyPointLabel(target, 60) + " ───")
	return tea.Sequence(
		tea.Println(banner+"\n\n"+renderForked(p.affected)),
		closeCmd,
		drainQueueCmd(),
	)
}

func (m *Model) refreshRunStatus() {
	if m.opts.App == nil || m.opts.Session == nil {
		return
	}
	if st, err := m.opts.App.EstimateRunStatus(m.opts.Session); err == nil {
		m.runStatus = st
	} else {
		slog.Debug("estimating run status after a history move failed", "err", err)
	}
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
	writeToolLines(&b, a)
	b.WriteString("\nRewinding moves the conversation. It does not undo files, commands, or network calls.")
	return b.String()
}

// renderForked is what the user is told after a fork.
//
// The parent loses nothing, so there is no warning to give about it. What the
// user does need is the other half of the same fact: work that happened after
// the fork point really did touch the workspace, and the new session's history
// does not contain it — so the agent there will not know it happened.
func renderForked(a session.AbandonedWork) string {
	if a.Undoable() {
		return "The original session is unchanged, and nothing outside the conversation ran after this point."
	}
	var b strings.Builder
	b.WriteString("The original session is unchanged. These ran after the fork point, so their\n")
	b.WriteString("effects are on disk but the new session's history does not mention them:\n")
	writeToolLines(&b, a)
	return b.String()
}

func writeToolLines(b *strings.Builder, a session.AbandonedWork) {
	for _, e := range a.Effects {
		name := e.ToolName
		if name == "" {
			name = "(unknown tool)"
		}
		if e.Returned {
			fmt.Fprintf(b, "  • %s\n", name)
			continue
		}
		// The worse case: it entered the tool and never reported back, so what
		// it changed was never recorded either.
		fmt.Fprintf(b, "  • %s — interrupted, outcome unknown\n", name)
	}
}

func (p *slashHistoryPointState) Render(m *Model, maxLineWidth int) string {
	var b strings.Builder
	title := "Rewind"
	if p.Mode == modeFork {
		title = "Fork"
	}
	b.WriteString(slashHistoryTitleStyle.Render(title))
	b.WriteString("\n\n")
	if p.LoadError != "" {
		b.WriteString("error: ")
		b.WriteString(truncateRunes(p.LoadError, maxLineWidth-8))
		return b.String() + "\n\nesc: close"
	}
	if p.Empty || len(p.Points) == 0 {
		if p.Mode == modeFork {
			b.WriteString("Nothing to fork from yet.")
		} else {
			b.WriteString("Nothing to rewind to yet.")
		}
		return b.String() + "\n\nesc: close"
	}

	end := p.Offset + m.historyRowBudget(p)
	if end > len(p.Points) {
		end = len(p.Points)
	}
	for i := p.Offset; i < end; i++ {
		cursor := "  "
		if i == p.Selected {
			cursor = "› "
		}
		b.WriteString(truncateRunes(cursor+historyPointLabel(p.Points[i], maxLineWidth-2), maxLineWidth))
		b.WriteByte('\n')
	}
	if remaining := len(p.Points) - end; remaining > 0 {
		fmt.Fprintf(&b, "  … %d more\n", remaining)
	}

	b.WriteString("\n")
	b.WriteString(truncateRunes(consequenceLine(p), maxLineWidth))
	if p.Mode == modeFork {
		b.WriteString("\n\n↑/↓: choose · enter: fork · esc: close")
	} else {
		b.WriteString("\n\n↑/↓: choose · enter: rewind · esc: close")
	}
	return b.String()
}

// consequenceLine is the one-line answer to "what does choosing this do".
//
// The two modes read the same computation differently. A rewind drops those
// messages from this conversation and leaves the tools' effects behind; a fork
// drops nothing — the parent keeps everything — but the new session starts
// without the knowledge of work that nonetheless happened.
func consequenceLine(p *slashHistoryPointState) string {
	if p.affectedErr != "" {
		return "could not check what this would affect: " + p.affectedErr
	}
	msgs := "message"
	if p.affected.Messages != 1 {
		msgs = "messages"
	}
	if p.Mode == modeFork {
		if p.affected.Undoable() {
			return "copies this conversation up to here into a new session"
		}
		return fmt.Sprintf("new session starts here · will not know about: %s", toolNames(p.affected))
	}
	if p.affected.Undoable() {
		return fmt.Sprintf("drops %d %s · nothing outside the conversation ran", p.affected.Messages, msgs)
	}
	return fmt.Sprintf("drops %d %s · leaves in place: %s", p.affected.Messages, msgs, toolNames(p.affected))
}

func toolNames(a session.AbandonedWork) string {
	names := make([]string, 0, len(a.Effects))
	for _, e := range a.Effects {
		name := e.ToolName
		if name == "" {
			name = "?"
		}
		if !e.Returned {
			name += "(interrupted)"
		}
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

// historyPointLabel is one row: who spoke, and enough of what they said to
// recognise the moment.
func historyPointLabel(p agentapp.RewindPoint, width int) string {
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

// historyRowBudget is how many rows fit once the panel chrome is accounted for.
func (m *Model) historyRowBudget(p *slashHistoryPointState) int {
	budget := m.panelListBudget(slashHistoryInlinePanelMaxContentLines, slashHistoryPanelChromeLines)
	if budget < 1 {
		budget = 1
	}
	if budget > len(p.Points) {
		budget = len(p.Points)
	}
	return budget
}

func scrollHistoryIntoView(m *Model, p *slashHistoryPointState) {
	budget := m.historyRowBudget(p)
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
