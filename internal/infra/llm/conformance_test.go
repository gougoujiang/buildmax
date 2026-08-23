package llm

// Cross-adapter conformance. One logical model reply is expressed once, encoded
// by each protocol's fixture, and read back through each adapter: the canonical
// content, tool calls, and usage must come out identical.
//
// This is what keeps three wire protocols honest as they change independently.
// The scenarios map onto the four capabilities the model catalog declares —
// text_chat, tool_calls, streaming_text, and usage_reporting.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// reply is one model turn, described in canonical terms. Each protocol fixture
// encodes this same value its own way.
type reply struct {
	text      string
	toolCalls []cllm.ToolCall
	// usage is what the provider reports. nil means it reported none, which is
	// not the same fact as zero tokens.
	usage *cllm.Usage
}

var conformanceScenarios = []struct {
	name  string
	reply reply
}{
	{
		name:  "text only",
		reply: reply{text: "Hello there", usage: &cllm.Usage{PromptTokens: 11, CompletionTokens: 5, TotalTokens: 16}},
	},
	{
		name: "one tool call",
		reply: reply{
			toolCalls: []cllm.ToolCall{{ID: "call_1", Name: "read_file", Arguments: `{"path":"a.go"}`}},
			usage:     &cllm.Usage{PromptTokens: 20, CompletionTokens: 8, TotalTokens: 28},
		},
	},
	{
		name: "text and several tool calls",
		reply: reply{
			text: "Looking at both files.",
			toolCalls: []cllm.ToolCall{
				{ID: "call_1", Name: "read_file", Arguments: `{"path":"a.go"}`},
				{ID: "call_2", Name: "read_file", Arguments: `{"path":"b.go"}`},
			},
			usage: &cllm.Usage{PromptTokens: 30, CompletionTokens: 12, TotalTokens: 42},
		},
	},
	{
		name:  "no usage reported",
		reply: reply{text: "Done."},
	},
}

// conformanceHistory exercises every message shape the canonical format has:
// a system prompt, a user turn, an assistant turn with tool calls, and the
// results answering them.
func conformanceHistory() []cllm.Message {
	return []cllm.Message{
		{Role: "system", Content: "You are a helpful agent."},
		{Role: "user", Content: "Read a.go and b.go"},
		{Role: "assistant", Content: "On it.", ToolCalls: []cllm.ToolCall{
			{ID: "call_1", Name: "read_file", Arguments: `{"path":"a.go"}`},
			{ID: "call_2", Name: "read_file", Arguments: `{"path":"b.go"}`},
		}},
		{Role: "tool", ToolCallID: "call_1", Content: "package a"},
		{Role: "tool", ToolCallID: "call_2", Content: "package b"},
	}
}

func conformanceTools() []cllm.ToolDef {
	return []cllm.ToolDef{{
		Name:        "read_file",
		Description: "Read a file",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"path": map[string]any{"type": "string"}},
			"required":   []string{"path"},
		},
	}}
}

// wantUsage is the canonical usage a reply should produce.
func (r reply) wantUsage() cllm.Usage {
	if r.usage == nil {
		return cllm.Usage{}
	}
	return *r.usage
}

// --- Protocol fixtures ------------------------------------------------------

func openAIChatBody(r reply) string {
	message := map[string]any{"role": "assistant", "content": r.text}
	if len(r.toolCalls) > 0 {
		calls := make([]any, 0, len(r.toolCalls))
		for _, tc := range r.toolCalls {
			calls = append(calls, map[string]any{
				"id":       tc.ID,
				"type":     "function",
				"function": map[string]any{"name": tc.Name, "arguments": tc.Arguments},
			})
		}
		message["tool_calls"] = calls
	}
	body := map[string]any{
		"id":      "chatcmpl-1",
		"object":  "chat.completion",
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": "stop"}},
	}
	if r.usage != nil {
		body["usage"] = map[string]any{
			"prompt_tokens":     r.usage.PromptTokens,
			"completion_tokens": r.usage.CompletionTokens,
			"total_tokens":      r.usage.TotalTokens,
		}
	}
	return mustJSON(body)
}

