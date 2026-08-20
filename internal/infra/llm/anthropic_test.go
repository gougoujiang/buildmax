package llm

// The Anthropic protocol rejects conversation shapes the other two tolerate, so
// this adapter repairs canonical history into a valid request. These tests pin
// each repair, because the canonical format stays permissive on purpose: making
// core/llm enforce one protocol's rules would charge the other two for them.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// asJSON renders converted messages the way the request encodes them, so a test
// asserts on what the provider would actually receive.
func asJSON(t *testing.T, messages []anthropic.MessageParam) []map[string]any {
	t.Helper()
	encoded, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	return decoded
}

func blockTypes(t *testing.T, message map[string]any) []string {
	t.Helper()
	content, ok := message["content"].([]any)
	if !ok {
		t.Fatalf("message content is not a block list: %+v", message)
	}
	types := make([]string, 0, len(content))
	for _, raw := range content {
		block, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("content entry is not a block: %+v", raw)
		}
		types = append(types, block["type"].(string))
	}
	return types
}

func TestAnthropicLiftsSystemMessagesOutOfHistory(t *testing.T) {
	system, messages, err := anthropicMessages([]cllm.Message{
		{Role: "system", Content: "You are an agent."},
		{Role: "user", Content: "Hi"},
		{Role: "system", Content: "Stay terse."},
	})
	if err != nil {
		t.Fatalf("anthropicMessages: %v", err)
	}
	if system != "You are an agent.\n\nStay terse." {
		t.Errorf("system = %q, want both system messages joined", system)
	}
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want only the user turn", len(messages))
	}
}

// A run of tool results must arrive as one user turn: this protocol expects
// every result answering a turn in the message that immediately follows it.
func TestAnthropicMergesConsecutiveToolResults(t *testing.T) {
	_, messages, err := anthropicMessages([]cllm.Message{
		{Role: "user", Content: "Read both"},
		{Role: "assistant", ToolCalls: []cllm.ToolCall{
			{ID: "call_1", Name: "read", Arguments: `{"path":"a"}`},
			{ID: "call_2", Name: "read", Arguments: `{"path":"b"}`},
		}},
		{Role: "tool", ToolCallID: "call_1", Content: "a"},
		{Role: "tool", ToolCallID: "call_2", Content: "b"},
	})
	if err != nil {
		t.Fatalf("anthropicMessages: %v", err)
	}
	decoded := asJSON(t, messages)
	if len(decoded) != 3 {
		t.Fatalf("got %d messages, want user, assistant, and one merged result turn", len(decoded))
	}
	if role := decoded[2]["role"]; role != "user" {
		t.Errorf("tool results were sent as role %v, want user", role)
	}
	types := blockTypes(t, decoded[2])
	if len(types) != 2 || types[0] != "tool_result" || types[1] != "tool_result" {
		t.Errorf("merged turn blocks = %v, want two tool_result blocks", types)
	}
}

// Trimming and compaction can remove either half of a call/result pair. The
// other protocols tolerate the leftover; this one answers 400, so the leftover
// is dropped here rather than sent.
func TestAnthropicDropsUnpairedToolCallsAndResults(t *testing.T) {
	_, messages, err := anthropicMessages([]cllm.Message{
		{Role: "user", Content: "Go"},
		{Role: "assistant", Content: "Working", ToolCalls: []cllm.ToolCall{
			{ID: "call_answered", Name: "read", Arguments: `{}`},
			{ID: "call_lost", Name: "read", Arguments: `{}`},
		}},
		{Role: "tool", ToolCallID: "call_answered", Content: "ok"},
		{Role: "tool", ToolCallID: "call_never_made", Content: "orphan"},
	})
	if err != nil {
		t.Fatalf("anthropicMessages: %v", err)
	}
	encoded := mustJSON(messages)
	if !strings.Contains(encoded, "call_answered") {
		t.Error("the paired tool call should survive")
	}
	if strings.Contains(encoded, "call_lost") {
		t.Error("a tool call with no result must be dropped")
	}
	if strings.Contains(encoded, "call_never_made") {
		t.Error("a tool result with no call must be dropped")
	}
	// The assistant turn keeps its text even after losing an unanswered call.
	decoded := asJSON(t, messages)
	if types := blockTypes(t, decoded[1]); len(types) != 2 || types[0] != "text" {
		t.Errorf("assistant blocks = %v, want text plus the answered tool_use", types)
	}
}

