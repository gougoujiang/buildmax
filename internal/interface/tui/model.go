// Package tui provides the Bubble Tea TUI models and views.

package tui

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"buildmax/internal/core/agent"
	"buildmax/internal/core/model"
	llm "buildmax/internal/infra/llm"
	"buildmax/internal/session"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
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

// Footer styles: model in green, workspace/branch in sky blue.
var footerModelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")) // green
var footerBranchStyle = lipgloss.NewStyle().Foreground(lightSkyBlue)             // sky blue

// TUIOpts holds dependencies and display config for the TUI (agent, session, model name, paths).
type TUIOpts struct {
	Agent       *agent.Agent
	LLMClient   *llm.Client
	Session     *session.Session
	ModelName   string
	Workspace   string
	Branch      string // current git branch (empty if not a repo); footer shows "branch: -" when empty
	Version     string
	SessionsDir string
	UserEmail   string // logged-in user email; empty if not logged in
}

// agentDoneMsg is sent when agent.Process finishes (in a tea.Cmd).
type agentDoneMsg struct {
	Reply string
	Stats agent.RunStats
	Err   error
}

// streamDeltaMsg is sent when a content delta arrives from the streaming LLM.
type streamDeltaMsg struct {
	Delta string
}

// carouselTickMsg is sent by tea.Tick to advance the assistant "..." carousel.
type carouselTickMsg struct{}

// titleGeneratedMsg is sent when the async LLM title generation finishes.
type titleGeneratedMsg struct {
	Title            string
	PromptTokens     int
	CompletionTokens int
	Err              error
}

// scrollIdleMsg is sent after scrollIdleDelay ms of no scroll; when its id matches lastScrollID, focus returns to input.
type scrollIdleMsg struct{ id int }

// Model is the root Bubble Tea model: viewport (banner + chat), input, footer.
type Model struct {
	opts            TUIOpts
	viewportBlock   ViewportBlock
	inputBlock      InputBlock
	busy            bool
	err             string // last error to show
	width           int
	height          int
	carouselDots    int          // 0, 1, 2 for ".", "..", "..."
	focusInput      bool         // true = input has focus; false = viewport has scroll focus
	lastScrollID    int          // used to ignore stale scroll-idle timers when user scrolls again
	streamingBuffer string       // current turn's assistant text while streaming
	streamChannel   chan tea.Msg // receives streamDeltaMsg and agentDoneMsg; set when agent run starts
	mcpOverlay      *mcpOverlayState
	skillsOverlay   *skillsOverlayState  // /skills list panel above the input
	sessionOverlay  *sessionOverlayState // /session list panel above the input
	slashPopup      *slashPopupState     // live /command completion above input
}

// streamSinkToChannel implements agent.StreamSink by sending streamDeltaMsg to a channel.
type streamSinkToChannel struct {
	channel chan tea.Msg
}

func (s *streamSinkToChannel) OnDelta(delta string) {
	s.channel <- streamDeltaMsg{Delta: delta}
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
	m.viewportBlock.RefreshAndGotoBottom(m.opts.Session, ViewportContentOpts{Version: m.opts.Version, Width: m.width})
	return m
}

// Init runs when the program starts; focus input and start cursor blink.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.inputBlock.Focus())
}