func openAIChatSSE(r reply) string {
	var b strings.Builder
	chunk := func(delta any) {
		b.WriteString("data: " + mustJSON(map[string]any{
			"id":      "chatcmpl-1",
			"object":  "chat.completion.chunk",
			"choices": []any{map[string]any{"index": 0, "delta": delta}},
		}) + "\n\n")
	}
	for _, piece := range splitForStream(r.text) {
		chunk(map[string]any{"content": piece})
	}
	for i, tc := range r.toolCalls {
		chunk(map[string]any{"tool_calls": []any{map[string]any{
			"index":    i,
			"id":       tc.ID,
			"type":     "function",
			"function": map[string]any{"name": tc.Name, "arguments": ""},
		}}})
		for _, piece := range splitForStream(tc.Arguments) {
			chunk(map[string]any{"tool_calls": []any{map[string]any{
				"index":    i,
				"function": map[string]any{"arguments": piece},
			}}})
		}
	}
	if r.usage != nil {
		b.WriteString("data: " + mustJSON(map[string]any{
			"id":      "chatcmpl-1",
			"object":  "chat.completion.chunk",
			"choices": []any{},
			"usage": map[string]any{
				"prompt_tokens":     r.usage.PromptTokens,
				"completion_tokens": r.usage.CompletionTokens,
				"total_tokens":      r.usage.TotalTokens,
			},
		}) + "\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

func responsesOutputItems(r reply) []any {
	items := make([]any, 0, len(r.toolCalls)+1)
	if r.text != "" {
		items = append(items, map[string]any{
			"type":    "message",
			"role":    "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": r.text}},
		})
	}
	for _, tc := range r.toolCalls {
		items = append(items, map[string]any{
			"type":      "function_call",
			"id":        "fc_" + tc.ID,
			"call_id":   tc.ID,
			"name":      tc.Name,
			"arguments": tc.Arguments,
		})
	}
	return items
}

func responsesBody(r reply) string {
	body := map[string]any{
		"id":     "resp_1",
		"object": "response",
		"status": "completed",
		"model":  "m",
		"output": responsesOutputItems(r),
	}
	if r.usage != nil {
		body["usage"] = map[string]any{
			"input_tokens":  r.usage.PromptTokens,
			"output_tokens": r.usage.CompletionTokens,
			"total_tokens":  r.usage.TotalTokens,
		}
	}
	return mustJSON(body)
}

func responsesSSE(r reply) string {
	var b strings.Builder
	for _, piece := range splitForStream(r.text) {
		b.WriteString("data: " + mustJSON(map[string]any{
			"type":  "response.output_text.delta",
			"delta": piece,
		}) + "\n\n")
	}
	for _, tc := range r.toolCalls {
		b.WriteString("data: " + mustJSON(map[string]any{
			"type": "response.output_item.done",
			"item": map[string]any{
				"type":      "function_call",
				"id":        "fc_" + tc.ID,
				"call_id":   tc.ID,
				"name":      tc.Name,
				"arguments": tc.Arguments,
			},
		}) + "\n\n")
	}
	var response map[string]any
	if err := json.Unmarshal([]byte(responsesBody(r)), &response); err != nil {
		panic(err)
	}
	b.WriteString("data: " + mustJSON(map[string]any{
		"type":     "response.completed",
		"response": response,
	}) + "\n\n")
	return b.String()
}

func anthropicBody(r reply) string {
	content := make([]any, 0, len(r.toolCalls)+1)
	if r.text != "" {
		content = append(content, map[string]any{"type": "text", "text": r.text})
	}
	for _, tc := range r.toolCalls {
		content = append(content, map[string]any{
			"type":  "tool_use",
			"id":    tc.ID,
			"name":  tc.Name,
			"input": json.RawMessage(tc.Arguments),
		})
	}
	usage := map[string]any{"input_tokens": 0, "output_tokens": 0}
	if r.usage != nil {
		usage = map[string]any{
			"input_tokens":  r.usage.PromptTokens,
			"output_tokens": r.usage.CompletionTokens,
		}
	}
	return mustJSON(map[string]any{
		"id":      "msg_1",
		"type":    "message",
		"role":    "assistant",
		"model":   "m",
		"content": content,
		"usage":   usage,
	})
}

func anthropicSSE(r reply) string {
	var b strings.Builder
	event := func(name string, payload map[string]any) {
		payload["type"] = name
		b.WriteString("event: " + name + "\ndata: " + mustJSON(payload) + "\n\n")
	}
	inputTokens := 0
	if r.usage != nil {
		inputTokens = r.usage.PromptTokens
	}
	event("message_start", map[string]any{"message": map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "m",
		"content": []any{},
		"usage":   map[string]any{"input_tokens": inputTokens, "output_tokens": 0},
	}})

	index := 0
	if r.text != "" {
		event("content_block_start", map[string]any{
			"index": index, "content_block": map[string]any{"type": "text", "text": ""},
		})
		for _, piece := range splitForStream(r.text) {
			event("content_block_delta", map[string]any{
				"index": index, "delta": map[string]any{"type": "text_delta", "text": piece},
			})
		}
		event("content_block_stop", map[string]any{"index": index})
		index++
	}
	for _, tc := range r.toolCalls {
		event("content_block_start", map[string]any{
			"index": index,
			"content_block": map[string]any{
				"type": "tool_use", "id": tc.ID, "name": tc.Name, "input": map[string]any{},
			},
		})
		for _, piece := range splitForStream(tc.Arguments) {
			event("content_block_delta", map[string]any{
				"index": index,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": piece},
			})
		}
		event("content_block_stop", map[string]any{"index": index})
		index++
	}

	outputTokens := 0
	if r.usage != nil {
		outputTokens = r.usage.CompletionTokens
	}
	event("message_delta", map[string]any{
		"delta": map[string]any{"stop_reason": "end_turn"},
		"usage": map[string]any{"output_tokens": outputTokens},
	})
	event("message_stop", map[string]any{})
	return b.String()
}

