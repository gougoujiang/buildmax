package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/core/session"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

const slashSessionInlinePanelMaxContentLines = 14

type slashSessionState struct {
	LoadError string
	Empty     bool
	All       []session.SessionItem // full sorted list, unchanged by filtering
	Filtered  []session.SessionItem // view after applying Query
	Selected  int
	Offset    int
	Query     string
	Renaming  bool
	RenameVal string
}

var slashSessionTitleStyle = slashPanelTitleStyle

func openSlashSession(m *Model) (tea.Model, tea.Cmd) {
	entries, err := agentapp.LoadSessionList(m.opts.SessionsDir)
	var st *slashSessionState
	switch {
	case err != nil:
		st = &slashSessionState{LoadError: err.Error()}
	case len(entries) == 0:
		st = &slashSessionState{Empty: true}
	default:
		sortSessionsByCreatedAtDesc(entries)
		st = &slashSessionState{All: entries, Filtered: entries}
		// Pre-select the current session if it is in the list.
		if m.opts.Session != nil {
			for i, e := range entries {
				if e.ID == m.opts.Session.ID {
					st.Selected = i
					scrollSessionIntoView(st)
					break
				}
			}
		}
	}
	m.slashSession = st
	return m.openPanel(st)
}

func (p *slashSessionState) FooterHint() string { return "esc: close sessions panel" }

func (p *slashSessionState) OnClose(m *Model) { m.slashSession = nil }

func (p *slashSessionState) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if p.Renaming {
		switch msg.Code {
		case tea.KeyEnter:
			confirmSlashSessionRename(m)
			return true, nil
		case tea.KeyEscape:
			cancelSlashSessionRename(m)
			return true, nil
		case tea.KeyBackspace:
			q := []rune(p.RenameVal)
			if len(q) > 0 {
				p.RenameVal = string(q[:len(q)-1])
			}
			return true, nil
		}
		s := msg.String()
		if len([]rune(s)) == 1 && []rune(s)[0] >= 32 {
			p.RenameVal += s
		}
		return true, nil
	}

	n := len(p.Filtered)
	switch msg.Code {
	case tea.KeyUp:
		if n > 0 && p.Selected > 0 {
			p.Selected--
			scrollSessionIntoView(p)
		}
		return true, nil
	case tea.KeyDown:
		if n > 0 && p.Selected < n-1 {
			p.Selected++
			scrollSessionIntoView(p)
		}
		return true, nil
	case tea.KeyEnter:
		_, cmd := confirmSlashSessionResume(m)
		return true, cmd
	case tea.KeyEscape:
		_, cmd := m.closeActivePanel()
		return true, cmd
	case tea.KeyBackspace:
		q := []rune(p.Query)
		if len(q) > 0 {
			setSessionFilter(m, string(q[:len(q)-1]))
		}
		return true, nil
	}

	switch msg.String() {
	case "d":
		if n > 0 {
			deleteSlashSessionEntry(m)
		}
		return true, nil
	case "r":
		if n > 0 {
			startSlashSessionRename(m)
		}
		return true, nil
	}
	// Printable character → append to search filter.
	s := msg.String()
	if len([]rune(s)) == 1 && []rune(s)[0] >= 32 {
		setSessionFilter(m, p.Query+s)
		return true, nil
	}
	return true, nil
}

// scrollSessionIntoView adjusts Offset so that Selected is within the visible window.
func scrollSessionIntoView(st *slashSessionState) {
	if st.Selected < st.Offset {
		st.Offset = st.Selected
	} else if st.Selected >= st.Offset+slashSessionInlinePanelMaxContentLines {
		st.Offset = st.Selected - slashSessionInlinePanelMaxContentLines + 1
	}
}

// applySessionFilter rebuilds Filtered from All using the current Query.
func applySessionFilter(st *slashSessionState) {
	if st.Query == "" {
		st.Filtered = st.All
		return
	}
	q := strings.ToLower(st.Query)
	filtered := make([]session.SessionItem, 0, len(st.All))
	for _, e := range st.All {
		if strings.Contains(strings.ToLower(e.Title), q) ||
			strings.Contains(strings.ToLower(e.ID), q) {
			filtered = append(filtered, e)
		}
	}
	st.Filtered = filtered
}