// runAgentWithStream starts the agent in a goroutine with a stream sink that sends deltas to channel,
// then sends agentDoneMsg and closes the channel. It returns a Cmd that reads one message from the channel.
func runAgentWithStream(opts TUIOpts, channel chan tea.Msg) tea.Cmd {
	sink := &streamSinkToChannel{channel: channel}
	go func() {
		ctx := session.CtxWithSessionID(context.Background(), opts.Session.ID())
		reply, stats, err := opts.Agent.ProcessAfterUserAppended(ctx, opts.Session, agent.WithStreamSink(sink))
		channel <- agentDoneMsg{Reply: reply, Stats: stats, Err: err}
		close(channel)
	}()
	return func() tea.Msg { return <-channel }
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
	if m.focusInput && m.slashPopup != nil && len(m.slashPopup.matches) > 0 {
		switch msg.Type {
		case tea.KeyUp:
			if m.slashPopup.selected > 0 {
				m.slashPopup.selected--
			} else {
				m.slashPopup.selected = len(m.slashPopup.matches) - 1
			}
			return m, nil
		case tea.KeyDown:
			m.slashPopup.selected = (m.slashPopup.selected + 1) % len(m.slashPopup.matches)
			return m, nil
		}
	}
	if m.focusInput && isScrollKey(msg) {
		m.focusInput = false
		m.inputBlock.Blur()
		vpCmd := m.viewportBlock.Update(msg)
		return m, tea.Batch(vpCmd, m.scheduleScrollIdleReturn())
	}
	switch msg.Type {
	case tea.KeyEscape:
		if m.busy {
			return m, nil
		}
		if m.slashPopup != nil {
			m.slashPopup = nil
			return m, nil
		}
		if m.sessionOverlay != nil {
			return closeSessionOverlay(m)
		}
		if m.skillsOverlay != nil {
			return closeSkillsOverlay(m)
		}
		if m.mcpOverlay != nil {
			return closeMCPOverlay(m)
		}
		m.inputBlock.Reset()
		m.inputBlock.SyncHeight()
		return m, nil
	case tea.KeyEnter:
		if m.busy {
			return m, nil
		}
		if m.slashPopup != nil && len(m.slashPopup.matches) > 0 {
			cmd := m.slashPopup.matches[m.slashPopup.selected]
			m.slashPopup = nil
			m.inputBlock.Reset()
			m.inputBlock.SyncHeight()
			return dispatchSlashCommand(m, cmd)
		}
		text := strings.TrimSpace(m.inputBlock.Value())
		if text == "" {
			return m, nil
		}
		if strings.HasPrefix(text, "/") {
			m.inputBlock.Reset()
			m.inputBlock.SyncHeight()
			m.slashPopup = nil
			parts := strings.Fields(text)
			if len(parts) == 0 {
				return m, nil
			}
			return dispatchSlashCommand(m, parts[0])
		}
		m.inputBlock.Reset()
		m.inputBlock.SyncHeight()
		m.opts.Session.Append(model.Message{Role: "user", Content: text})
		m.viewportBlock.RefreshAndGotoBottom(m.opts.Session, ViewportContentOpts{Version: m.opts.Version, Width: m.width, Busy: true})
		m.busy = true
		m.err = ""
		m.carouselDots = 0
		m.streamingBuffer = ""
		channel := make(chan tea.Msg)
		m.streamChannel = channel
		return m, tea.Batch(
			tea.Tick(time.Duration(carouselTick)*time.Millisecond, func(t time.Time) tea.Msg { return carouselTickMsg{} }),
			runAgentWithStream(m.opts, channel),
		)
	}
	cmd := m.inputBlock.Update(msg)
	m.inputBlock.SyncHeight()
	m.syncSlashPopupFromInput()
	return m, cmd
}

func handleWindowSize(m *Model, msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	inputW := m.width - 4
	if inputW < 8 {
		inputW = 8
	}
	m.inputBlock.SetWidth(inputW)
	m.inputBlock.SyncHeight()
	m.syncViewportSize()
	// Rebuild viewport content with new width so text wraps to full width (fixes --continue narrow content).
	m.viewportBlock.RefreshAndGotoBottom(m.opts.Session, ViewportContentOpts{
		Version: m.opts.Version, Width: m.width,
		Busy: m.busy, CarouselDots: m.carouselDots, StreamingTail: m.streamingBuffer,
	})
	m.syncSlashPopupFromInput()
	return m, nil
}

func handleAgentDone(m *Model, msg agentDoneMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	m.carouselDots = 0
	m.streamingBuffer = ""
	m.streamChannel = nil
	if msg.Err != nil {
		m.err = msg.Err.Error()
		slog.Error("agent failed", "err", msg.Err)
		return m, nil
	}
	m.viewportBlock.RefreshAndGotoBottom(m.opts.Session, ViewportContentOpts{Version: m.opts.Version, Width: m.width})

	// Remember whether the session had no title before persist (i.e. first turn).
	needsLLMTitle := m.opts.Session.Title() == ""

	m.opts.Session.AddUsage(msg.Stats.PromptTokens, msg.Stats.CompletionTokens)
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
		titleClient := session.TitleChatFunc(func(ctx context.Context, msgs []model.Message) (string, model.Usage, error) {
			content, _, usage, err := opts.LLMClient.ChatWithTools(ctx, msgs, nil)
			return content, usage, err
		})
		title, usage, err := session.GenerateTitle(context.Background(), titleClient, opts.Session.Messages())
		return titleGeneratedMsg{Title: title, PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, Err: err}
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
	if msg.PromptTokens > 0 || msg.CompletionTokens > 0 {
		m.opts.Session.AddUsage(msg.PromptTokens, msg.CompletionTokens)
	}
	m.opts.Session.SetTitle(msg.Title)
	// Re-persist with the LLM-generated title and title usage.
	if err := session.PersistAfterReply(m.opts.Session, m.opts.SessionsDir, m.opts.Workspace, 100); err != nil {
		slog.Error("re-persist session with LLM title failed", "err", err)
	}
	return m, nil
}

func handleCarouselTick(m *Model, msg carouselTickMsg) (tea.Model, tea.Cmd) {
	if m.busy {
		m.carouselDots = (m.carouselDots + 1) % 3
		m.viewportBlock.RefreshAndGotoBottom(m.opts.Session, ViewportContentOpts{Version: m.opts.Version, Width: m.width, Busy: true, CarouselDots: m.carouselDots, StreamingTail: m.streamingBuffer})
		return m, tea.Tick(time.Duration(carouselTick)*time.Millisecond, func(t time.Time) tea.Msg { return carouselTickMsg{} })
	}
	return m, nil
}