// ollamaMessagePayload encodes a reply the way /api/chat carries one. Tool
// calls have no identifier on this protocol and their arguments are an object,
// which is exactly what the adapter has to repair.
func ollamaMessagePayload(r reply) map[string]any {
	message := map[string]any{"role": "assistant", "content": r.text}
	if len(r.toolCalls) > 0 {
		calls := make([]any, 0, len(r.toolCalls))
		for _, tc := range r.toolCalls {
			calls = append(calls, map[string]any{
				"function": map[string]any{
					"name":      tc.Name,
					"arguments": json.RawMessage(tc.Arguments),
				},
			})
		}
		message["tool_calls"] = calls
	}
	return message
}

func ollamaBody(r reply) string {
	body := map[string]any{
		"model":       "m",
		"message":     ollamaMessagePayload(r),
		"done":        true,
		"done_reason": "stop",
	}
	if r.usage != nil {
		body["prompt_eval_count"] = r.usage.PromptTokens
		body["eval_count"] = r.usage.CompletionTokens
	}
	return mustJSON(body)
}

// ollamaNDJSON is a stream of whole JSON objects rather than SSE, and tool
// calls arrive complete inside one of them rather than as argument deltas.
func ollamaNDJSON(r reply) string {
	var b strings.Builder
	for _, piece := range splitForStream(r.text) {
		b.WriteString(mustJSON(map[string]any{
			"model":   "m",
			"message": map[string]any{"role": "assistant", "content": piece},
			"done":    false,
		}) + "\n")
	}
	if len(r.toolCalls) > 0 {
		message := ollamaMessagePayload(reply{toolCalls: r.toolCalls})
		message["content"] = ""
		b.WriteString(mustJSON(map[string]any{"model": "m", "message": message, "done": false}) + "\n")
	}
	final := map[string]any{
		"model":       "m",
		"message":     map[string]any{"role": "assistant", "content": ""},
		"done":        true,
		"done_reason": "stop",
	}
	if r.usage != nil {
		final["prompt_eval_count"] = r.usage.PromptTokens
		final["eval_count"] = r.usage.CompletionTokens
	}
	b.WriteString(mustJSON(final) + "\n")
	return b.String()
}

// splitForStream cuts a value into two pieces so streaming fixtures deliver it
// in more than one event, which is how a real provider sends it.
func splitForStream(s string) []string {
	switch {
	case s == "":
		return nil
	case len(s) < 4:
		return []string{s}
	default:
		mid := len(s) / 2
		return []string{s[:mid], s[mid:]}
	}
}