// setSessionFilter updates the search query and rebuilds the filtered list.
func setSessionFilter(m *Model, query string) {
	if m.slashSession == nil {
		return
	}
	m.slashSession.Query = query
	applySessionFilter(m.slashSession)
	m.slashSession.Selected = 0
	m.slashSession.Offset = 0
}

func startSlashSessionRename(m *Model) {
	if m.slashSession == nil || len(m.slashSession.Filtered) == 0 {
		return
	}
	entry := m.slashSession.Filtered[m.slashSession.Selected]
	m.slashSession.Renaming = true
	m.slashSession.RenameVal = entry.Title
}

func cancelSlashSessionRename(m *Model) {
	if m.slashSession == nil {
		return
	}
	m.slashSession.Renaming = false
	m.slashSession.RenameVal = ""
}

func confirmSlashSessionRename(m *Model) {
	if m.slashSession == nil || len(m.slashSession.Filtered) == 0 {
		return
	}
	entry := m.slashSession.Filtered[m.slashSession.Selected]
	title := strings.TrimSpace(m.slashSession.RenameVal)
	if title == "" {
		cancelSlashSessionRename(m)
		return
	}
	if err := agentapp.RenameSession(m.opts.SessionsDir, entry.ID, title); err != nil {
		m.slashSession.LoadError = "rename failed: " + err.Error()
		return
	}
	entries, err := agentapp.LoadSessionList(m.opts.SessionsDir)
	if err != nil {
		m.slashSession.LoadError = err.Error()
		return
	}
	sortSessionsByCreatedAtDesc(entries)
	query := m.slashSession.Query
	st := &slashSessionState{All: entries, Query: query}
	applySessionFilter(st)
	for i, e := range st.Filtered {
		if e.ID == entry.ID {
			st.Selected = i
			break
		}
	}
	scrollSessionIntoView(st)
	m.slashSession = st
	m.activePanel = st
}

// confirmSlashSessionResume loads the selected session, prints its history to the
// terminal scrollback, and makes it the active session.
func confirmSlashSessionResume(m *Model) (tea.Model, tea.Cmd) {
	if m.slashSession == nil || len(m.slashSession.Filtered) == 0 {
		return m.closeActivePanel()
	}
	entry := m.slashSession.Filtered[m.slashSession.Selected]
	sess, err := agentapp.LoadSession(m.opts.SessionsDir, entry.ID)
	if err != nil {
		m.slashSession.LoadError = "load failed: " + err.Error()
		return m, nil
	}
	m.opts.Session = agentapp.NewSessionContext(sess, m.opts.ModelName)
	if m.opts.App != nil {
		if st, err := m.opts.App.EstimateRunStatus(m.opts.Session); err == nil {
			m.runStatus = st
		}
	}
	m.slashSession = nil
	m.activePanel = nil
	m.focusInput = true

	title := entry.Title
	if title == "" {
		title = entry.ID
	}
	separator := messageBarStyle.Render("─── Resumed: " + title + " ───")
	history := buildMessagesForScrollback(sess, m.width, m.opts.GlamourStyle)
	output := separator + "\n\n" + history
	return m, tea.Sequence(
		tea.Println(output),
		// The resumed session may have deliveries parked while it was off
		// screen; the drain hands them over now that it is the one looking.
		tea.Batch(textarea.Blink, m.inputBlock.Focus(), drainQueueCmd()),
	)
}

