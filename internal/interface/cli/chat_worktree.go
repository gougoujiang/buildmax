package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/agentapp/worktree"

	tea "charm.land/bubbletea/v2"
)

const slashWorktreeInlinePanelMaxLines = 14

// slashWorktreePanelChromeLines are the panel's own lines: box border, title,
// the blank line, the "… N more" row, and the key hint.
const slashWorktreePanelChromeLines = 7

// slashWorktreePanel implements slashPanel for the /worktree overlay. It shows
// what exists and who is in it — the visibility the design record owes a user
// in place of automatic cleanup, since nothing removes a worktree on its own.
type slashWorktreePanel struct {
	Entries []worktree.Info
	Err     string
	Current string
}

func openSlashWorktree(m *Model) (tea.Model, tea.Cmd) {
	panel := &slashWorktreePanel{}
	mgr := worktreeManager(m)
	if mgr == nil {
		panel.Err = "Worktrees are not available in this session."
		return m.openPanel(panel)
	}
	panel.Current = mgr.Current()
	entries, err := mgr.List(context.Background())
	if err != nil {
		panel.Err = err.Error()
	} else {
		panel.Entries = entries
	}
	return m.openPanel(panel)
}

// worktreeManager reaches the runtime's worktree lifecycle, nil when the
// surface does not offer one.
func worktreeManager(m *Model) *worktree.Manager {
	if m.opts.App == nil {
		return nil
	}
	return m.opts.App.Worktrees()
}

func (p *slashWorktreePanel) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if msg.Code == tea.KeyEscape {
		_, cmd := m.closeActivePanel()
		return true, cmd
	}
	return false, nil
}

func (p *slashWorktreePanel) FooterHint() string { return "esc: close worktree panel" }

func (p *slashWorktreePanel) OnClose(_ *Model) {}

func (p *slashWorktreePanel) Render(m *Model, maxLineWidth int) string {
	var b strings.Builder
	b.WriteString(slashPanelTitleStyle.Render("Worktrees"))
	b.WriteString("\n\n")
	if p.Err != "" {
		b.WriteString(truncateRunes(p.Err, maxLineWidth))
		return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
	}
	if len(p.Entries) == 0 {
		b.WriteString("This repository has no worktrees.\nAsk the agent to open one, or keep working here.")
		return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
	}
	budget := m.panelListBudget(slashWorktreeInlinePanelMaxLines, slashWorktreePanelChromeLines)
	shown := 0
	for _, e := range p.Entries {
		if shown+1 > budget {
			if rem := len(p.Entries) - shown; rem > 0 {
				fmt.Fprintf(&b, "… %d more\n", rem)
			}
			break
		}
		line := e.Name + " — " + e.Path
		switch {
		case e.Current:
			line += "  [this session]"
		case e.Occupied:
			line += "  [in use by " + e.Holder + "]"
		}
		b.WriteString(truncateRunes(line, maxLineWidth))
		b.WriteByte('\n')
		shown++
	}
	return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
}