// shortenRetryBackoff makes a retry test finish in milliseconds. The decision
// to retry is what is under test; the wait between tries is covered by
// retry_test.go.
func shortenRetryBackoff() func() {
	original := retryBackoff
	retryBackoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	return func() { retryBackoff = original }
}

func mustJSON(v any) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

// --- Harness ----------------------------------------------------------------

// protocol names one adapter together with the fixtures that feed it.
type protocol struct {
	provider string
	blocking func(reply) string
	stream   func(reply) string
	// errorBody is how this protocol reports a failure.
	errorBody func(status int) string
	// toolCallID is set by a protocol that carries no identifiers of its own
	// and whose adapter therefore mints them. It maps a scenario's identifier
	// to the one that protocol will produce for the same reply.
	toolCallID func(history []cllm.Message, index int) string
	// pairsCallWithResult reports whether an outgoing request still links a
	// tool call to the result answering it. Set only by a protocol that does
	// not link them by identifier.
	pairsCallWithResult func(body, id, name string) bool
}

// want restates a canonical scenario as this protocol will deliver it. Only the
// identifiers move; content, arguments, and usage must survive unchanged, which
// is what the suite is for.
func (p protocol) want(r reply, history []cllm.Message) reply {
	if p.toolCallID == nil {
		return r
	}
	out := r
	out.toolCalls = make([]cllm.ToolCall, len(r.toolCalls))
	copy(out.toolCalls, r.toolCalls)
	for i := range out.toolCalls {
		out.toolCalls[i].ID = p.toolCallID(history, i)
	}
	return out
}

var protocols = []protocol{
	{
		provider: config.LLMProviderOpenAICompatible,
		blocking: openAIChatBody,
		stream:   openAIChatSSE,
		errorBody: func(int) string {
			return `{"error":{"message":"upstream refused","type":"invalid_request_error"}}`
		},
	},
	{
		provider: config.LLMProviderOpenAI,
		blocking: responsesBody,
		stream:   responsesSSE,
		errorBody: func(int) string {
			return `{"error":{"message":"upstream refused","type":"invalid_request_error"}}`
		},
	},
	{
		provider: config.LLMProviderAnthropic,
		blocking: anthropicBody,
		stream:   anthropicSSE,
		errorBody: func(int) string {
			return `{"type":"error","error":{"type":"authentication_error","message":"upstream refused"}}`
		},
	},
	{
		provider: config.LLMProviderOllama,
		blocking: ollamaBody,
		stream:   ollamaNDJSON,
		errorBody: func(int) string {
			return `{"error":"upstream refused"}`
		},
		toolCallID: func(history []cllm.Message, index int) string {
			return fmt.Sprintf("call_%d", priorToolCalls(history)+index+1)
		},
		// This protocol answers a call by name, so the identifier is resolved
		// on the way out and must not appear on the wire at all.
		pairsCallWithResult: func(body, id, name string) bool {
			return strings.Contains(body, `"tool_name":"`+name+`"`) &&
				strings.Contains(body, `"name":"`+name+`"`) &&
				!strings.Contains(body, id)
		},
	},
}

// upstream is a fake provider that answers whichever protocol is asked of it.
type upstream struct {
	server   *httptest.Server
	requests int
	bodies   []string
}

// newUpstreamWithBody answers every request with one fixed body, for a test
// that supplies its own protocol-shaped fixture.
func newUpstreamWithBody(t *testing.T, body string) *upstream {
	t.Helper()
	up := &upstream{}
	up.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		up.requests++
		requestBody, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		up.bodies = append(up.bodies, string(requestBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(up.server.Close)
	return up
}

func newUpstream(t *testing.T, p protocol, r reply, failStatus int) *upstream {
	t.Helper()
	up := &upstream{}
	up.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		up.requests++
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		up.bodies = append(up.bodies, string(body))

		if failStatus != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(failStatus)
			_, _ = w.Write([]byte(p.errorBody(failStatus)))
			return
		}
		if strings.Contains(string(body), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(p.stream(r)))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(p.blocking(r)))
	}))
	t.Cleanup(up.server.Close)
	return up
}