// A conversation cannot open with model output, and trimming can leave one
// there.
func TestAnthropicDropsMessagesBeforeTheFirstUserTurn(t *testing.T) {
	_, messages, err := anthropicMessages([]cllm.Message{
		{Role: "assistant", Content: "leftover from a trimmed turn"},
		{Role: "user", Content: "Hello"},
	})
	if err != nil {
		t.Fatalf("anthropicMessages: %v", err)
	}
	decoded := asJSON(t, messages)
	if len(decoded) != 1 || decoded[0]["role"] != "user" {
		t.Fatalf("messages = %+v, want only the user turn", decoded)
	}
}

func TestAnthropicRejectsHistoryWithNoUserTurn(t *testing.T) {
	_, _, err := anthropicMessages([]cllm.Message{
		{Role: "system", Content: "prompt"},
		{Role: "assistant", Content: "orphan"},
	})
	if err == nil {
		t.Fatal("expected an error: this protocol cannot be called without a user turn")
	}
}

// An empty text block is rejected, so an empty message is never emitted.
func TestAnthropicSkipsEmptyContent(t *testing.T) {
	_, messages, err := anthropicMessages([]cllm.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: ""},
		{Role: "user", Content: ""},
	})
	if err != nil {
		t.Fatalf("anthropicMessages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want only the non-empty turn", len(messages))
	}
}

// Arguments are replayed exactly as recorded. Malformed arguments become an
// empty object rather than stranding the conversation.
func TestAnthropicToolInput(t *testing.T) {
	if got := anthropicToolInput(`{"path":"a.go"}`); string(got.(json.RawMessage)) != `{"path":"a.go"}` {
		t.Errorf("valid arguments = %v, want them passed through verbatim", got)
	}
	for _, arguments := range []string{"", "   ", "not json"} {
		got, ok := anthropicToolInput(arguments).(map[string]any)
		if !ok || len(got) != 0 {
			t.Errorf("anthropicToolInput(%q) = %v, want an empty object", arguments, got)
		}
	}
}

// A tool schema is converted whole: narrowing it to properties and required
// would silently drop what a tool author wrote.
func TestAnthropicToolSchemaKeepsExtraKeywords(t *testing.T) {
	tools := anthropicTools([]cllm.ToolDef{{
		Name:        "read",
		Description: "Read a file",
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"path": map[string]any{"type": "string"}},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
	}})
	if len(tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(tools))
	}
	encoded := mustJSON(tools[0])
	for _, want := range []string{`"path"`, `"required":["path"]`, `"additionalProperties":false`} {
		if !strings.Contains(encoded, want) {
			t.Errorf("encoded tool %s is missing %s", encoded, want)
		}
	}
}

// A history this protocol cannot represent is a deterministic failure, so it
// must be reported once rather than retried into a threefold delay.
func TestAnthropicUnrepresentableHistoryIsNotRetried(t *testing.T) {
	up := newUpstream(t, protocols[2], reply{text: "unused"}, 0)
	client := newTestClient(t, "anthropic", up.server.URL)

	_, _, _, err := client.ChatCompletionBlocking(t.Context(), []cllm.Message{
		{Role: "system", Content: "prompt"},
		{Role: "assistant", Content: "orphan"},
	}, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if up.requests != 0 {
		t.Errorf("upstream saw %d requests; a request that cannot be built must not be sent", up.requests)
	}
	if isRetryableError(&requestError{err: err}) {
		t.Error("a request that could not be built must not be retryable")
	}
}
