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
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	footerLines      = 1
	inputMinLines    = 1
	inputMaxLines    = 3   // max lines for input; grows from 1 as user types, viewport shrinks
	carouselTick     = 400 // ms between carousel dot updates
	scrollIdleDelay  = 1500 // ms of no scroll before focus returns to input
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

// scrollIdleMsg is sent after scrollIdleDelay ms of no scroll; when its id matches lastScrollID, focus returns to input.
type scrollIdleMsg struct{ id int }

// Model is the root Bubble Tea model: viewport (banner + chat), input, footer.
type Model struct {
	opts         TUIOpts
	viewport     viewport.Model
	input        textarea.Model
	busy         bool
	err          string // last error to show
	width        int
	height       int
	carouselDots int // 0, 1, 2 for ".", "..", "..."
	focusInput   bool // true = input has focus; false = viewport has scroll focus
	lastScrollID int // used to ignore stale scroll-idle timers when user scrolls again
}

// NewModel builds a TUI model with viewport (banner + chat), input, and stored opts.
func NewModel(opts TUIOpts) Model {
	vp := viewport.New(80, 20)
	vp.SetContent(buildViewportContent(opts.Session, opts.Version, 80, false, 0))
	vp.MouseWheelEnabled = true

	ti := textarea.New()
	ti.Placeholder = "Type a message..."
	ti.ShowLineNumbers = false
	ti.SetHeight(inputMinLines) // start as one line; syncInputHeight will grow up to inputMaxLines as user types
	ti.SetWidth(76)             // leave room for input box border and padding
	// Set focus on the textarea so it receives keys and shows the cursor.
	// (Init() receives a copy of the model, so Focus() there would not persist.)
	ti.Focus()

	m := Model{
		opts:       opts,
		viewport:   vp,
		input:      ti,
		width:      80,
		height:     24,
		focusInput: true, // input focused by default
	}
	return m
}

// Init runs when the program starts; focus input and start cursor blink.
func (m Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.input.Focus())
}

// desiredInputHeight returns how many lines the input should use (1 to inputMaxLines) based on wrapped content.
func desiredInputHeight(value string, width int) int {
	if width <= 0 {
		return inputMinLines
	}
	n := 0
	for _, line := range strings.Split(value, "\n") {
		n += len(wrapLine(line, width))
	}
	if n == 0 {
		return inputMinLines
	}
	if n > inputMaxLines {
		return inputMaxLines
	}
	return n
}

// syncInputHeight sets the textarea height to match wrapped content (1 to inputMaxLines).
func (m *Model) syncInputHeight() {
	want := desiredInputHeight(m.input.Value(), m.input.Width())
	if want != m.input.Height() {
		m.input.SetHeight(want)
	}
}

// runAgentAfterUserAppended runs agent.ProcessAfterUserAppended in the background (user message already in session).
func runAgentAfterUserAppended(opts TUIOpts) tea.Msg {
	reply, err := opts.Agent.ProcessAfterUserAppended(context.Background(), opts.Session)
	return agentDoneMsg{Reply: reply, Err: err}
}

