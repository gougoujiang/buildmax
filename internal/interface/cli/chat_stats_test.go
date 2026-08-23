package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/agentapp"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
	"github.com/gougoujiang/buildmax/internal/infra/trace"

	tea "charm.land/bubbletea/v2"
)

// statsPanelModel is a sized model with a session open, which is what the
// panel's width- and height-driven trimming reads.
func statsPanelModel(t *testing.T) *Model {
	t.Helper()
	m := NewModel(TUIOpts{Session: testSessionContext(), Workspace: t.TempDir()})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return next.(*Model)
}

func renderStatsPanel(t *testing.T, s agentapp.SessionStats) string {
	t.Helper()
	m := statsPanelModel(t)
	p := &slashStatsPanel{Stats: s}
	return p.Render(m, m.panelContentWidth())
}

func TestSlashStats_IsRegisteredForCompletionAndDispatch(t *testing.T) {
	if !slices.Contains(builtinSlashCommands, "/stats") {
		t.Fatal("/stats is missing from the completion list")
	}
	if !slices.IsSorted(builtinSlashCommands) {
		t.Errorf("builtinSlashCommands is not sorted: %v", builtinSlashCommands)
	}
	m := statsPanelModel(t)
	opened, _ := dispatchSlashCommand(m, "/stats")
	mod := opened.(*Model)
	if mod.activePanel == nil {
		t.Fatal("/stats opened no panel")
	}
	if mod.err != "" {
		t.Errorf("dispatching /stats set err = %q, want none", mod.err)
	}
}

// The panel folds the live session, so a run with no session open must say so
// rather than render an empty report that reads as a session that did nothing.
func TestSlashStats_NoSessionSaysSo(t *testing.T) {
	m := NewModel(TUIOpts{Workspace: t.TempDir()})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	mod := next.(*Model)
	opened, _ := openSlashStats(mod)
	mod = opened.(*Model)
	if mod.activePanel == nil {
		t.Fatal("openSlashStats opened no panel")
	}
	out := mod.activePanel.Render(mod, mod.panelContentWidth())
	if !strings.Contains(out, "no session is open") {
		t.Errorf("panel does not report the missing session:\n%s", out)
	}
}

// Same rule as the command: a provider reporting no cache usage has not
// reported a miss, so no cache line is printed at all.
func TestSlashStats_UnreportedCacheIsNotPrintedAsZero(t *testing.T) {
	out := renderStatsPanel(t, agentapp.SessionStats{
		ID:    "s1",
		Usage: cllm.Usage{PromptTokens: 1000, CompletionTokens: 50},
	})
	if strings.Contains(out, "Cache") {
		t.Errorf("panel claims a cache result nobody measured:\n%s", out)
	}
	if !strings.Contains(out, "not priced") {
		t.Errorf("panel does not say the session was never priced:\n%s", out)
	}
}

func TestSlashStats_MissingTraceSaysUnavailableNotZero(t *testing.T) {
	out := renderStatsPanel(t, agentapp.SessionStats{
		ID:           "s1",
		Conversation: session.ConversationStats{UserMessages: 1, ToolCalls: 3},
	})
	if !strings.Contains(out, "timings are unavailable") {
		t.Errorf("panel does not say the timings are missing:\n%s", out)
	}
	if strings.Contains(out, "waiting") {
		t.Errorf("panel reports a wall clock it never measured:\n%s", out)
	}
}

// The warnings are shared with the command deliberately: one that appeared on
// only one surface would be a warning nobody trusted.
func TestSlashStats_CarriesTheSameCaveatsAsTheCommand(t *testing.T) {
	s := agentapp.SessionStats{
		ID:             "s1",
		CostIncomplete: true,
		Runs:           trace.SessionSummary{Runs: 2, Incomplete: 1},
	}
	panel := renderStatsPanel(t, s)
	notes := statsCaveats(s)
	if len(notes) == 0 {
		t.Fatal("the fixture produced no caveats, so the test asserts nothing")
	}
	for _, want := range notes {
		// Truncated to the panel width, so match on a distinctive head.
		head := string([]rune(want)[:40])
		if !strings.Contains(panel, head) {
			t.Errorf("panel is missing the caveat %q:\n%s", want, panel)
		}
	}
}

// Parallel tool execution can push summed tool time past the wall clock.
func TestSlashStats_OverlappingToolTimeDoesNotSubtract(t *testing.T) {
	out := renderStatsPanel(t, agentapp.SessionStats{
		ID:   "s1",
		Runs: trace.SessionSummary{Runs: 1, Wall: 10_000_000_000, ToolWall: 25_000_000_000},
	})
	if !strings.Contains(out, "overlapping") {
		t.Errorf("panel does not explain why the split does not divide:\n%s", out)
	}
	if strings.Contains(out, "model /") {
		t.Errorf("panel split a wall clock that tool time exceeds:\n%s", out)
	}
}

// A trimmed tool list must say it was trimmed: a short list is otherwise
// indistinguishable from a session that used four tools.
func TestSlashStats_TrimmedToolListSaysSo(t *testing.T) {
	var tools []session.ToolStats
	for i := range 40 {
		tools = append(tools, session.ToolStats{
			Name: strings.Repeat("t", i%9+3), Calls: 1, ResultBytes: 100 - i,
		})
	}
	out := renderStatsPanel(t, agentapp.SessionStats{
		ID:           "s1",
		Conversation: session.ConversationStats{UserMessages: 3, ToolCalls: 40, Tools: tools},
		Runs:         trace.SessionSummary{Runs: 1, Wall: 5_000_000_000},
	})
	if !strings.Contains(out, "more") {
		t.Errorf("panel trimmed the tool list without saying so:\n%s", out)
	}
}
