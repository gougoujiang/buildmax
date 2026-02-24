// Package tui provides the Bubble Tea TUI models and views.

package tui

import (
	"buildmax/internal/session"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// ViewportBlock groups viewport state for the scrollable banner + chat area.
type ViewportBlock struct {
	viewport viewport.Model
}

// NewViewportBlock returns a ViewportBlock with the given initial width and height.
func NewViewportBlock(width, height int) ViewportBlock {
	vp := viewport.New(width, height)
	vp.MouseWheelEnabled = true
	return ViewportBlock{viewport: vp}
}

// RefreshAndGotoBottom builds viewport content from session and opts, sets it on the viewport, and scrolls to the bottom.
func (vb *ViewportBlock) RefreshAndGotoBottom(sess *session.Session, opts ViewportContentOpts) {
	content := buildViewportContent(sess, opts)
	vb.viewport.SetContent(content)
	vb.viewport.GotoBottom()
}

// SetSize sets the viewport width and height.
func (vb *ViewportBlock) SetSize(width, height int) {
	vb.viewport.Width = width
	vb.viewport.Height = height
}

// Update forwards the message to the viewport and returns its command.
func (vb *ViewportBlock) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	vb.viewport, cmd = vb.viewport.Update(msg)
	return cmd
}

// View returns the viewport's rendered content.
func (vb *ViewportBlock) View() string {
	return vb.viewport.View()
}

// ScrollUp scrolls the viewport up by delta lines.
func (vb *ViewportBlock) ScrollUp(delta int) {
	vb.viewport.ScrollUp(delta)
}

// ScrollDown scrolls the viewport down by delta lines.
func (vb *ViewportBlock) ScrollDown(delta int) {
	vb.viewport.ScrollDown(delta)
}

// MouseWheelDelta returns the viewport's mouse wheel delta (used for scroll amount).
func (vb *ViewportBlock) MouseWheelDelta() int {
	return vb.viewport.MouseWheelDelta
}
