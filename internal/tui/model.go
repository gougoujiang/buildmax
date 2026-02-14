// Package tui provides the Bubble Tea TUI models and views.

package tui

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"buildmax/internal/agent"
	"buildmax/internal/llm"
	"buildmax/internal/session"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	footerLines     = 1
	carouselTick    = 400  // ms between carousel dot updates
	scrollIdleDelay = 1500 // ms of no scroll before focus returns to input
)

// Light sky blue theme color for input border and message bar.
var lightSkyBlue = lipgloss.Color("#87CEFA")

// inputBoxStyle wraps the input area in a wireframe box (light sky blue border).
var inputBoxStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.RoundedBorder()).
	BorderForeground(lightSkyBlue).
	Padding(0, 1)

// TUIOpts holds dependencies and display config for the TUI (agent, session, model name, paths).
type TUIOpts struct {
	Agent       *agent.Agent
	LLMClient   *llm.Client
	Session     *session.Session
	ModelName   string
	Workspace   string
	Version     string
	SessionsDir string
}

// agentDoneMsg is sent when agent.Process finishes (in a tea.Cmd).
type agentDoneMsg struct {
	Reply string
	Err   error
}

// carouselTickMsg is sent by tea.Tick to advance the assistant "..." carousel.
type carouselTickMsg struct{}

// titleGeneratedMsg is sent when the async LLM title generation finishes.
type titleGeneratedMsg struct {
	Title string
	Err   error
}

// scrollIdleMsg is sent after scrollIdleDelay ms of no scroll; when its id matches lastScrollID, focus returns to input.
type scrollIdleMsg struct{ id int }

// Model is the root Bubble Tea model: viewport (banner + chat), input, footer.
type Model struct {
	opts          TUIOpts
	viewportBlock ViewportBlock
	inputBlock    InputBlock
	busy          bool
	err           string // last error to show
	width         int
	height        int
	carouselDots  int  // 0, 1, 2 for ".", "..", "..."
	focusInput    bool // true = input has focus; false = viewport has scroll focus
	lastScrollID  int  // used to ignore stale scroll-idle timers when user scrolls again
}

// NewModel builds a TUI model with viewport (banner + chat), input, and stored opts.
func NewModel(opts TUIOpts) *Model {
	m := &Model{
		opts:          opts,
		viewportBlock: NewViewportBlock(80, 20),
		inputBlock:    NewInputBlock(),
		width:         80,
		height:        24,
		focusInput:    true,
	}
	m.viewportBlock.RefreshAndGotoBottom(m.opts.Session, m.opts.Version, m.width, false, 0)
	return m
}

// Init runs when the program starts; focus input and start cursor blink.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.inputBlock.Focus())
}

// runAgentAfterUserAppended runs agent.ProcessAfterUserAppended in the background (user message already in session).
func runAgentAfterUserAppended(opts TUIOpts) tea.Msg {
	reply, _, err := opts.Agent.ProcessAfterUserAppended(context.Background(), opts.Session)
	return agentDoneMsg{Reply: reply, Err: err}
}

// FocusInput returns true when the input has focus, false when the viewport has scroll focus.
// Used by tests to assert focus state.
func (m *Model) FocusInput() bool {
	return m.focusInput
}

// isScrollKey returns true for keys that scroll the viewport (Up, Down, PgUp, PgDown, Home, End).
func isScrollKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd:
		return true
	default:
		return false
	}
}

// scheduleScrollIdleReturn increments lastScrollID and returns a Cmd that sends scrollIdleMsg after scrollIdleDelay.
// When that message is received and its id matches lastScrollID, focus returns to input.
func (m *Model) scheduleScrollIdleReturn() tea.Cmd {
	m.lastScrollID++
	id := m.lastScrollID
	return tea.Tick(scrollIdleDelay*time.Millisecond, func(t time.Time) tea.Msg { return scrollIdleMsg{id: id} })
}

