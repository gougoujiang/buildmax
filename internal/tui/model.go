// Package tui provides the Bubble Tea TUI models and views.
package tui

import (
	"github.com/charmbracelet/bubbletea"
)

// Model is the root TUI model.
type Model struct {
	ready bool
}

// NewModel returns a new TUI model.
func NewModel() Model {
	return Model{}
}

// Init runs when the program starts.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.ready = true
	}
	return m, nil
}

// View renders the UI.
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}
	return "\n  BuildMax – AI Agent TUI\n\n  Press q or ctrl+c to quit.\n\n"
}