// FocusInput returns true when the input has focus, false when the viewport has scroll focus.
// Used by tests to assert focus state.
func (m Model) FocusInput() bool {
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

// Update handles messages: keys (quit, submit, focus toggle, scroll), resize, agentDoneMsg.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global quit (before focus routing)
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyRunes:
			if len(msg.Runes) == 1 && msg.Runes[0] == 'q' {
				return m, tea.Quit
			}
		}
		// Tab toggles focus between input and viewport
		if msg.Type == tea.KeyTab {
			m.focusInput = !m.focusInput
			if m.focusInput {
				m.input.Focus()
			} else {
				m.input.Blur()
				return m, m.scheduleScrollIdleReturn()
			}
			return m, nil
		}
		// When viewport has scroll focus: Enter/Esc return focus to input; other keys scroll viewport
		if !m.focusInput {
			if msg.Type == tea.KeyEnter || msg.Type == tea.KeyEscape {
				m.focusInput = true
				m.input.Focus()
				return m, nil
			}
			var vpCmd tea.Cmd
			m.viewport, vpCmd = m.viewport.Update(msg)
			return m, tea.Batch(vpCmd, m.scheduleScrollIdleReturn())
		}
		// Input focused: scroll keys (Up/Down/PgUp/PgDown/Home/End) move focus to viewport and scroll
		if m.focusInput && isScrollKey(msg) {
			m.focusInput = false
			m.input.Blur()
			var vpCmd tea.Cmd
			m.viewport, vpCmd = m.viewport.Update(msg)
			return m, tea.Batch(vpCmd, m.scheduleScrollIdleReturn())
		}
		// Input focused: existing behaviour (submit, clear, typing)
		switch msg.Type {
		case tea.KeyEscape:
			if !m.busy {
				m.input.Reset()
				(&m).syncInputHeight()
			}
			return m, nil
		case tea.KeyEnter:
			if m.busy {
				return m, nil
			}
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.input.Reset()
			(&m).syncInputHeight() // shrink back to 1 line after send
			m.opts.Session.Append(llm.Message{Role: "user", Content: text})
			content := buildViewportContent(m.opts.Session, m.opts.Version, m.width, true, 0)
			m.viewport.SetContent(content)
			m.viewport.GotoBottom()
			m.busy = true
			m.err = ""
			m.carouselDots = 0
			return m, tea.Batch(
				tea.Tick(time.Duration(carouselTick)*time.Millisecond, func(t time.Time) tea.Msg { return carouselTickMsg{} }),
				tea.Cmd(func() tea.Msg { return runAgentAfterUserAppended(m.opts) }),
			)
		}
		// Forward all other keys to input (typing)
		m.input, cmd = m.input.Update(msg)
		(&m).syncInputHeight() // grow up to 3 lines as user types
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Viewport height = total - input - footer
		inputH := m.input.Height()
		vpHeight := m.height - inputH - footerLines
		if vpHeight < 1 {
			vpHeight = 1
		}
		m.viewport.Width = m.width
		m.viewport.Height = vpHeight
		// Input width leaves room for wireframe box (border + padding)
		inputW := m.width - 4
		if inputW < 8 {
			inputW = 8
		}
		m.input.SetWidth(inputW)
		(&m).syncInputHeight() // recompute height after width change
		return m, nil

	case agentDoneMsg:
		m.busy = false
		m.carouselDots = 0
		if msg.Err != nil {
			m.err = msg.Err.Error()
			slog.Error("agent failed", "err", msg.Err)
		} else {
			content := buildViewportContent(m.opts.Session, m.opts.Version, m.width, false, 0)
			m.viewport.SetContent(content)
			m.viewport.GotoBottom()
			if err := session.SaveToDir(m.opts.Session, m.opts.SessionsDir); err != nil {
				slog.Error("save session failed", "err", err)
				m.err = err.Error()
			}
		}
		return m, nil

	case carouselTickMsg:
		if m.busy {
			m.carouselDots = (m.carouselDots + 1) % 3
			content := buildViewportContent(m.opts.Session, m.opts.Version, m.width, true, m.carouselDots)
			m.viewport.SetContent(content)
			m.viewport.GotoBottom()
			return m, tea.Tick(time.Duration(carouselTick)*time.Millisecond, func(t time.Time) tea.Msg { return carouselTickMsg{} })
		}
		return m, nil

	case tea.MouseMsg:
		// Handle wheel: if user was on input, move focus to viewport so scroll works;
		// then scroll. (When focus is on input, some terminals deliver wheel to input and we
		// never see it; after Tab or a scroll key, focus is on viewport and wheel works.)
		if msg.Action == tea.MouseActionPress {
			delta := m.viewport.MouseWheelDelta
			if delta <= 0 {
				delta = 3
			}
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if m.focusInput {
					m.focusInput = false
					m.input.Blur()
				}
				m.viewport.ScrollUp(delta)
				return m, m.scheduleScrollIdleReturn()
			case tea.MouseButtonWheelDown:
				if m.focusInput {
					m.focusInput = false
					m.input.Blur()
				}
				m.viewport.ScrollDown(delta)
				return m, m.scheduleScrollIdleReturn()
			}
		}
		var vpCmd tea.Cmd
		m.viewport, vpCmd = m.viewport.Update(msg)
		return m, vpCmd
	case scrollIdleMsg:
		// After user stops scrolling (no scroll for scrollIdleDelay ms), return focus to input.
		if msg.id == m.lastScrollID && !m.focusInput {
			m.focusInput = true
			m.input.Focus()
		}
		return m, nil
	}

	// Forward other keys to input
	m.input, cmd = m.input.Update(msg)
	(&m).syncInputHeight()
	return m, cmd
}

// View renders viewport, input, and footer.
func (m Model) View() string {
	var b strings.Builder

	// Viewport (banner + chat)
	vpHeight := m.height - m.input.Height() - footerLines
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.viewport.Height = vpHeight
	m.viewport.Width = m.width
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Input in a wireframe box (width so box fits terminal)
	boxWidth := m.width - 2
	if boxWidth < 10 {
		boxWidth = 10
	}
	if m.busy {
		b.WriteString(inputBoxStyle.Width(boxWidth).Render("Waiting for reply…"))
	} else {
		b.WriteString(inputBoxStyle.Width(boxWidth).Render(m.input.View()))
	}
	b.WriteString("\n")

	// Footer
	footer := "model: " + m.opts.ModelName + " | @" + m.opts.Workspace + " | ctrl+c: quit | esc: clear"
	if m.err != "" {
		footer += " | error: " + m.err
	}
	b.WriteString(footer)

	return b.String()
}