func handleStreamDelta(m *Model, msg streamDeltaMsg) (tea.Model, tea.Cmd) {
	m.streamingBuffer += msg.Delta
	m.viewportBlock.RefreshAndGotoBottom(m.opts.Session, ViewportContentOpts{Version: m.opts.Version, Width: m.width, Busy: true, CarouselDots: m.carouselDots, StreamingTail: m.streamingBuffer})
	if m.streamChannel == nil {
		return m, nil
	}
	return m, func() tea.Msg { return <-m.streamChannel }
}

func handleMouseMsg(m *Model, msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionPress && m.inputBlock.CanScroll() && m.focusInput {
		delta := m.viewportBlock.MouseWheelDelta()
		if delta <= 0 {
			delta = 3
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			return m, m.inputBlock.ScrollUp(delta)
		case tea.MouseButtonWheelDown:
			return m, m.inputBlock.ScrollDown(delta)
		}
	}
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

func (m *Model) renderInputView() string {
	boxWidth := m.width - 2
	if boxWidth < 10 {
		boxWidth = 10
	}
	if m.busy {
		return inputBoxStyle.Width(boxWidth).Render("Waiting for reply…")
	}
	return inputBoxStyle.Width(boxWidth).Render(m.inputBlock.View())
}

func (m *Model) renderFooterView() string {
	workspacePart := "@" + m.opts.Workspace
	if m.opts.Branch != "" {
		workspacePart += " (|-" + m.opts.Branch + ")"
	}
	line1 := footerModelStyle.Render("model: "+m.opts.ModelName) + " | " +
		footerBranchStyle.Render(workspacePart)
	if m.opts.UserEmail != "" {
		line1 += " | " + m.opts.UserEmail
	}

	line2 := "ctrl+c: quit | esc: clear/dismiss | opt+mouse: select text | /… + ↑↓: commands"
	if m.sessionOverlay != nil {
		line2 += " | esc: close sessions panel"
	}
	if m.skillsOverlay != nil {
		line2 += " | esc: close skills panel"
	}
	if m.mcpOverlay != nil {
		line2 += " | esc: close MCP panel"
	}
	if m.err != "" {
		line2 += " | error: " + m.err
	}
	return line1 + "\n" + line2
}

func (m *Model) syncViewportSize() {
	wasAtBottom := m.viewportBlock.AtBottom()
	inputHeight := lipgloss.Height(m.renderInputView())
	footerHeight := lipgloss.Height(m.renderFooterView())
	extra := 0
	if s := m.renderMCPInlinePanel(); s != "" {
		extra += lipgloss.Height(s)
	}
	if s := m.renderSkillsInlinePanel(); s != "" {
		extra += lipgloss.Height(s)
	}
	if s := m.renderSessionInlinePanel(); s != "" {
		extra += lipgloss.Height(s)
	}
	if s := m.renderSlashPopupPanel(); s != "" {
		extra += lipgloss.Height(s)
	}
	vpHeight := m.height - inputHeight - footerHeight - extra
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.viewportBlock.SetSize(m.width, vpHeight)
	if wasAtBottom {
		m.viewportBlock.GotoBottom()
	}
}

// Update handles messages: keys (quit, submit, focus toggle, scroll), resize, agentDoneMsg, streamDeltaMsg.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return handleKeyMsg(m, msg)
	case tea.WindowSizeMsg:
		return handleWindowSize(m, msg)
	case agentDoneMsg:
		return handleAgentDone(m, msg)
	case streamDeltaMsg:
		return handleStreamDelta(m, msg)
	case titleGeneratedMsg:
		return handleTitleGenerated(m, msg)
	case carouselTickMsg:
		return handleCarouselTick(m, msg)
	case tea.MouseMsg:
		return handleMouseMsg(m, msg)
	case scrollIdleMsg:
		return handleScrollIdle(m, msg)
	case mcpProbeDoneMsg:
		return handleMCPProbeDone(m, msg)
	default:
		cmd := m.inputBlock.Update(msg)
		m.inputBlock.SyncHeight()
		m.syncSlashPopupFromInput()
		return m, cmd
	}
}

// View renders viewport, optional MCP/skills/session panels and slash popup above the input, then footer.
func (m *Model) View() string {
	inputView := m.renderInputView()
	footerView := m.renderFooterView()
	m.syncViewportSize()
	parts := []string{m.viewportBlock.View()}
	if s := m.renderMCPInlinePanel(); s != "" {
		parts = append(parts, s)
	}
	if s := m.renderSkillsInlinePanel(); s != "" {
		parts = append(parts, s)
	}
	if s := m.renderSessionInlinePanel(); s != "" {
		parts = append(parts, s)
	}
	if s := m.renderSlashPopupPanel(); s != "" {
		parts = append(parts, s)
	}
	parts = append(parts, inputView, footerView)
	return strings.Join(parts, "\n")
}
