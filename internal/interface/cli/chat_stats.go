package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/util"

	tea "charm.land/bubbletea/v2"
)

// slashStatsToolRows caps the tool table in the panel. `buildmax stats` prints
// a longer one and --json prints all of them; an overlay is a glance.
const slashStatsToolRows = 5

// slashStatsChromeLines are the panel's own lines: box border, title, the
// summary block above the table, its header, and the key hint.
const slashStatsChromeLines = 18

// slashStatsPanel implements slashPanel for the /stats overlay.
//
// The statistics are folded once, when the panel opens, rather than on every
// render: a render runs on each keystroke and each frame of the spinner, and
// the fold reads every trace file the session has.
type slashStatsPanel struct {
	Stats     agentapp.SessionStats
	LoadError string
}

func openSlashStats(m *Model) (tea.Model, tea.Cmd) {
	p := &slashStatsPanel{}
	if m.opts.Session == nil || m.opts.Session.Session == nil {
		p.LoadError = "no session is open"
		return m.openPanel(p)
	}
	// The live session, not the file: a session is persisted after each
	// assistant reply, so reading it back would answer about the turn before
	// the one on screen.
	stats, err := agentapp.NewSessionStats(m.opts.Session.Session, m.opts.Workspace, config.TracesDir())
	if err != nil {
		p.LoadError = err.Error()
	}
	p.Stats = stats
	return m.openPanel(p)
}

func (p *slashStatsPanel) HandleKey(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	if msg.Code == tea.KeyEscape {
		_, cmd := m.closeActivePanel()
		return true, cmd
	}
	return false, nil
}

func (p *slashStatsPanel) FooterHint() string { return "esc: close stats panel" }

func (p *slashStatsPanel) OnClose(_ *Model) {}

func (p *slashStatsPanel) Render(m *Model, maxLineWidth int) string {
	var b strings.Builder
	b.WriteString(slashPanelTitleStyle.Render("Session stats"))
	b.WriteString("\n\n")
	if p.LoadError != "" {
		b.WriteString(truncateRunes(p.LoadError, maxLineWidth))
		return strings.TrimRight(b.String(), "\n") + "\n\nesc: close"
	}

	b.WriteString(renderStatsSummary(p.Stats, maxLineWidth))
	budget := m.panelListBudget(slashStatsToolRows, slashStatsChromeLines)
	if table := renderStatsToolTable(p.Stats, budget, maxLineWidth); table != "" {
		b.WriteString("\n")
		b.WriteString(table)
	}
	for _, note := range statsCaveats(p.Stats) {
		b.WriteString("\n! " + truncateRunes(note, maxLineWidth-2))
	}
	return strings.TrimRight(b.String(), "\n") + "\n\nesc: close · buildmax stats for the full record"
}

