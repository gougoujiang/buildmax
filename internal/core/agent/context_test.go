package agent

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

func msg(role, content string) llm.Message { return llm.Message{Role: role, Content: content} }

func toolCallMsg(content string, calls ...llm.ToolCall) llm.Message {
	return llm.Message{Role: "assistant", Content: content, ToolCalls: calls}
}

func toolResultMsg(id, content string) llm.Message {
	return llm.Message{Role: "tool", Content: content, ToolCallID: id}
}

// TestSplitForCompaction_BasicSplit verifies that messages are split into
// toSummarize and toKeep with the reserve window intact.
func TestSplitForCompaction_BasicSplit(t *testing.T) {
	// Build 10 messages with ~20 chars each (~5 tokens each).
	msgs := make([]llm.Message, 10)
	for i := range msgs {
		msgs[i] = msg("user", "hello world 1234")
	}
	// Reserve 3 messages worth (~15 tokens). EstimateMessageTokens ≈ 8 per message.
	reserveTokens := EstimateMessageTokens(msgs[0]) * 3

	toSummarize, toKeep := splitForCompaction(msgs, reserveTokens)
	if len(toSummarize) == 0 {
		t.Fatal("expected toSummarize to be non-empty")
	}
	if len(toKeep) == 0 {
		t.Fatal("expected toKeep to be non-empty")
	}
	if len(toSummarize)+len(toKeep) != len(msgs) {
		t.Errorf("split sum mismatch: %d + %d != %d", len(toSummarize), len(toKeep), len(msgs))
	}
}

// TestSplitForCompaction_EmptyInput returns (nil, msgs) for empty input.
func TestSplitForCompaction_EmptyInput(t *testing.T) {
	toSummarize, toKeep := splitForCompaction(nil, 1000)
	if toSummarize != nil {
		t.Errorf("toSummarize = %v, want nil", toSummarize)
	}
	if toKeep != nil {
		t.Errorf("toKeep = %v, want nil", toKeep)
	}
}

// TestSplitForCompaction_AllFitInReserve returns (nil, msgs) when everything fits.
func TestSplitForCompaction_AllFitInReserve(t *testing.T) {
	msgs := []llm.Message{msg("user", "hi"), msg("assistant", "hello")}
	toSummarize, toKeep := splitForCompaction(msgs, 100_000)
	if toSummarize != nil {
		t.Errorf("toSummarize should be nil when all fit in reserve, got %v", toSummarize)
	}
	if len(toKeep) != len(msgs) {
		t.Errorf("toKeep length = %d, want %d", len(toKeep), len(msgs))
	}
}

// TestSplitForCompaction_NeverSplitsToolCallGroup verifies that the split boundary
// never lands mid-tool-call-group (toKeep must not start with a "tool" role message).
func TestSplitForCompaction_NeverSplitsToolCallGroup(t *testing.T) {
	tc := llm.ToolCall{ID: "1", Name: "bash", Arguments: `{}`}
	msgs := []llm.Message{
		msg("user", "do something"),
		msg("user", "do something else"),
		msg("user", "do something else"),
		msg("user", "do something else"),
		toolCallMsg("running", tc),
		toolResultMsg("1", "done"),
	}
	// Choose a reserve that would naturally land inside the tool-call group.
	reserveTokens := EstimateMessageTokens(msgs[len(msgs)-1]) + 1

	_, toKeep := splitForCompaction(msgs, reserveTokens)
	if len(toKeep) > 0 && toKeep[0].Role == "tool" {
		t.Errorf("toKeep starts with a 'tool' message — split is mid tool-call group")
	}
}
