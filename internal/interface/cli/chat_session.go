package cli

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/session"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

const slashSessionInlinePanelMaxContentLines = 14

// slashSessionPanelChromeLines are the panel's own lines: box border, title,
// the search bar, the "… N more" row, and the key hints. Renaming adds its own
// prompt line and the blank line under it.
const slashSessionPanelChromeLines = 9

type slashSessionState struct {
	LoadError string
	Empty     bool
	// Everything is every visible session on the machine. All is the current
	// scope's slice of it, and Filtered is All after the search query. Keeping
	// the widest list means toggling scope does not re-read the store, so the
	// selection survives the toggle rather than being rebuilt under the cursor.
	Everything []session.ItemSummary
	All        []session.ItemSummary
	Filtered   []session.ItemSummary
	Selected   int
	Offset     int
	Query      string
	Renaming   bool
	RenameVal  string

	// ProjectID is the Project the panel is scoped to by default. Empty means
	// this run has none, and the panel opens on every session -- a scope of
	// nothing would be an empty list with no way to explain itself.
	ProjectID   string
	ProjectName string
	AllProjects bool
	// ProjectNames labels rows and widens the search in the all-projects view.
	// Without it every row from another repository reads as an untitled session
	// that cannot be told from its neighbours.
	ProjectNames map[string]string
}

var slashSessionTitleStyle = slashPanelTitleStyle

func openSlashSession(m *Model) (tea.Model, tea.Cmd) {
	entries, err := agentapp.NewSessionManager(m.opts.SessionsDir).List()
	var st *slashSessionState
	switch {
	case err != nil:
		st = &slashSessionState{LoadError: err.Error()}
	case len(entries) == 0:
		st = &slashSessionState{Empty: true}
	default:
		sortSessionsByCreatedAtDesc(entries)
		project := m.opts.App.Project()
		st = &slashSessionState{
			Everything:   entries,
			ProjectID:    project.ID,
			ProjectName:  project.Name,
			AllProjects:  project.ID == "",
			ProjectNames: projectNamesByID(),
		}
		applySessionScope(st)
		// Pre-select the current session if it is in the list.
		if m.opts.Session != nil {
			for i, e := range st.Filtered {
				if e.ID == m.opts.Session.ID() {
					st.Selected = i
					scrollSessionIntoView(m, st)
					break
				}
			}
		}
	}
	m.slashSession = st
	return m.openPanel(st)
}

// projectNamesByID reads the Project catalog for labelling. A failure yields no
// names rather than an error: the panel's job is to list sessions, and one that
// refused to open because a label was unavailable would be worse than one whose
// rows are named by id.
func projectNamesByID() map[string]string {
	rows, err := agentapp.NewProjectManager(config.ProjectsDir()).Store().List(context.Background())
	if err != nil {
		slog.Warn("list projects for the session panel failed", "err", err)
		return nil
	}
	names := make(map[string]string, len(rows))
	for _, r := range rows {
		names[r.ID] = r.Name
	}
	return names
}

