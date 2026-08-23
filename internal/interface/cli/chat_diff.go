package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gougoujiang/buildmax/internal/infra/git"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const slashDiffPanelMaxLines = 22

// slashDiffPanelChromeLines are the panel's own lines: box border, title, the
// blank line above the panes, and the key hints.
const slashDiffPanelChromeLines = 6

type slashDiffState struct {
	Diff            git.WorkspaceDiff
	LoadError       string
	Selected        int
	Focus           diffPaneFocus
	ListViewport    viewport.Model
	ContentViewport viewport.Model
}

type diffPaneFocus int

const (
	diffFocusList diffPaneFocus = iota
	diffFocusContent
)

func openSlashDiff(m *Model) (tea.Model, tea.Cmd) {
	st := &slashDiffState{}
	if m.opts.App == nil {
		st.LoadError = "agent app is not initialized"
		m.slashDiff = st
		return m.openPanel(st)
	}
	diff, err := git.ReadWorkspace(context.Background(), m.opts.App.WorkspaceRoot())
	if err != nil {
		st.LoadError = err.Error()
	} else {
		st.Diff = diff
	}
	st.Focus = diffFocusList
	m.slashDiff = st
	// Activate panel first so width-driven sync runs against the right model.
	model, cmd := m.openPanel(st)
	m.syncDiffViewports()
	return model, cmd
}

func (p *slashDiffState) FooterHint() string { return "esc: close diff panel" }

func (p *slashDiffState) OnClose(m *Model) { m.slashDiff = nil }

func (p *slashDiffState) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	switch msg.Code {
	case tea.KeyEscape:
		_, cmd := m.closeActivePanel()
		return true, cmd
	case tea.KeyLeft:
		p.Focus = diffFocusList
		return true, nil
	case tea.KeyRight:
		p.Focus = diffFocusContent
		return true, nil
	case tea.KeyUp:
		if p.Focus == diffFocusList {
			if p.Selected > 0 {
				p.Selected--
				p.ContentViewport.GotoTop()
				m.syncDiffViewports()
			}
		} else {
			p.ContentViewport.ScrollUp(1)
		}
		return true, nil
	case tea.KeyDown:
		if p.Focus == diffFocusList {
			if p.Selected < len(p.Diff.Files)-1 {
				p.Selected++
				p.ContentViewport.GotoTop()
				m.syncDiffViewports()
			}
		} else {
			p.ContentViewport.ScrollDown(1)
		}
		return true, nil
	case tea.KeyPgUp:
		if p.Focus == diffFocusList {
			p.ListViewport.PageUp()
		} else {
			p.ContentViewport.PageUp()
		}
		return true, nil
	case tea.KeyPgDown:
		if p.Focus == diffFocusList {
			p.ListViewport.PageDown()
		} else {
			p.ContentViewport.PageDown()
		}
		return true, nil
	}
	return false, nil
}

func (p *slashDiffState) Render(m *Model, maxWidth int) string {
	var b strings.Builder
	title := "Diff"
	if p.Diff.Workspace != "" {
		title += " — " + p.Diff.Workspace
	}
	b.WriteString(slashPanelTitleStyle.Render(truncateRunes(title, maxWidth)))
	b.WriteByte('\n')
	if p.LoadError != "" {
		b.WriteString("\n" + p.LoadError)
		return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
	}
	if p.Diff.Error != "" {
		b.WriteString("\n" + p.Diff.Error)
		return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
	}
	if len(p.Diff.Files) == 0 {
		b.WriteString("\nNo changed files.")
		return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
	}

	bodyH := m.diffBodyHeight()
	leftW, rightW := diffPaneWidths(maxWidth)
	leftStyle := diffPaneBorderStyle.Width(leftW).Height(bodyH)
	rightStyle := lipgloss.NewStyle().Width(rightW).Height(bodyH)
	if p.Focus == diffFocusList {
		leftStyle = leftStyle.Inherit(diffFocusedStyle)
	} else {
		rightStyle = rightStyle.Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lightSkyBlue).
			PaddingLeft(1)
	}
	// A pane keeps its border and padding inside the width it was given, so the
	// viewport gets what is left. A viewport as wide as its pane wraps every
	// line, and the pane grows past the height the panel budgeted for.
	m.syncDiffViewportsWithSize(leftW-leftStyle.GetHorizontalFrameSize(), rightW-rightStyle.GetHorizontalFrameSize(), bodyH)
	left := leftStyle.Render(p.ListViewport.View())
	right := rightStyle.Render(p.ContentViewport.View())
	b.WriteByte('\n')
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, right))
	b.WriteString("\n\nesc: close · ←→ pane · ↑↓/pgup/pgdn scroll")
	return strings.TrimRight(b.String(), "\n")
}

