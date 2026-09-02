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
// what exists, who is in it, and what each tree holds uncommitted — the
// visibility the design record owes a user in place of automatic cleanup (D5) —
// and lets the user remove a tree, the one action that follows from seeing a
// stale one nothing else will ever reap.
type slashWorktreePanel struct {
	Entries  []worktree.Info
	Err      string
	Selected int
	Offset   int
	// Confirming gates the removal of Entries[Selected] behind a second
	// keypress, because a worktree may hold the only copy of its work (D4).
	Confirming bool
	// Notice carries the outcome of the last action or a refusal, shown under
	// the list until the next navigation.
	Notice string
}

func openSlashWorktree(m *Model) (tea.Model, tea.Cmd) {
	panel := &slashWorktreePanel{}
	mgr := worktreeManager(m)
	if mgr == nil {
		panel.Err = "Worktrees are not available in this session."
		return m.openPanel(panel)
	}
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
	if p.Confirming {
		switch msg.Code {
		case tea.KeyEnter:
			confirmWorktreeRemove(m, p)
			return true, nil
		case tea.KeyEscape:
			p.Confirming, p.Notice = false, ""
			return true, nil
		}
		switch msg.String() {
		case "y":
			confirmWorktreeRemove(m, p)
		case "n":
			p.Confirming, p.Notice = false, ""
		}
		return true, nil
	}

	n := len(p.Entries)
	switch msg.Code {
	case tea.KeyUp:
		if n > 0 && p.Selected > 0 {
			p.Selected--
			p.Notice = ""
			scrollWorktreeIntoView(m, p)
		}
		return true, nil
	case tea.KeyDown:
		if n > 0 && p.Selected < n-1 {
			p.Selected++
			p.Notice = ""
			scrollWorktreeIntoView(m, p)
		}
		return true, nil
	case tea.KeyEscape:
		_, cmd := m.closeActivePanel()
		return true, cmd
	}
	if msg.String() == "d" && n > 0 {
		beginWorktreeRemove(p)
	}
	return true, nil
}

// beginWorktreeRemove asks for confirmation, or refuses outright the two trees
// a removal cannot touch: the one this session lives in, and one another
// session holds. Both are refused here rather than left to Manager.Remove so
// the reason lands next to the row instead of as a raw error after a confirm.
func beginWorktreeRemove(p *slashWorktreePanel) {
	e := p.Entries[p.Selected]
	switch {
	case e.Current:
		p.Notice = "This session is in " + e.Name + "; leave it before removing it."
	case e.Occupied:
		p.Notice = e.Name + " is in use by " + e.Holder + "."
	default:
		p.Confirming, p.Notice = true, ""
	}
}

func confirmWorktreeRemove(m *Model, p *slashWorktreePanel) {
	p.Confirming = false
	mgr := worktreeManager(m)
	if mgr == nil {
		p.Notice = "Worktrees are not available in this session."
		return
	}
	e := p.Entries[p.Selected]
	// A tree with work is removed only because the user confirmed it here; the
	// confirm prompt already spelled out what would be discarded.
	discard := e.Dirty > 0 || e.Unmerged > 0
	if err := mgr.Remove(context.Background(), e.Path, discard); err != nil {
		p.Notice = "remove failed: " + err.Error()
		return
	}
	refreshWorktreePanel(m, p)
	p.Notice = "Removed " + e.Name + "."
}

// refreshWorktreePanel re-reads the list after a removal and keeps the cursor on
// a valid row.
func refreshWorktreePanel(m *Model, p *slashWorktreePanel) {
	mgr := worktreeManager(m)
	if mgr == nil {
		return
	}
	entries, err := mgr.List(context.Background())
	if err != nil {
		p.Err = err.Error()
		return
	}
	p.Entries = entries
	if p.Selected >= len(entries) {
		p.Selected = len(entries) - 1
	}
	if p.Selected < 0 {
		p.Selected = 0
	}
	scrollWorktreeIntoView(m, p)
}

// worktreeRowBudget is how many rows fit; the confirm prompt costs two extra
// lines, so it narrows the window while a removal is pending.
func (m *Model) worktreeRowBudget(p *slashWorktreePanel) int {
	chrome := slashWorktreePanelChromeLines
	if p != nil && p.Confirming {
		chrome += 2
	}
	return m.panelListBudget(slashWorktreeInlinePanelMaxLines, chrome)
}

func scrollWorktreeIntoView(m *Model, p *slashWorktreePanel) {
	rows := m.worktreeRowBudget(p)
	if p.Selected < p.Offset {
		p.Offset = p.Selected
	} else if p.Selected >= p.Offset+rows {
		p.Offset = p.Selected - rows + 1
	}
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

	end := p.Offset + m.worktreeRowBudget(p)
	if end > len(p.Entries) {
		end = len(p.Entries)
	}
	for i := p.Offset; i < end; i++ {
		cursor := "  "
		if i == p.Selected {
			cursor = "› "
		}
		e := p.Entries[i]
		// Tags before the path so a narrow terminal truncates the path, not the
		// occupancy and dirty state that a cleanup decision turns on.
		line := cursor + e.Name + "  " + worktreeTags(e) + "  " + e.Path
		b.WriteString(truncateRunes(line, maxLineWidth))
		b.WriteByte('\n')
	}
	if remaining := len(p.Entries) - end; remaining > 0 {
		fmt.Fprintf(&b, "  … %d more\n", remaining)
	}

	out := strings.TrimRight(b.String(), "\n")
	if p.Notice != "" {
		out += "\n\n" + truncateRunes(p.Notice, maxLineWidth)
	}
	if p.Confirming {
		return out + "\n\n" + worktreeConfirmPrompt(p.Entries[p.Selected])
	}
	return out + "\n\n↑↓ select · d: remove · esc: close"
}

// worktreeTags renders the occupancy and the uncommitted state a removal
// decision reads, in that order.
func worktreeTags(e worktree.Info) string {
	var tags []string
	switch {
	case e.Current:
		tags = append(tags, "[this session]")
	case e.Occupied:
		tags = append(tags, "[in use by "+e.Holder+"]")
	}
	return strings.Join(append(tags, worktreeStateTag(e)), " ")
}

// worktreeStateTag names what the tree would lose if removed, or "[clean]" when
// nothing would.
func worktreeStateTag(e worktree.Info) string {
	var parts []string
	if e.Dirty > 0 {
		parts = append(parts, fmt.Sprintf("%d uncommitted", e.Dirty))
	}
	if e.Unmerged > 0 {
		parts = append(parts, fmt.Sprintf("%d unmerged", e.Unmerged))
	}
	if len(parts) == 0 {
		return "[clean]"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// worktreeConfirmPrompt spells out what a removal discards, so confirming a
// dirty tree is a deliberate choice and not a reflex on a clean-looking list.
func worktreeConfirmPrompt(e worktree.Info) string {
	if e.Dirty == 0 && e.Unmerged == 0 {
		return "Remove " + e.Name + " and its branch?  enter: remove · esc: cancel"
	}
	var parts []string
	if e.Dirty > 0 {
		parts = append(parts, fmt.Sprintf("%d uncommitted file(s)", e.Dirty))
	}
	if e.Unmerged > 0 {
		parts = append(parts, fmt.Sprintf("%d commit(s) no other branch reaches", e.Unmerged))
	}
	return "Remove " + e.Name + "? It holds " + strings.Join(parts, " and ") +
		" that will be lost permanently.  enter: discard & remove · esc: cancel"
}
