package agent

import (
	"context"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// delegatingTool stands in for the Task tool: it reports a finished child run
// to whatever accumulator the parent installed, exactly as the real subagent
// runner does.
type delegatingTool struct {
	stats RunStats
	seen  bool
}

func (t *delegatingTool) Name() string        { return "delegate" }
func (t *delegatingTool) Description() string { return "delegate work" }
func (t *delegatingTool) Parameters() any     { return map[string]any{} }

func (t *delegatingTool) Execute(ctx context.Context, _ map[string]any) (string, error) {
	u := DelegatedUsageFromCtx(ctx)
	t.seen = u != nil
	u.Report(t.stats)
	return "child done", nil
}

func testPricing() llm.Pricing {
	return llm.Pricing{
		Currency:          "USD",
		InputPerMTok:      1_000_000,
		CacheReadPerMTok:  100_000,
		CacheWritePerMTok: 1_250_000,
		OutputPerMTok:     5_000_000,
	}
}

// A delegation's spend has to reach the run that paid for it. Before the
// roll-up the subagent runner discarded the child's RunStats, so every Task
// call was free as far as the session file, the footer, and the trace knew.
func TestRunLoop_DelegatedSpendReachesTheParent(t *testing.T) {
	childCost, ok := llm.EstimateCost(llm.Usage{PromptTokens: 900, CompletionTokens: 100}, testPricing())
	if !ok {
		t.Fatal("EstimateCost: child run went unpriced, so the test cannot assert money")
	}
	tool := &delegatingTool{stats: RunStats{
		ToolCalls:        3,
		PromptTokens:     900,
		CompletionTokens: 100,
		CacheReadTokens:  400,
		Cost:             &childCost,
	}}

	client := &mockLLMClient{responses: []mockResponse{
		{
			toolCalls: []llm.ToolCall{{ID: "call_1", Name: "delegate", Arguments: "{}"}},
			usage:     llm.Usage{PromptTokens: 100, CompletionTokens: 10},
		},
		{content: "done", usage: llm.Usage{PromptTokens: 120, CompletionTokens: 20}},
	}}

	_, stats, err := runLoopWithUserMsg(context.Background(), client,
		newTestToolRegistry(tool), &testBuffer{}, "go",
		func(o *RunLoopOpts) { o.Pricing = testPricing() })
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}
	if !tool.seen {
		t.Fatal("no accumulator on the tool's context: a delegation had nowhere to report")
	}

	// Totals are inclusive: what the run cost is what it cost, whoever ran the
	// calls. 100+120 of the parent's own prompt tokens plus the child's 900.
	if want := 1120; stats.PromptTokens != want {
		t.Errorf("PromptTokens = %d, want %d (parent 220 + delegated 900)", stats.PromptTokens, want)
	}
	if want := 130; stats.CompletionTokens != want {
		t.Errorf("CompletionTokens = %d, want %d", stats.CompletionTokens, want)
	}
	if want := 400; stats.CacheReadTokens != want {
		t.Errorf("CacheReadTokens = %d, want %d", stats.CacheReadTokens, want)
	}
	// One delegation is one tool call of the parent, and the child's own three
	// are counted nowhere else.
	if want := 1; stats.ToolCalls != want {
		t.Errorf("ToolCalls = %d, want %d — the parent made one call", stats.ToolCalls, want)
	}

	if stats.Delegated == nil {
		t.Fatal("Delegated is nil: the totals include the delegation but nothing says how much of them it is")
	}
	if stats.Delegated.Runs != 1 {
		t.Errorf("Delegated.Runs = %d, want 1", stats.Delegated.Runs)
	}
	if stats.Delegated.PromptTokens != 900 {
		t.Errorf("Delegated.PromptTokens = %d, want 900", stats.Delegated.PromptTokens)
	}
	if stats.Delegated.ToolCalls != 3 {
		t.Errorf("Delegated.ToolCalls = %d, want 3", stats.Delegated.ToolCalls)
	}
	if stats.Cost == nil {
		t.Fatal("Cost is nil on a priced run")
	}
	if stats.Cost.Total <= childCost.Total {
		t.Errorf("Cost.Total = %d, want more than the child's %d alone", stats.Cost.Total, childCost.Total)
	}
}

// A run that delegated nothing must not grow a breakdown of nothing: an empty
// Delegated block reads as "it delegated and spent zero", which is a different
// claim from "it never delegated".
func TestRunLoop_NoDelegationLeavesNoBreakdown(t *testing.T) {
	client := &mockLLMClient{responses: []mockResponse{
		{content: "done", usage: llm.Usage{PromptTokens: 10, CompletionTokens: 2}},
	}}
	_, stats, err := runLoopWithUserMsg(context.Background(), client,
		newTestToolRegistry(), &testBuffer{}, "go")
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}
	if stats.Delegated != nil {
		t.Errorf("Delegated = %+v, want nil on a run that delegated nothing", stats.Delegated)
	}
}

// An unpriced delegation must mark the total partial rather than be absorbed
// as free. Same rule the session totals already follow.
func TestDelegatedUsage_UnpricedChildMarksTotalIncomplete(t *testing.T) {
	var u DelegatedUsage
	u.Report(RunStats{PromptTokens: 500, CostIncomplete: true})

	var s RunStats
	s.absorb(u.Drain())
	if !s.CostIncomplete {
		t.Error("CostIncomplete = false after absorbing an unpriced delegation, want true")
	}
	if s.Delegated == nil || !s.Delegated.CostIncomplete {
		t.Error("the breakdown does not carry the same gap the total does")
	}
}

// Drain returns the delta, so a loop that folds every iteration does not count
// an earlier delegation again on the next one.
func TestDelegatedUsage_DrainDoesNotRepeat(t *testing.T) {
	var u DelegatedUsage
	u.Report(RunStats{PromptTokens: 100})

	var s RunStats
	s.absorb(u.Drain())
	s.absorb(u.Drain())
	if s.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d after two drains of one delegation, want 100", s.PromptTokens)
	}
	if s.Delegated.Runs != 1 {
		t.Errorf("Delegated.Runs = %d, want 1", s.Delegated.Runs)
	}
}