func diffPaneWidths(boxW int) (leftW, rightW int) {
	leftW = max(28, min(50, boxW*2/5))
	rightW = boxW - leftW - 3
	if rightW < 24 {
		rightW = 24
	}
	return leftW, rightW
}

func (m *Model) buildDiffFileList(width int) string {
	st := m.slashDiff
	if st == nil || len(st.Diff.Files) == 0 {
		return ""
	}
	var b strings.Builder
	for i, file := range st.Diff.Files {
		if i > 0 {
			b.WriteByte('\n')
		}
		pathWidth := max(1, width-5)
		line := statusGlyph(file.Status) + " " + truncateMiddleRunes(file.Path, pathWidth)
		if i == st.Selected {
			b.WriteString(diffSelectedStyle.Render("› " + line))
		} else {
			b.WriteString(diffPathStyle.Render("  " + line))
		}
	}
	return b.String()
}

func (m *Model) buildDiffPreview(maxWidth int) string {
	st := m.slashDiff
	if st.Selected < 0 || st.Selected >= len(st.Diff.Files) {
		return ""
	}
	file := st.Diff.Files[st.Selected]
	var lines []string
	lines = append(lines, diffSelectedStyle.Render(clipRunes(displayDiffPath(file), maxWidth)))
	if file.Binary {
		lines = append(lines, diffMetaStyle.Render("Binary file changed."))
	} else if strings.TrimSpace(file.Patch) == "" {
		lines = append(lines, diffMetaStyle.Render("No text diff available."))
	} else {
		lines = append(lines, formatPatchLines(file.Patch, maxWidth)...)
	}
	if file.Truncated {
		lines = append(lines, diffMetaStyle.Render("Diff truncated."))
	}
	return strings.Join(lines, "\n")
}

func statusGlyph(status git.ChangeStatus) string {
	switch status {
	case git.StatusAdded:
		return diffAddStyle.Render("+")
	case git.StatusDeleted:
		return diffDelStyle.Render("-")
	case git.StatusRenamed:
		return diffRenStyle.Render("↔")
	case git.StatusModified:
		return diffModStyle.Render("●")
	default:
		return diffModStyle.Render("●")
	}
}

func displayDiffPath(f git.ChangedFile) string {
	if f.Status == git.StatusRenamed && f.OldPath != "" {
		return f.OldPath + " -> " + f.Path
	}
	return f.Path
}

// diffLineNumWidth is the width of the (single) line-number column in the
// patch view.
const diffLineNumWidth = 4

// diffTabExpansion is the number of spaces each leading/inline tab expands to
// before width measurement. Without this, clipRunes() measures \t as 1 rune
// while the terminal renders it as several visible columns, causing long
// diff lines to overflow rightW and visually wrap into the left pane.
const diffTabExpansion = 4

