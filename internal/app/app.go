// Package app provides the application bootstrap and TUI program.
package app

import (
	"github.com/charmbracelet/bubbletea"
	"github.com/gougoujiang/buildmax/internal/tui"
)

// NewModel returns the root Bubble Tea model for the TUI.
func NewModel() tea.Model {
	return tui.NewModel()
}