// applySessionScope narrows Everything to the current scope, then re-applies
// the search query on top of it.
func applySessionScope(st *slashSessionState) {
	if st.AllProjects || st.ProjectID == "" {
		st.All = st.Everything
	} else {
		st.All = filterByProject(st.Everything, st.ProjectID)
	}
	applySessionFilter(st)
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
			scrollSessionIntoView(m, p)
		}
		return true, nil
	case tea.KeyDown:
		if n > 0 && p.Selected < n-1 {
			p.Selected++
			scrollSessionIntoView(m, p)
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
	case "a":
		if p.ProjectID != "" {
			p.AllProjects = !p.AllProjects
			applySessionScope(p)
			p.Selected, p.Offset = 0, 0
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

// sessionRowBudget is how many session rows fit on screen. Scrolling and
// rendering both read it so the window that moves is the window that prints.
func (m *Model) sessionRowBudget(st *slashSessionState) int {
	chrome := slashSessionPanelChromeLines
	if st != nil && st.Renaming {
		chrome += 2
	}
	return m.panelListBudget(slashSessionInlinePanelMaxContentLines, chrome)
}

// scrollSessionIntoView adjusts Offset so that Selected is within the visible window.
func scrollSessionIntoView(m *Model, st *slashSessionState) {
	rows := m.sessionRowBudget(st)
	if st.Selected < st.Offset {
		st.Offset = st.Selected
	} else if st.Selected >= st.Offset+rows {
		st.Offset = st.Selected - rows + 1
	}
}

// applySessionFilter rebuilds Filtered from All using the current Query.
func applySessionFilter(st *slashSessionState) {
	if st.Query == "" {
		st.Filtered = st.All
		return
	}
	q := strings.ToLower(st.Query)
	filtered := make([]session.ItemSummary, 0, len(st.All))
	for _, e := range st.All {
		match := strings.Contains(strings.ToLower(e.Title), q) ||
			strings.Contains(strings.ToLower(e.ID), q)
		// Project name is searchable only where it is shown. In the scoped view
		// every row shares one project, so matching on it would silently return
		// the whole list for a query that appears to have found something.
		if !match && st.AllProjects {
			match = strings.Contains(strings.ToLower(st.ProjectNames[e.ProjectID]), q)
		}
		if match {
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
	if err := agentapp.NewSessionManager(m.opts.SessionsDir).Rename(entry.ID, title); err != nil {
		m.slashSession.LoadError = "rename failed: " + err.Error()
		return
	}
	entries, err := agentapp.NewSessionManager(m.opts.SessionsDir).List()
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
	scrollSessionIntoView(m, st)
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
	sess, err := agentapp.NewSessionManager(m.opts.SessionsDir).Open(entry.ID, m.opts.ModelName)
	if err != nil {
		m.slashSession.LoadError = "load failed: " + err.Error()
		return m, nil
	}
	// One writer per session: the one being left goes before the one being
	// resumed is held, or this process would hold two locks and refuse itself
	// the session it just closed.
	if m.opts.Session != nil {
		if err := m.opts.Session.Close(); err != nil {
			slog.Warn("closing the previous session failed", "err", err)
		}
	}
	m.opts.Session = sess
	if m.opts.App != nil {
		if st, err := m.opts.App.EstimateRunUsage(m.opts.Session); err == nil {
			m.runStatus = st
		}
	}
	m.slashSession = nil
	m.activePanel = nil
	m.dropStaleSuggestion()
	m.focusInput = true

	title := entry.Title
	if title == "" {
		title = entry.ID
	}
	separator := messageBarStyle.Render("─── Resumed: " + title + " ───")
	history := buildMessagesForScrollback(sess.Messages(), m.width, m.opts.GlamourStyle)
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
	if err := agentapp.NewSessionManager(m.opts.SessionsDir).Delete(entry.ID); err != nil {
		m.slashSession.LoadError = "delete failed: " + err.Error()
		return
	}
	entries, err := agentapp.NewSessionManager(m.opts.SessionsDir).List()
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
	scrollSessionIntoView(m, st)
	m.slashSession = st
	m.activePanel = st
}

func sortSessionsByCreatedAtDesc(entries []session.ItemSummary) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})
}

func formatSessionListRow(maxWidth int, e *session.ItemSummary, project string) string {
	timeStr := e.CreatedAt.Local().Format("01-02 15:04")
	ago := humanAgo(time.Since(e.CreatedAt))
	title := e.Title
	if title == "" {
		title = "(no title)"
	}
	if project != "" {
		title = project + " · " + title
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

func (p *slashSessionState) Render(m *Model, maxLineWidth int) string {
	var b strings.Builder
	b.WriteString(slashSessionTitleStyle.Render("Sessions" + p.scopeLabel()))
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
		switch {
		case p.Empty:
			b.WriteString("No saved sessions yet.")
		case p.Query == "" && !p.AllProjects:
			// Say which scope is empty and how to widen it. Falling back to
			// every session instead would answer a question the user did not
			// ask, which is the behaviour this scoping exists to remove.
			b.WriteString("No sessions in this project yet.")
		default:
			b.WriteString("No sessions match.")
		}
		return strings.TrimRight(b.String(), "\n") + "\n\n" + p.keyHints()
	}
	end := p.Offset + m.sessionRowBudget(p)
	if end > len(p.Filtered) {
		end = len(p.Filtered)
	}
	for i := p.Offset; i < end; i++ {
		cursor := "  "
		if i == p.Selected {
			cursor = "› "
		}
		row := formatSessionListRow(maxLineWidth-2, &p.Filtered[i], p.rowProject(&p.Filtered[i]))
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
	return out + "\n\n" + p.keyHints()
}

// scopeLabel names what the list is showing, so a short list is never mistaken
// for the whole machine or the reverse.
func (p *slashSessionState) scopeLabel() string {
	switch {
	case p.ProjectID == "":
		return ""
	case p.AllProjects:
		return " — all projects"
	case p.ProjectName != "":
		return " — " + p.ProjectName
	default:
		return " — this project"
	}
}

func (p *slashSessionState) keyHints() string {
	hints := "↑↓ select · enter: resume · r: rename · d: delete"
	if p.ProjectID != "" {
		if p.AllProjects {
			hints += " · a: this project"
		} else {
			hints += " · a: all projects"
		}
	}
	return hints + " · esc: close"
}

// rowProject is the label a row carries, and only in the view where rows come
// from more than one Project.
func (p *slashSessionState) rowProject(e *session.ItemSummary) string {
	if !p.AllProjects || e.ProjectID == "" {
		return ""
	}
	if name := p.ProjectNames[e.ProjectID]; name != "" {
		return name
	}
	return e.ProjectID
}
