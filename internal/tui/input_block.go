// Package tui provides the Bubble Tea TUI models and views.

package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	inputMinLines = 1
	inputMaxLines = 3 // max lines for input; grows from 1 as user types, viewport shrinks
)

// InputBlock groups input state for the textarea at the bottom.
type InputBlock struct {
	input textarea.Model
}

// NewInputBlock returns an InputBlock with textarea configured (prompt, placeholder, initial height/width, focused).
func NewInputBlock() InputBlock {
	ti := textarea.New()
	ti.Prompt = "> "
	ti.Placeholder = "Type a message..."
	ti.ShowLineNumbers = false
	ti.SetHeight(inputMinLines)
	ti.SetWidth(76) // leave room for input box border and padding
	ti.Focus()
	return InputBlock{input: ti}
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

// SyncHeight sets the textarea height to match wrapped content (1 to inputMaxLines).
func (ib *InputBlock) SyncHeight() {
	want := desiredInputHeight(ib.input.Value(), ib.input.Width())
	if want != ib.input.Height() {
		ib.input.SetHeight(want)
	}
}

// SetWidth sets the textarea width.
func (ib *InputBlock) SetWidth(w int) {
	ib.input.SetWidth(w)
}

// Focus focuses the textarea and returns its command (for tea.Batch in Init).
func (ib *InputBlock) Focus() tea.Cmd {
	return ib.input.Focus()
}

// Blur blurs the textarea.
func (ib *InputBlock) Blur() {
	ib.input.Blur()
}

// Reset clears the textarea value.
func (ib *InputBlock) Reset() {
	ib.input.Reset()
}

// Value returns the textarea value.
func (ib *InputBlock) Value() string {
	return ib.input.Value()
}

// Height returns the textarea height in lines.
func (ib *InputBlock) Height() int {
	return ib.input.Height()
}

// Update forwards the message to the textarea and returns its command.
func (ib *InputBlock) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	ib.input, cmd = ib.input.Update(msg)
	return cmd
}

// View returns the textarea's rendered content.
func (ib *InputBlock) View() string {
	return ib.input.View()
}