func handleKeyMsg(m *Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyRunes:
		if len(msg.Runes) == 1 && msg.Runes[0] == 'q' {
			return m, tea.Quit
		}
	}
	if msg.Type == tea.KeyTab {
		m.focusInput = !m.focusInput
		if m.focusInput {
			m.inputBlock.Focus()
		} else {
			m.inputBlock.Blur()
			return m, m.scheduleScrollIdleReturn()
		}
		return m, nil
	}
	if !m.focusInput {
		if msg.Type == tea.KeyEnter || msg.Type == tea.KeyEscape {
			m.focusInput = true
			m.inputBlock.Focus()
			return m, nil
		}
		vpCmd := m.viewportBlock.Update(msg)
		return m, tea.Batch(vpCmd, m.scheduleScrollIdleReturn())
	}
	if m.focusInput && isScrollKey(msg) {
		m.focusInput = false
		m.inputBlock.Blur()
		vpCmd := m.viewportBlock.Update(msg)
		return m, tea.Batch(vpCmd, m.scheduleScrollIdleReturn())
	}
	switch msg.Type {
	case tea.KeyEscape:
		if !m.busy {
			m.inputBlock.Reset()
			m.inputBlock.SyncHeight()
		}
		return m, nil
	case tea.KeyEnter:
		if m.busy {
			return m, nil
		}
		text := strings.TrimSpace(m.inputBlock.Value())
		if text == "" {
			return m, nil
		}
		m.inputBlock.Reset()
		m.inputBlock.SyncHeight()
		m.opts.Session.Append(llm.Message{Role: "user", Content: text})
		m.viewportBlock.RefreshAndGotoBottom(m.opts.Session, m.opts.Version, m.width, true, 0)
		m.busy = true
		m.err = ""
		m.carouselDots = 0
		return m, tea.Batch(
			tea.Tick(time.Duration(carouselTick)*time.Millisecond, func(t time.Time) tea.Msg { return carouselTickMsg{} }),
			tea.Cmd(func() tea.Msg { return runAgentAfterUserAppended(m.opts) }),
		)
	}
	cmd := m.inputBlock.Update(msg)
	m.inputBlock.SyncHeight()
	return m, cmd
}

func handleWindowSize(m *Model, msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	inputH := m.inputBlock.Height()
	vpHeight := m.height - inputH - footerLines
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.viewportBlock.SetSize(m.width, vpHeight)
	inputW := m.width - 4
	if inputW < 8 {
		inputW = 8
	}
	m.inputBlock.SetWidth(inputW)
	m.inputBlock.SyncHeight()
	return m, nil
}

func handleAgentDone(m *Model, msg agentDoneMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	m.carouselDots = 0
	if msg.Err != nil {
		m.err = msg.Err.Error()
		slog.Error("agent failed", "err", msg.Err)
		return m, nil
	}
	m.viewportBlock.RefreshAndGotoBottom(m.opts.Session, m.opts.Version, m.width, false, 0)

	// Remember whether the session had no title before persist (i.e. first turn).
	needsLLMTitle := m.opts.Session.Title() == ""

	if err := session.PersistAfterReply(m.opts.Session, m.opts.SessionsDir, m.opts.Workspace, 100); err != nil {
		slog.Error("persist session failed", "err", err)
		m.err = err.Error()
		return m, nil
	}

	// Fire async LLM title generation for new sessions.
	if needsLLMTitle && m.opts.LLMClient != nil {
		return m, generateTitleCmd(m.opts)
	}
	return m, nil
}

// generateTitleCmd returns a tea.Cmd that calls the LLM to generate a session title in the background.
func generateTitleCmd(opts TUIOpts) tea.Cmd {
	return func() tea.Msg {
		chatFn := func(ctx context.Context, msgs []llm.Message) (string, error) {
			content, _, err := opts.LLMClient.ChatWithTools(ctx, msgs, nil)
			return content, err
		}
		title, err := session.GenerateTitle(context.Background(), chatFn, opts.Session.Messages())
		return titleGeneratedMsg{Title: title, Err: err}
	}
}