func newTestClient(t *testing.T, provider, baseURL string) *LLMClient {
	t.Helper()
	client, err := NewClient(Config{
		Provider:      provider,
		APIKey:        "test-key",
		BaseURL:       baseURL,
		Model:         "test-model",
		ContextWindow: 32_000,
	})
	if err != nil {
		t.Fatalf("NewClient(%s): %v", provider, err)
	}
	return client
}

// --- Tests ------------------------------------------------------------------

func TestAdaptersAgreeOnBlockingReplies(t *testing.T) {
	for _, scenario := range conformanceScenarios {
		for _, p := range protocols {
			t.Run(scenario.name+"/"+p.provider, func(t *testing.T) {
				up := newUpstream(t, p, scenario.reply, 0)
				client := newTestClient(t, p.provider, up.server.URL)

				completion, err := client.ChatCompletionBlocking(
					context.Background(), conformanceHistory(), conformanceTools())
				if err != nil {
					t.Fatalf("ChatCompletionBlocking: %v", err)
				}
				assertReply(t, p.want(scenario.reply, conformanceHistory()), completion)
			})
		}
	}
}

func TestAdaptersAgreeOnStreamedReplies(t *testing.T) {
	for _, scenario := range conformanceScenarios {
		for _, p := range protocols {
			t.Run(scenario.name+"/"+p.provider, func(t *testing.T) {
				up := newUpstream(t, p, scenario.reply, 0)
				client := newTestClient(t, p.provider, up.server.URL)

				var delivered strings.Builder
				completion, err := client.ChatCompletionStreaming(
					context.Background(), conformanceHistory(), conformanceTools(),
					func(delta string) { delivered.WriteString(delta) })
				if err != nil {
					t.Fatalf("ChatCompletionStreaming: %v", err)
				}
				assertReply(t, p.want(scenario.reply, conformanceHistory()), completion)
				if delivered.String() != scenario.reply.text {
					t.Errorf("deltas delivered %q, want %q", delivered.String(), scenario.reply.text)
				}
			})
		}
	}
}

func assertReply(t *testing.T, want reply, got cllm.Completion) {
	t.Helper()
	if got.Content != want.text {
		t.Errorf("content = %q, want %q", got.Content, want.text)
	}
	if len(got.ToolCalls) != len(want.toolCalls) {
		t.Fatalf("got %d tool calls, want %d: %+v", len(got.ToolCalls), len(want.toolCalls), got.ToolCalls)
	}
	for i, call := range got.ToolCalls {
		expected := want.toolCalls[i]
		if call.ID != expected.ID || call.Name != expected.Name || call.Arguments != expected.Arguments {
			t.Errorf("tool call %d = %+v, want %+v", i, call, expected)
		}
	}
	if got.Usage != want.wantUsage() {
		t.Errorf("usage = %+v, want %+v", got.Usage, want.wantUsage())
	}
	// Reasoning is off in these scenarios, so no protocol may invent state.
	if got.ProviderState != nil {
		t.Errorf("provider state = %+v, want none when reasoning is off", got.ProviderState)
	}
}

// TestAdaptersAgreeOnErrorClassification pins the two things a caller acts on:
// what the failure is called, and whether trying again is worth it.
func TestAdaptersAgreeOnErrorClassification(t *testing.T) {
	cases := []struct {
		status       int
		wantContains string
		wantRequests int
	}{
		{status: http.StatusUnauthorized, wantContains: "authentication failed", wantRequests: 1},
		{status: http.StatusTooManyRequests, wantContains: "rate limited", wantRequests: maxRetryAttempts},
		{status: http.StatusInternalServerError, wantContains: "internal server error", wantRequests: maxRetryAttempts},
		{status: http.StatusBadRequest, wantContains: "400", wantRequests: 1},
	}
	for _, tc := range cases {
		for _, p := range protocols {
			t.Run(fmt.Sprintf("%d/%s", tc.status, p.provider), func(t *testing.T) {
				up := newUpstream(t, p, reply{}, tc.status)
				client := newTestClient(t, p.provider, up.server.URL)
				// Backoff would make this test sleep for seconds; the retry
				// decision is what is under test, not the wait between tries.
				restore := shortenRetryBackoff()
				defer restore()

				_, err := client.ChatCompletionBlocking(
					context.Background(), conformanceHistory(), conformanceTools())
				if err == nil {
					t.Fatal("expected an error")
				}
				if !strings.Contains(err.Error(), tc.wantContains) {
					t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantContains)
				}
				if up.requests != tc.wantRequests {
					t.Errorf("upstream saw %d requests, want %d", up.requests, tc.wantRequests)
				}
			})
		}
	}
}