func formatPatchLines(patch string, maxWidth int) []string {
	var out []string
	oldLine, newLine := 0, 0
	// Budget for line text after the "NUM │ " prefix.
	textBudget := maxWidth - diffLineNumWidth - 3
	if textBudget < 8 {
		textBudget = 8
	}
	tabSpaces := strings.Repeat(" ", diffTabExpansion)
	for _, raw := range strings.Split(strings.ReplaceAll(patch, "\r\n", "\n"), "\n") {
		raw = strings.ReplaceAll(raw, "\t", tabSpaces)
		if strings.HasPrefix(raw, "@@") {
			oldLine, newLine = parseHunkStart(raw)
			out = append(out, diffHunkStyle.Render(clipRunes(raw, maxWidth)))
			continue
		}
		if strings.HasPrefix(raw, "diff --git") || strings.HasPrefix(raw, "index ") ||
			strings.HasPrefix(raw, "--- ") || strings.HasPrefix(raw, "+++ ") {
			out = append(out, diffHeaderStyle.Render(clipRunes(raw, maxWidth)))
			continue
		}
		var num string
		style := diffPathStyle
		switch {
		case strings.HasPrefix(raw, "+"):
			num = strconv.Itoa(newLine)
			newLine++
			style = diffAddStyle
		case strings.HasPrefix(raw, "-"):
			num = strconv.Itoa(oldLine)
			oldLine++
			style = diffDelStyle
		default:
			if newLine > 0 {
				num = strconv.Itoa(newLine)
				newLine++
			}
			if oldLine > 0 {
				oldLine++
			}
		}
		line := fmt.Sprintf("%*s │ %s", diffLineNumWidth, num, clipRunes(raw, textBudget))
		out = append(out, style.Render(line))
	}
	return out
}

// clipRunes returns s truncated to at most max display runes (no ellipsis,
// since these are diff lines and we want them to look "cut" rather than
// modified).
func clipRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func parseHunkStart(line string) (oldStart, newStart int) {
	fields := strings.Fields(line)
	for _, f := range fields {
		if strings.HasPrefix(f, "-") {
			oldStart = parseRangeStart(f[1:])
		}
		if strings.HasPrefix(f, "+") {
			newStart = parseRangeStart(f[1:])
		}
	}
	return oldStart, newStart
}

func parseRangeStart(s string) int {
	if idx := strings.IndexByte(s, ','); idx >= 0 {
		s = s[:idx]
	}
	n, _ := strconv.Atoi(s)
	return n
}

func (m *Model) syncDiffViewports() {
	if m == nil || m.slashDiff == nil {
		return
	}
	bodyH := m.diffBodyHeight()
	leftW, rightW := diffPaneWidths(max(48, m.panelContentWidth()))
	// Rendering re-syncs with the focused pane's exact frame; this keeps the
	// viewports usable for the key handling that runs between frames.
	m.syncDiffViewportsWithSize(leftW-diffPaneBorderStyle.GetHorizontalFrameSize(), rightW, bodyH)
}

func (m *Model) syncDiffViewportsWithSize(leftW, rightW, height int) {
	st := m.slashDiff
	if st == nil {
		return
	}
	if st.ListViewport.Width() == 0 {
		st.ListViewport = viewport.New(viewport.WithWidth(leftW), viewport.WithHeight(height))
		st.ListViewport.SoftWrap = false
	} else {
		st.ListViewport.SetWidth(leftW)
		st.ListViewport.SetHeight(height)
	}
	if st.ContentViewport.Width() == 0 {
		st.ContentViewport = viewport.New(viewport.WithWidth(rightW), viewport.WithHeight(height))
		st.ContentViewport.SoftWrap = false
	} else {
		st.ContentViewport.SetWidth(rightW)
		st.ContentViewport.SetHeight(height)
	}
	st.ListViewport.SetContent(m.buildDiffFileList(leftW))
	st.ContentViewport.SetContent(m.buildDiffPreview(rightW))
	st.ListViewport.EnsureVisible(st.Selected, 0, 0)
	st.ContentViewport.FillHeight = true
	st.ListViewport.FillHeight = true
}

func truncateMiddleRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	if maxRunes <= 1 {
		return "…"
	}
	head := (maxRunes - 1) / 2
	tail := maxRunes - 1 - head
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}

func (m *Model) diffBodyHeight() int {
	return m.panelListBudget(slashDiffPanelMaxLines, slashDiffPanelChromeLines)
}
