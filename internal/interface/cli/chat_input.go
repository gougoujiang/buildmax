// Package cli provides the BuildMax CLI commands and interactive Bubble Tea UI.

package cli

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

const (
	inputMinLines = 1
	inputMaxLines = 3 // max lines for input; grows from 1 as user types, viewport shrinks
	// idlePlaceholder is what the input says when it has no ghost suggestion.
	idlePlaceholder = "Type a message..."
)

// InputBlock groups input state for the textarea at the bottom.
type InputBlock struct {
	input textarea.Model
	// ghost is the predicted answer offered as dim text, accepted with tab.
	// It rides on the textarea's placeholder because the two have the same
	// rule — shown only while the input is empty — so typing hides the
	// suggestion without anything here having to notice.
	ghost string
}

// NewInputBlock returns an InputBlock with textarea configured (prompt, placeholder, initial height/width, focused).
func NewInputBlock() InputBlock {
	ti := textarea.New()
	ti.Prompt = "> "
	ti.Placeholder = idlePlaceholder
	ti.ShowLineNumbers = false
	ti.SetHeight(inputMinLines)
	ti.SetWidth(76) // leave room for input box border and padding
	ti.Focus()
	return InputBlock{input: ti}
}

// SetGhost offers text as the ghost suggestion, or clears it when s is empty.
func (ib *InputBlock) SetGhost(s string) {
	ib.ghost = s
	if s == "" {
		ib.input.Placeholder = idlePlaceholder
	} else {
		ib.input.Placeholder = s
	}
	ib.SyncHeight()
}

// Ghost returns the suggestion currently on offer, or "" when there is none.
// It is on offer only while the input is empty: once the user has typed, the
// suggestion is not what they are about to send.
func (ib *InputBlock) Ghost() string {
	if ib.input.Value() != "" {
		return ""
	}
	return ib.ghost
}

// desiredInputHeight returns how many lines the input should use (1 to inputMaxLines) based on wrapped content.
func desiredInputHeight(value string, width int) int {
	n := wrappedLineCount(value, width)
	if n > inputMaxLines {
		return inputMaxLines
	}
	return n
}

func wrappedLineCount(value string, width int) int {
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
	return n
}

// SyncHeight sets the textarea height to match wrapped content (1 to inputMaxLines).
//
// An empty input is measured against the ghost suggestion instead: the
// textarea renders the placeholder over its own height, so a two-line
// suggestion in a one-line box would be silently cut in half.
func (ib *InputBlock) SyncHeight() {
	measured := ib.input.Value()
	if measured == "" {
		measured = ib.ghost
	}
	want := desiredInputHeight(measured, ib.input.Width())
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

// SetValue replaces the textarea content.
func (ib *InputBlock) SetValue(v string) {
	ib.input.SetValue(v)
}

// Height returns the textarea height in lines.
func (ib *InputBlock) Height() int {
	return ib.input.Height()
}

// CanScroll reports whether the textarea has hidden wrapped lines above or below.
func (ib *InputBlock) CanScroll() bool {
	return wrappedLineCount(ib.input.Value(), ib.input.Width()) > ib.input.Height()
}

// Update forwards the message to the textarea and returns its command.
func (ib *InputBlock) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	ib.input, cmd = ib.input.Update(msg)
	return cmd
}

// ScrollUp moves the textarea viewport/cursor upward by the requested number of lines.
func (ib *InputBlock) ScrollUp(lines int) tea.Cmd {
	var cmds []tea.Cmd
	for i := 0; i < lines; i++ {
		cmds = append(cmds, ib.Update(tea.KeyPressMsg{Code: tea.KeyUp}))
	}
	return tea.Batch(cmds...)
}

// ScrollDown moves the textarea viewport/cursor downward by the requested number of lines.
func (ib *InputBlock) ScrollDown(lines int) tea.Cmd {
	var cmds []tea.Cmd
	for i := 0; i < lines; i++ {
		cmds = append(cmds, ib.Update(tea.KeyPressMsg{Code: tea.KeyDown}))
	}
	return tea.Batch(cmds...)
}

// View returns the textarea's rendered content.
func (ib *InputBlock) View() string {
	return ib.input.View()
}