// deleteSlashSessionEntry deletes the selected session from disk and refreshes the list.
func deleteSlashSessionEntry(m *Model) {
	if m.slashSession == nil || len(m.slashSession.Filtered) == 0 {
		return
	}
	prevSelected := m.slashSession.Selected
	entry := m.slashSession.Filtered[prevSelected]
	if err := agentapp.DeleteSession(m.opts.SessionsDir, entry.ID); err != nil {
		m.slashSession.LoadError = "delete failed: " + err.Error()
		return
	}
	entries, err := agentapp.LoadSessionList(m.opts.SessionsDir)
	if err != nil {
		st := &slashSessionState{LoadError: err.Error()}
		m.slashSession = st
		m.activePanel = st
		return
	}
	sortSessionsByCreatedAtDesc(entries)
	if len(entries) == 0 {
		st := &slashSessionState{Empty: true}
		m.slashSession = st
		m.activePanel = st
		return
	}
	query := m.slashSession.Query
	st := &slashSessionState{All: entries, Query: query}
	applySessionFilter(st)
	newSelected := prevSelected
	if newSelected >= len(st.Filtered) {
		newSelected = len(st.Filtered) - 1
	}
	if newSelected < 0 {
		newSelected = 0
	}
	st.Selected = newSelected
	scrollSessionIntoView(st)
	m.slashSession = st
	m.activePanel = st
}

func sortSessionsByCreatedAtDesc(entries []session.SessionItem) {
	sort.SliceStable(entries, func(i, j int) bool {
		ti, ei := time.Parse(time.RFC3339, entries[i].CreatedAt)
		tj, ej := time.Parse(time.RFC3339, entries[j].CreatedAt)
		if ei != nil && ej != nil {
			return false
		}
		if ei != nil {
			return false
		}
		if ej != nil {
			return true
		}
		return ti.After(tj)
	})
}

func formatSessionListRow(maxWidth int, e *session.SessionItem) string {
	t, err := time.Parse(time.RFC3339, e.CreatedAt)
	timeStr := e.CreatedAt
	var ago string
	if err == nil {
		timeStr = t.Local().Format("01-02 15:04")
		ago = humanAgo(time.Since(t))
	}
	title := e.Title
	if title == "" {
		title = "(no title)"
	}
	suffix := "  " + ago
	prefix := timeStr + "  "
	avail := maxWidth - len([]rune(prefix)) - len([]rune(suffix))
	if avail < 4 {
		avail = 4
	}
	return prefix + truncateRunes(title, avail) + suffix
}

// humanAgo returns a compact human-readable duration like "2h ago", "3d ago", "just now".
func humanAgo(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
	}
}

func (p *slashSessionState) Render(_ *Model, maxLineWidth int) string {
	var b strings.Builder
	b.WriteString(slashSessionTitleStyle.Render("Sessions"))
	b.WriteString("\n\n")
	if p.LoadError != "" {
		b.WriteString("error: ")
		b.WriteString(truncateRunes(p.LoadError, maxLineWidth-8))
		return b.String() + "\n\nesc: close"
	}
	// Search bar
	queryDisplay := p.Query + "█"
	if p.Query == "" {
		queryDisplay = "█"
	}
	b.WriteString("/ " + queryDisplay)
	b.WriteString("\n\n")
	if p.Renaming {
		b.WriteString("Rename: ")
		b.WriteString(p.RenameVal)
		b.WriteString("█\n\n")
	}
	if p.Empty || len(p.Filtered) == 0 {
		if p.Empty {
			b.WriteString("No saved sessions yet.")
		} else {
			b.WriteString("No sessions match.")
		}
		return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
	}
	end := p.Offset + slashSessionInlinePanelMaxContentLines
	if end > len(p.Filtered) {
		end = len(p.Filtered)
	}
	for i := p.Offset; i < end; i++ {
		cursor := "  "
		if i == p.Selected {
			cursor = "› "
		}
		row := formatSessionListRow(maxLineWidth-2, &p.Filtered[i])
		b.WriteString(truncateRunes(cursor+row, maxLineWidth))
		b.WriteByte('\n')
	}
	if remaining := len(p.Filtered) - end; remaining > 0 {
		fmt.Fprintf(&b, "  … %d more\n", remaining)
	}
	out := strings.TrimRight(b.String(), "\n")
	if p.Renaming {
		return out + "\n\nenter: save rename · esc: cancel"
	}
	return out + "\n\n↑↓ select · enter: resume · r: rename · d: delete · esc: close"
}