func handleTitleGenerated(m *Model, msg titleGeneratedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		slog.Warn("LLM title generation failed", "err", msg.Err)
		return m, nil
	}
	if msg.Title == "" {
		return m, nil
	}
	m.opts.Session.SetTitle(msg.Title)
	// Re-persist with the LLM-generated title.
	if err := session.PersistAfterReply(m.opts.Session, m.opts.SessionsDir, m.opts.Workspace, 100); err != nil {
		slog.Error("re-persist session with LLM title failed", "err", err)
	}
	return m, nil
}

func handleCarouselTick(m *Model, msg carouselTickMsg) (tea.Model, tea.Cmd) {
	if m.busy {
		m.carouselDots = (m.carouselDots + 1) % 3
		m.viewportBlock.RefreshAndGotoBottom(m.opts.Session, m.opts.Version, m.width, true, m.carouselDots)
		return m, tea.Tick(time.Duration(carouselTick)*time.Millisecond, func(t time.Time) tea.Msg { return carouselTickMsg{} })
	}
	return m, nil
}

func handleMouseMsg(m *Model, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionPress {
		delta := m.viewportBlock.MouseWheelDelta()
		if delta <= 0 {
			delta = 3
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.focusInput {
				m.focusInput = false
				m.inputBlock.Blur()
			}
			m.viewportBlock.ScrollUp(delta)
			return m, m.scheduleScrollIdleReturn()
		case tea.MouseButtonWheelDown:
			if m.focusInput {
				m.focusInput = false
				m.inputBlock.Blur()
			}
			m.viewportBlock.ScrollDown(delta)
			return m, m.scheduleScrollIdleReturn()
		}
	}
	vpCmd := m.viewportBlock.Update(msg)
	return m, vpCmd
}

func handleScrollIdle(m *Model, msg scrollIdleMsg) (tea.Model, tea.Cmd) {
	if msg.id == m.lastScrollID && !m.focusInput {
		m.focusInput = true
		m.inputBlock.Focus()
	}
	return m, nil
}

// Update handles messages: keys (quit, submit, focus toggle, scroll), resize, agentDoneMsg.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleKeyMsg(m, msg)
	case tea.WindowSizeMsg:
		return handleWindowSize(m, msg)
	case agentDoneMsg:
		return handleAgentDone(m, msg)
	case titleGeneratedMsg:
		return handleTitleGenerated(m, msg)
	case carouselTickMsg:
		return handleCarouselTick(m, msg)
	case tea.MouseMsg:
		return handleMouseMsg(m, msg)
	case scrollIdleMsg:
		return handleScrollIdle(m, msg)
	default:
		cmd := m.inputBlock.Update(msg)
		m.inputBlock.SyncHeight()
		return m, cmd
	}
}

// View renders viewport, input, and footer.
func (m *Model) View() string {
	var b strings.Builder

	// Viewport (banner + chat)
	vpHeight := m.height - m.inputBlock.Height() - footerLines
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.viewportBlock.SetSize(m.width, vpHeight)
	b.WriteString(m.viewportBlock.View())
	b.WriteString("\n")

	// Input in a wireframe box (width so box fits terminal)
	boxWidth := m.width - 2
	if boxWidth < 10 {
		boxWidth = 10
	}
	if m.busy {
		b.WriteString(inputBoxStyle.Width(boxWidth).Render("Waiting for reply…"))
	} else {
		b.WriteString(inputBoxStyle.Width(boxWidth).Render(m.inputBlock.View()))
	}
	b.WriteString("\n")

	// Footer
	footer := "model: " + m.opts.ModelName + " | @" + m.opts.Workspace + " | ctrl+c: quit | esc: clear/focus input"
	if m.err != "" {
		footer += " | error: " + m.err
	}
	b.WriteString(footer)

	return b.String()
}
