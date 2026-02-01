// Package tui provides the Bubble Tea TUI models and views.

package tui

import (
	"context"
	"log/slog"
	"strings"

	"buildmax/internal/agent"
	"buildmax/internal/session"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	footerLines   = 1
	inputMinLines = 1
)

// inputBoxStyle wraps the input area in a wireframe box.
var inputBoxStyle = lipgloss.NewStyle().
	BorderStyle(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("240")).
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

// Model is the root Bubble Tea model: viewport (banner + chat), input, footer.
type Model struct {
	opts    TUIOpts
	viewport viewport.Model
	input   textarea.Model
	busy    bool
	err     string // last error to show
	width   int
	height  int
}

// NewModel builds a TUI model with viewport (banner + chat), input, and stored opts.
func NewModel(opts TUIOpts) Model {
	vp := viewport.New(80, 20)
	vp.SetContent(buildViewportContent(opts.Session, opts.Version))
	vp.MouseWheelEnabled = true

	ti := textarea.New()
	ti.Placeholder = "Type a message..."
	ti.ShowLineNumbers = false
	ti.SetHeight(inputMinLines)
	ti.SetWidth(76) // leave room for input box border and padding
	// Set focus on the textarea so it receives keys and shows the cursor.
	// (Init() receives a copy of the model, so Focus() there would not persist.)
	ti.Focus()

	m := Model{
		opts:     opts,
		viewport: vp,
		input:    ti,
		width:    80,
		height:   24,
	}
	return m
}

// Init runs when the program starts; focus input and start cursor blink.
func (m Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.input.Focus())
}

// runAgent runs agent.Process in the background and sends agentDoneMsg.
func runAgent(opts TUIOpts, text string) tea.Msg {
	reply, err := opts.Agent.Process(context.Background(), opts.Session, text)
	return agentDoneMsg{Reply: reply, Err: err}
}

// Update handles messages: keys (quit, submit), resize, agentDoneMsg.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Only intercept quit and submit by key type; forward everything else to input so typing works.
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.busy {
				return m, nil
			}
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.input.Reset()
			m.busy = true
			m.err = ""
			return m, tea.Cmd(func() tea.Msg { return runAgent(m.opts, text) })
		case tea.KeyRunes:
			if len(msg.Runes) == 1 && msg.Runes[0] == 'q' {
				return m, tea.Quit
			}
		}
		// Forward all keys (including runes for typing) to input
		m.input, cmd = m.input.Update(msg)
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
		return m, nil

	case agentDoneMsg:
		m.busy = false
		if msg.Err != nil {
			m.err = msg.Err.Error()
			slog.Error("agent failed", "err", msg.Err)
		} else {
			content := buildViewportContent(m.opts.Session, m.opts.Version)
			m.viewport.SetContent(content)
			m.viewport.GotoBottom()
			if err := session.SaveToDir(m.opts.Session, m.opts.SessionsDir); err != nil {
				slog.Error("save session failed", "err", err)
				m.err = err.Error()
			}
		}
		return m, nil

	case tea.MouseMsg:
		var vpCmd tea.Cmd
		m.viewport, vpCmd = m.viewport.Update(msg)
		return m, vpCmd
	}

	// Forward other keys to input
	m.input, cmd = m.input.Update(msg)
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
		b.WriteString(inputBoxStyle.Width(boxWidth).Render("... thinking ..."))
	} else {
		b.WriteString(inputBoxStyle.Width(boxWidth).Render(m.input.View()))
	}
	b.WriteString("\n")

	// Footer
	footer := "model: " + m.opts.ModelName + " | " + m.opts.Workspace + " | ctrl+c: quit"
	if m.err != "" {
		footer += " | error: " + m.err
	}
	b.WriteString(footer)

	return b.String()
}
