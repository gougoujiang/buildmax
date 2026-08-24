package session

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// interruptedTurn builds a turn that requested one tool call and stopped at the
// given point, so each classification in §7.3 can be produced by truncation
// rather than by hand-built records.
func interruptedTurn(stopAfter int) []Item {
	payloads := []Payload{
		TurnStarted{RunID: "run1"},
		MessageItem{Message: llm.Message{Role: "user", Content: "go"}},
		MessageItem{Message: llm.Message{
			Role:      "assistant",
			ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "Bash"}},
		}},
		ToolExecutionStarted{ToolCallID: "call_1", ToolName: "Bash"},
		ToolResult{ToolCallID: "call_1", Status: ToolStatusCompleted, Content: "done"},
		TurnFinished{Status: TurnCompleted},
	}
	return journal(payloads[:stopAfter]...)
}

func TestAnalyzeClassifiesByHowFarTheTurnGot(t *testing.T) {
	cases := []struct {
		name       string
		stopAfter  int
		needed     bool
		uncertain  []string
		notStarted []string
	}{
		{name: "assistant call only", stopAfter: 3, needed: true, notStarted: []string{"call_1"}},
		{name: "entered the tool", stopAfter: 4, needed: true, uncertain: []string{"call_1"}},
		{name: "tool returned", stopAfter: 5, needed: true},
		{name: "turn closed", stopAfter: 6, needed: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := interruptedTurn(tc.stopAfter)
			rec, err := Analyze(items, mustHead(t, items))
			if err != nil {
				t.Fatalf("Analyze: %v", err)
			}
			if rec.Needed() != tc.needed {
				t.Fatalf("Needed = %v, want %v", rec.Needed(), tc.needed)
			}
			if got := callIDs(rec.Uncertain); !equalStrings(got, tc.uncertain) {
				t.Errorf("uncertain = %v, want %v", got, tc.uncertain)
			}
			if got := callIDs(rec.NotStarted); !equalStrings(got, tc.notStarted) {
				t.Errorf("not started = %v, want %v", got, tc.notStarted)
			}
		})
	}
}

func TestAnalyzeReportsNothingForAStoppedTurn(t *testing.T) {
	// A turn closed by a live process said what it had done. Re-deriving that
	// judgement could only contradict it, so recovery leaves it alone even
	// though the stop was not a normal completion.
	for _, status := range []string{TurnCanceled, TurnInterrupted, TurnFailed} {
		items := journal(
			TurnStarted{RunID: "run1"},
			MessageItem{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "Bash"}}}},
			ToolExecutionStarted{ToolCallID: "call_1", ToolName: "Bash"},
			ToolResult{ToolCallID: "call_1", Status: ToolStatusUnknown},
			TurnFinished{Status: status},
		)
		rec, err := Analyze(items, mustHead(t, items))
		if err != nil {
			t.Fatalf("%s: Analyze: %v", status, err)
		}
		if rec.Needed() {
			t.Errorf("%s: turn reported as needing recovery", status)
		}
	}
}

func TestAnalyzeOnlyLooksAtTheOpenTurn(t *testing.T) {
	// An earlier turn's calls were resolved when that turn closed; carrying
	// them forward would repair the same call on every later interruption.
	items := journal(
		TurnStarted{RunID: "run1"},
		MessageItem{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "old_call", Name: "Bash"}}}},
		ToolExecutionStarted{ToolCallID: "old_call", ToolName: "Bash"},
		ToolResult{ToolCallID: "old_call", Status: ToolStatusCompleted},
		TurnFinished{Status: TurnCompleted},
		TurnStarted{RunID: "run2"},
		MessageItem{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "new_call", Name: "Write"}}}},
		ToolExecutionStarted{ToolCallID: "new_call", ToolName: "Write"},
	)
	rec, err := Analyze(items, mustHead(t, items))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if rec.TurnID != "run2" {
		t.Fatalf("turn = %q, want run2", rec.TurnID)
	}
	if got := callIDs(rec.Uncertain); !equalStrings(got, []string{"new_call"}) {
		t.Errorf("uncertain = %v, want [new_call]", got)
	}
}

func TestAnalyzeTreatsAnOrphanedExecutionAsUncertain(t *testing.T) {
	// The assistant message is committed before the tool is entered, so an
	// execution record with no matching call means a torn journal. The tool was
	// entered either way, which is the fact that matters.
	items := journal(
		TurnStarted{RunID: "run1"},
		ToolExecutionStarted{ToolCallID: "orphan", ToolName: "Bash"},
	)
	rec, err := Analyze(items, mustHead(t, items))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got := callIDs(rec.Uncertain); !equalStrings(got, []string{"orphan"}) {
		t.Fatalf("uncertain = %v, want [orphan]", got)
	}
}

func TestAnalyzeIgnoresAnAbandonedBranch(t *testing.T) {
	// A tool left in flight on a branch the user rewound away from is not the
	// live branch's problem to repair.
	items := journal(
		TurnStarted{RunID: "run1"},
		MessageItem{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1", Name: "Bash"}}}},
		ToolExecutionStarted{ToolCallID: "call_1", ToolName: "Bash"},
	)
	items = append(items,
		NewItem(4, "id", "ia", testTime, "run2", HeadSelected{Reason: "user_rewind"}),
		NewItem(5, "ie", "id", testTime, "run2", TurnFinished{Status: TurnCompleted}),
	)
	rec, err := Analyze(items, mustHead(t, items))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if rec.Needed() {
		t.Errorf("recovery reported for a branch that was rewound away: %+v", rec)
	}
}

func callIDs(calls []ToolCallOutcome) []string {
	if len(calls) == 0 {
		return nil
	}
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.ToolCallID)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
