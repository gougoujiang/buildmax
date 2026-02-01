// Package app provides the application bootstrap and TUI program.
package app

import (
	"github.com/charmbracelet/bubbletea"
	"buildmax/internal/tui"
)

// NewModel returns the root Bubble Tea model for the TUI with the given opts.
func NewModel(opts tui.TUIOpts) tea.Model {
	return tui.NewModel(opts)
}
