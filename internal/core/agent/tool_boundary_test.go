package agent

import (
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// boundaryHistory records what a durable history would have written, and can
// be made to fail at either commit point.
type boundaryHistory struct {
	messages []llm.Message
	starts   [][]ToolCallStart
	outcomes []ToolOutcome

	failStart  error
	failResult error
}

func (h *boundaryHistory) HistoryMessages() []llm.Message { return h.messages }

func (h *boundaryHistory) Append(m llm.Message) error {
	h.messages = append(h.messages, m)
	return nil
}

func (h *boundaryHistory) ToolExecutionStarted(calls []ToolCallStart) error {
	if h.failStart != nil {
		return h.failStart
	}
	h.starts = append(h.starts, calls)
	return nil
}

func (h *boundaryHistory) AppendToolResult(out ToolOutcome) error {
	if h.failResult != nil {
		return h.failResult
	}
	h.outcomes = append(h.outcomes, out)
	h.messages = append(h.messages, llm.Message{
		Role: "tool", Content: out.Result, ToolCallID: out.ID, Parts: out.Parts,
	})
	return nil
}

// plainHistory implements only MessageHistory, like an in-memory caller.
type plainHistory struct{ messages []llm.Message }

func (h *plainHistory) HistoryMessages() []llm.Message { return h.messages }
func (h *plainHistory) Append(m llm.Message) error {
	h.messages = append(h.messages, m)
	return nil
}

func TestOutcomeOfClassifiesByWhatActuallyHappened(t *testing.T) {
	cases := []struct {
		name string
		call pendingCall
		want string
	}{
		{
			name: "ran and returned",
			call: pendingCall{executed: true},
			want: ToolStatusCompleted,
		},
		{
			name: "ran and failed",
			call: pendingCall{executed: true, errKind: ToolErrorFailed},
			want: ToolStatusFailed,
		},
		{
			name: "panicked",
			call: pendingCall{executed: true, errKind: ToolErrorPanic},
			want: ToolStatusFailed,
		},
		{
			// Refused by policy, unknown, or loop-guarded: nothing outside
			// BuildMax ran, which is the distinction the status carries.
			name: "never reached its tool",
			call: pendingCall{decided: true},
			want: ToolStatusDenied,
		},
		{
			name: "rejected at parse",
			call: pendingCall{decided: true, errKind: ToolErrorInvalidArgs},
			want: ToolStatusDenied,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := outcomeOf(&tc.call).Status; got != tc.want {
				t.Errorf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRecordToolBoundarySkipsCallsThatNeverRun(t *testing.T) {
	h := &boundaryHistory{}
	group := []pendingCall{
		{call: llm.ToolCall{ID: "will_run", Name: "Bash"}},
		{call: llm.ToolCall{ID: "denied", Name: "Bash"}, decided: true},
	}
	if err := recordToolBoundary(RunLoopOpts{History: h}, group); err != nil {
		t.Fatalf("recordToolBoundary: %v", err)
	}
	if len(h.starts) != 1 || len(h.starts[0]) != 1 || h.starts[0][0].ID != "will_run" {
		t.Fatalf("starts = %v, want only the call that reaches a tool", h.starts)
	}
}

func TestRecordToolBoundaryWritesNothingWhenEveryCallIsDecided(t *testing.T) {
	h := &boundaryHistory{}
	group := []pendingCall{{call: llm.ToolCall{ID: "denied"}, decided: true}}
	if err := recordToolBoundary(RunLoopOpts{History: h}, group); err != nil {
		t.Fatalf("recordToolBoundary: %v", err)
	}
	// An empty record would claim a boundary was crossed when none was.
	if len(h.starts) != 0 {
		t.Errorf("starts = %v, want none", h.starts)
	}
}

func TestRecordToolBoundaryIsANoOpForAPlainHistory(t *testing.T) {
	h := &plainHistory{}
	group := []pendingCall{{call: llm.ToolCall{ID: "a", Name: "Bash"}}}
	if err := recordToolBoundary(RunLoopOpts{History: h}, group); err != nil {
		t.Fatalf("recordToolBoundary: %v", err)
	}
	if len(h.messages) != 0 {
		t.Errorf("a plain history must not be written to at the boundary: %v", h.messages)
	}
}

func TestRecordToolBoundaryPropagatesFailure(t *testing.T) {
	fail := errors.New("disk full")
	h := &boundaryHistory{failStart: fail}
	group := []pendingCall{{call: llm.ToolCall{ID: "a", Name: "Bash"}}}
	// A boundary that cannot be recorded must stop the turn: running the tool
	// anyway would produce exactly the unclassifiable outcome the record exists
	// to prevent.
	if err := recordToolBoundary(RunLoopOpts{History: h}, group); !errors.Is(err, fail) {
		t.Fatalf("err = %v, want %v", err, fail)
	}
}

func TestAppendToolOutcomeUsesTheDurablePathWhenAvailable(t *testing.T) {
	h := &boundaryHistory{}
	c := pendingCall{
		call:     llm.ToolCall{ID: "call_1", Name: "Bash"},
		result:   "output",
		executed: true,
		parts:    []llm.ContentPart{{Type: llm.ContentPartText, Text: "output"}},
	}
	if err := appendToolOutcome(RunLoopOpts{History: h}, &c); err != nil {
		t.Fatalf("appendToolOutcome: %v", err)
	}
	if len(h.outcomes) != 1 {
		t.Fatalf("outcomes = %v, want one", h.outcomes)
	}
	got := h.outcomes[0]
	if got.ID != "call_1" || got.Status != ToolStatusCompleted || got.Result != "output" || len(got.Parts) != 1 {
		t.Errorf("outcome = %+v, want the call's id, status, result and parts", got)
	}
}

func TestAppendToolOutcomeFallsBackToAToolMessage(t *testing.T) {
	h := &plainHistory{}
	c := pendingCall{call: llm.ToolCall{ID: "call_1", Name: "Bash"}, result: "output", executed: true}
	if err := appendToolOutcome(RunLoopOpts{History: h}, &c); err != nil {
		t.Fatalf("appendToolOutcome: %v", err)
	}
	// An in-memory history offers no boundary, so the loop still has to give
	// the model its tool result.
	if len(h.messages) != 1 || h.messages[0].Role != "tool" || h.messages[0].ToolCallID != "call_1" {
		t.Fatalf("messages = %#v, want one tool-role message", h.messages)
	}
}

func TestAppendToolOutcomePropagatesFailure(t *testing.T) {
	fail := errors.New("disk full")
	h := &boundaryHistory{failResult: fail}
	c := pendingCall{call: llm.ToolCall{ID: "call_1"}, executed: true}
	if err := appendToolOutcome(RunLoopOpts{History: h}, &c); !errors.Is(err, fail) {
		t.Fatalf("err = %v, want %v", err, fail)
	}
}