// renderStatsSummary is the panel's condensed report. It is a separate
// rendering from the command's, not a narrowed copy of it: a boxed overlay a
// few dozen columns wide and a full terminal want different layouts, and the
// shared thing worth sharing is the data and the rules about what may be said
// of it.
func renderStatsSummary(s agentapp.SessionStats, maxLineWidth int) string {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)

	row := func(label, value string) {
		fmt.Fprintf(tw, "%s\t%s\n", label, truncateRunes(value, maxLineWidth-12))
	}

	spend := fmt.Sprintf("%s in / %s out",
		formatCount(s.Usage.PromptTokens), formatCount(s.Usage.CompletionTokens))
	if s.Cost != nil {
		spend += " · " + cllm.FormatAmount(s.Cost.Total) + " " + s.Cost.Currency
	} else {
		spend += " · not priced"
	}
	row("Spend", spend)

	// Only where a provider reported cache usage: a "0 / 0" line on a provider
	// that reports nothing would claim a miss nobody measured.
	if s.Usage.CacheReadTokens > 0 || s.Usage.CacheWriteTokens > 0 {
		cache := fmt.Sprintf("%s read / %s write",
			formatCount(s.Usage.CacheReadTokens), formatCount(s.Usage.CacheWriteTokens))
		if s.Usage.PromptTokens > 0 {
			cache += fmt.Sprintf(" · %.0f%% of prompt",
				float64(s.Usage.CacheReadTokens)/float64(s.Usage.PromptTokens)*100)
		}
		if saved, ok := s.CacheSaved(); ok {
			cache += " · saved " + cllm.FormatAmount(saved)
		} else if s.Cost != nil && s.Cost.Baseline > 0 {
			// A run that only ever wrote cache entries paid more than it would
			// have uncached, and calling that a small win would be a lie.
			cache += " · cost more than uncached"
		}
		row("Cache", cache)
	}

	if d := s.Runs.Delegated; d != nil && d.Runs > 0 {
		line := fmt.Sprintf("%s · %s in / %s out",
			countLabel(d.Runs, "run"), formatCount(d.PromptTokens), formatCount(d.CompletionTokens))
		if d.Cost != nil {
			line += " · " + cllm.FormatAmount(d.Cost.Total) + " " + d.Cost.Currency
		}
		row("Delegated", line)
	}

	ctx := "peak not recorded"
	if share, ok := s.ContextPeakShare(); ok {
		ctx = fmt.Sprintf("peak %s of %s (%.0f%%)",
			formatCount(s.Runs.PeakContextTokens), formatCount(s.Runs.ContextWindow), share*100)
	}
	ctx += " · " + countLabel(s.Runs.Compactions, "compaction")
	row("Context", ctx)

	c := s.Conversation
	row("History", fmt.Sprintf("%s text / %s tool output",
		formatBytes(c.TextBytes), formatBytes(c.ToolResultBytes)))

	work := fmt.Sprintf("%s · %s · %s",
		countLabel(c.UserMessages, "message"), countLabel(c.AssistantTurns, "turn"),
		countLabel(c.ToolCalls, "tool call"))
	if s.Runs.ToolFailures > 0 {
		work += fmt.Sprintf(" · %d could not complete", s.Runs.ToolFailures)
	}
	if s.Runs.ToolDenials > 0 {
		work += fmt.Sprintf(" · %d denied", s.Runs.ToolDenials)
	}
	row("Work", work)

	if s.Runs.Runs == 0 {
		row("Time", "no trace recorded, so timings are unavailable")
	} else {
		time := util.FormatDuration(s.Runs.Wall) + " waiting"
		if model, ok := s.ModelTime(); ok {
			time += fmt.Sprintf(" · %s model / %s tools",
				util.FormatDuration(model), util.FormatDuration(s.Runs.ToolWall))
		} else if s.Runs.ToolWall > 0 {
			// Parallel tool execution can push summed tool time past the wall
			// clock; subtracting anyway would print a negative model time.
			time += fmt.Sprintf(" · %s in tools, overlapping", util.FormatDuration(s.Runs.ToolWall))
		}
		if s.Runs.Subagents > 0 {
			time += " · " + countLabel(s.Runs.Subagents, "subagent run")
		}
		row("Time", time)
	}
	_ = tw.Flush()
	return trimRowPadding(b.String())
}

func renderStatsToolTable(s agentapp.SessionStats, budget, maxLineWidth int) string {
	tools := mergeToolStats(s)
	if len(tools) == 0 || budget <= 0 {
		return ""
	}
	shown := tools
	if len(shown) > budget {
		shown = shown[:budget]
	}
	var b strings.Builder
	b.WriteString("Tools, heaviest first\n")
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for _, t := range shown {
		name := t.Name
		if name == "" {
			name = "(unattributed)"
		}
		output := "-"
		if t.ResultBytes > 0 {
			output = formatBytes(t.ResultBytes)
		}
		spent := "-"
		if t.Wall > 0 {
			spent = util.FormatDuration(t.Wall)
		}
		fmt.Fprintf(tw, "  %s\t%d\t%s\t%s\t%s\n",
			truncateRunes(name, maxLineWidth/3), t.Calls, output, spent, t.Note)
	}
	_ = tw.Flush()
	out := trimRowPadding(b.String())
	if len(tools) > len(shown) {
		out += fmt.Sprintf("  … %d more\n", len(tools)-len(shown))
	}
	return out
}
