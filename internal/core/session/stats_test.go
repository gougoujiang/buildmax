package session

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

func statsSession() State {
	return State{
		CompactionIdx: 2,
		Messages: []llm.Message{
			{Role: "user", Content: "1234567890"},
			{Role: "assistant", Content: "ok", ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: "Grep"},
				{ID: "c2", Name: "Read"},
			}},
			{Role: "tool", ToolCallID: "c1", Content: "0123456789012345678901234567890123456789"},
			{Role: "tool", ToolCallID: "c2", Content: "short"},
			// A background event travels as a user message because no provider
			// has a portable role for one.
			{Role: "user", Content: "job done", Source: llm.MessageSourceCommandResult},
			{Role: "assistant", Content: "finished"},
		},
	}
}

func TestStats_CountsWhatTheUserActuallySaid(t *testing.T) {
	got := Stats(statsSession())
	if got.UserMessages != 1 {
		t.Errorf("UserMessages = %d, want 1 — a background event is not the user speaking", got.UserMessages)
	}
	if got.BackgroundMessages != 1 {
		t.Errorf("BackgroundMessages = %d, want 1", got.BackgroundMessages)
	}
	if got.AssistantTurns != 2 {
		t.Errorf("AssistantTurns = %d, want 2", got.AssistantTurns)
	}
	if got.ToolCalls != 2 || got.ToolResults != 2 {
		t.Errorf("ToolCalls/ToolResults = %d/%d, want 2/2", got.ToolCalls, got.ToolResults)
	}
	if got.CompactedMessages != 2 {
		t.Errorf("CompactedMessages = %d, want 2", got.CompactedMessages)
	}
}

// The point of the per-tool breakdown is which tool is filling the context, and
// that is not the call count: one search can outweigh fifty reads.
func TestStats_AttributesResultBytesToTheToolThatProducedThem(t *testing.T) {
	got := Stats(statsSession())
	if len(got.Tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(got.Tools))
	}
	if got.Tools[0].Name != "Grep" {
		t.Errorf("heaviest tool = %q, want Grep — the list is ordered by bytes, not calls", got.Tools[0].Name)
	}
	if got.Tools[0].ResultBytes != 40 {
		t.Errorf("Grep ResultBytes = %d, want 40", got.Tools[0].ResultBytes)
	}
	if got.Tools[0].MaxResultBytes != 40 {
		t.Errorf("Grep MaxResultBytes = %d, want 40", got.Tools[0].MaxResultBytes)
	}
	if got.ToolResultBytes != 45 {
		t.Errorf("ToolResultBytes = %d, want 45", got.ToolResultBytes)
	}
	if got.TextBytes != 28 {
		t.Errorf("TextBytes = %d, want 28 — tool output is counted apart from the conversation", got.TextBytes)
	}
}

// A result whose assistant message was compacted out from under it still holds
// bytes in the context. Dropping it would understate exactly the long sessions
// this view exists for.
func TestStats_UnattributableResultIsCountedNotDropped(t *testing.T) {
	got := Stats(State{Messages: []llm.Message{
		{Role: "tool", ToolCallID: "gone", Content: "1234567890"},
	}})
	if got.ToolResultBytes != 10 {
		t.Errorf("ToolResultBytes = %d, want 10", got.ToolResultBytes)
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "" {
		t.Errorf("Tools = %+v, want one entry under the empty name", got.Tools)
	}
}

// Zero cache reads and unreported cache usage are different facts, and only the
// first says the cache missed.
func TestCacheReadShare_UnreportedIsNotAMiss(t *testing.T) {
	if _, ok := (Meta{PromptTokens: 1000}).CacheReadShare(); ok {
		t.Error("CacheReadShare reported a share for a provider that reported no cache usage")
	}
	share, ok := (Meta{PromptTokens: 1000, CacheReadTokens: 250}).CacheReadShare()
	if !ok || share != 0.25 {
		t.Errorf("CacheReadShare = %v, %v; want 0.25, true", share, ok)
	}
}

func TestStats_EmptyStateIsEmpty(t *testing.T) {
	if got := Stats(State{}); got.UserMessages != 0 || len(got.Tools) != 0 {
		t.Errorf("Stats(State{}) = %+v, want nothing counted", got)
	}
}