// TestUnknownProviderIsRejected keeps a misconfigured model from quietly
// reaching a provider the operator did not name.
func TestUnknownProviderIsRejected(t *testing.T) {
	_, err := NewClient(Config{Provider: "bedrock", APIKey: "k", Model: "m"})
	if err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
	for _, known := range config.LLMProviders() {
		if !strings.Contains(err.Error(), known) {
			t.Errorf("error %q should name the supported provider %q", err.Error(), known)
		}
	}
}

// TestDefaultProviderIsChatCompletions keeps every configuration written before
// providers existed calling exactly what it always called.
func TestDefaultProviderIsChatCompletions(t *testing.T) {
	client := newTestClient(t, "", "http://127.0.0.1:1")
	if got := client.Provider(); got != config.LLMProviderOpenAICompatible {
		t.Errorf("default provider = %q, want %q", got, config.LLMProviderOpenAICompatible)
	}
}

// TestToolCallIDsSurviveAcrossProtocols pins the assumption that makes a
// session portable: history is stored in the canonical format, so a
// conversation started under one protocol can be continued under another, and
// the tool call identifiers it recorded are echoed back unchanged.
//
// The identifiers are opaque strings each provider mints in its own shape, so
// the test replays every shape through every adapter. Ollama is the one
// protocol with no identifier field: it pairs by tool name, so what survives
// there is the link rather than the string.
func TestToolCallIDsSurviveAcrossProtocols(t *testing.T) {
	mintedIDs := map[string]string{
		"openai_compatible": "call_abc123",
		"openai_responses":  "fc_68a1b2c3",
		"anthropic":         "toolu_01ABCdefGHI",
	}
	for mintedBy, id := range mintedIDs {
		for _, p := range protocols {
			t.Run(mintedBy+"_replayed_on_"+p.provider, func(t *testing.T) {
				up := newUpstream(t, p, reply{text: "ok"}, 0)
				client := newTestClient(t, p.provider, up.server.URL)

				history := []cllm.Message{
					{Role: "user", Content: "Read a.go"},
					{Role: "assistant", ToolCalls: []cllm.ToolCall{
						{ID: id, Name: "read_file", Arguments: `{"path":"a.go"}`},
					}},
					{Role: "tool", ToolCallID: id, Content: "package a"},
				}
				if _, err := client.ChatCompletionBlocking(
					context.Background(), history, conformanceTools()); err != nil {
					t.Fatalf("ChatCompletionBlocking: %v", err)
				}
				if len(up.bodies) != 1 {
					t.Fatalf("upstream saw %d requests, want 1", len(up.bodies))
				}
				// Twice: once naming the call, once answering it. A protocol
				// that dropped either half would strand the turn.
				paired := strings.Count(up.bodies[0], id) == 2
				if p.pairsCallWithResult != nil {
					paired = p.pairsCallWithResult(up.bodies[0], id, "read_file")
				}
				if !paired {
					t.Errorf("request %s should link the call %q to its result",
						up.bodies[0], id)
				}
			})
		}
	}
}

// TestSystemPromptReachesEveryProtocol keeps the one message shape the OpenAI
// protocols carry inline and the others move to a top-level field.
func TestSystemPromptReachesEveryProtocol(t *testing.T) {
	const prompt = "You are a careful agent."
	for _, p := range protocols {
		t.Run(p.provider, func(t *testing.T) {
			up := newUpstream(t, p, reply{text: "ok"}, 0)
			client := newTestClient(t, p.provider, up.server.URL)

			if _, err := client.ChatCompletionBlocking(context.Background(), []cllm.Message{
				{Role: "system", Content: prompt},
				{Role: "user", Content: "Hello"},
			}, nil); err != nil {
				t.Fatalf("ChatCompletionBlocking: %v", err)
			}
			if !strings.Contains(up.bodies[0], prompt) {
				t.Errorf("request %s does not carry the system prompt", up.bodies[0])
			}
		})
	}
}
