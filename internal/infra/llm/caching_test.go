package llm

// Prompt caching splits three ways: Anthropic needs breakpoints placed in the
// request, and both OpenAI protocols cache on their own and only report it. The
// reporting has to agree anyway, because a spend report reads one shape.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

func newCachingClient(t *testing.T, provider, baseURL string) *LLMClient {
	t.Helper()
	client, err := NewClient(Config{
		Provider: provider, APIKey: "test-key", BaseURL: baseURL,
		Model: "test-model", ContextWindow: 32_000,
		CacheControl: config.CacheControl{Mode: config.CacheModeForce},
	})
	if err != nil {
		t.Fatalf("NewClient(%s): %v", provider, err)
	}
	return client
}

// The tool definitions and system prompt are the same on every call in a run,
// so that is where the first breakpoint goes.
func TestAnthropicPlacesCacheBreakpoints(t *testing.T) {
	up := newUpstreamWithBody(t, anthropicBody(reply{text: "ok"}))
	client := newCachingClient(t, cllm.ProviderAnthropic, up.server.URL)

	if _, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{
		Messages: conformanceHistory(), Tools: conformanceTools(), Profile: cllm.ProfileAgentTurn,
	}); err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	var request map[string]any
	if err := json.Unmarshal([]byte(up.bodies[0]), &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	system, ok := request["system"].([]any)
	if !ok || len(system) == 0 {
		t.Fatalf("request %s has no system blocks", up.bodies[0])
	}
	if _, marked := system[len(system)-1].(map[string]any)["cache_control"]; !marked {
		t.Error("the system prompt carries no cache breakpoint")
	}
	if _, marked := request["cache_control"]; !marked {
		t.Error("the request carries no trailing cache breakpoint")
	}
}

// An agent turn caches without being configured to. The prefix it sends —
// tools, system prompt, the history so far — goes out again on the next
// iteration, which is the case a cache write is priced for.
func TestAgentTurnsCacheByDefaultOnAnthropic(t *testing.T) {
	up := newUpstreamWithBody(t, anthropicBody(reply{text: "ok"}))
	client := newTestClient(t, cllm.ProviderAnthropic, up.server.URL)

	if _, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{
		Messages: conformanceHistory(), Tools: conformanceTools(), Profile: cllm.ProfileAgentTurn,
	}); err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	if !strings.Contains(up.bodies[0], "cache_control") {
		t.Errorf("request %s carries no breakpoint; an agent turn is the case caching pays for",
			up.bodies[0])
	}
}

// The same default sends nothing on a call that will never be sent again. A
// write costs more than ordinary input, so buying one for a title is a straight
// loss — and it is the reason the default can be on at all.
func TestUtilityCallsDoNotCacheByDefault(t *testing.T) {
	for _, profile := range []cllm.CallProfile{
		cllm.ProfileTitle, cllm.ProfileCompaction, cllm.ProfileProbe, cllm.ProfileEvaluation,
	} {
		t.Run(string(profile), func(t *testing.T) {
			up := newUpstreamWithBody(t, anthropicBody(reply{text: "ok"}))
			client := newTestClient(t, cllm.ProviderAnthropic, up.server.URL)

			if _, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{
				Messages: conformanceHistory(), Tools: conformanceTools(), Profile: profile,
			}); err != nil {
				t.Fatalf("ChatCompletionBlocking: %v", err)
			}
			if strings.Contains(up.bodies[0], "cache_control") {
				t.Errorf("request %s bought a cache write nothing will read", up.bodies[0])
			}
		})
	}
}

// An explicit off is an opt-out and stays one, on every call.
func TestOffSendsNoBreakpointOnAnAgentTurn(t *testing.T) {
	up := newUpstreamWithBody(t, anthropicBody(reply{text: "ok"}))
	client, err := NewClient(Config{
		Provider: cllm.ProviderAnthropic, APIKey: "test-key", BaseURL: up.server.URL,
		Model: "test-model", ContextWindow: 32_000,
		CacheControl: config.CacheControl{Mode: config.CacheModeOff},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{
		Messages: conformanceHistory(), Tools: conformanceTools(), Profile: cllm.ProfileAgentTurn,
	}); err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	if strings.Contains(up.bodies[0], "cache_control") {
		t.Errorf("request %s cached after an explicit opt-out", up.bodies[0])
	}
}

// Retention beyond the provider's default is deliberate, so it appears in the
// request only when it was asked for.
func TestAnthropicSendsTheRequestedTTL(t *testing.T) {
	tests := map[string]string{
		config.CacheTTLProviderDefault: "",
		config.CacheTTL1h:              "1h",
		config.CacheTTL5m:              "5m",
	}
	for policy, want := range tests {
		t.Run(policy, func(t *testing.T) {
			up := newUpstreamWithBody(t, anthropicBody(reply{text: "ok"}))
			client, err := NewClient(Config{
				Provider: cllm.ProviderAnthropic, APIKey: "test-key", BaseURL: up.server.URL,
				Model: "test-model", ContextWindow: 32_000,
				CacheControl: config.CacheControl{Mode: config.CacheModeAuto, TTL: policy},
			})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if _, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{
				Messages: conformanceHistory(), Tools: conformanceTools(), Profile: cllm.ProfileAgentTurn,
			}); err != nil {
				t.Fatalf("ChatCompletionBlocking: %v", err)
			}
			hasTTL := strings.Contains(up.bodies[0], `"ttl"`)
			if want == "" {
				if hasTTL {
					t.Errorf("request %s pinned a retention the operator left to the provider", up.bodies[0])
				}
				return
			}
			if !strings.Contains(up.bodies[0], `"ttl":"`+want+`"`) {
				t.Errorf("request %s does not carry ttl %q", up.bodies[0], want)
			}
		})
	}
}

// With no system prompt the tool definitions are still the stable part of the
// request. Without a breakpoint on them the only cacheable boundary left is the
// rolling one, which lands after a user message that changes every turn — so
// the run would pay to cache the one part that is never the same twice.
func TestAnthropicCachesToolsWhenThereIsNoSystemPrompt(t *testing.T) {
	up := newUpstreamWithBody(t, anthropicBody(reply{text: "ok"}))
	client := newTestClient(t, cllm.ProviderAnthropic, up.server.URL)

	if _, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{
		Messages: []cllm.Message{{Role: "user", Content: "hi"}},
		Tools:    conformanceTools(),
		Profile:  cllm.ProfileAgentTurn,
	}); err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	var request map[string]any
	if err := json.Unmarshal([]byte(up.bodies[0]), &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if _, present := request["system"]; present {
		t.Fatalf("this scenario needs a request with no system block: %s", up.bodies[0])
	}
	tools, ok := request["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("request %s has no tools", up.bodies[0])
	}
	if _, marked := tools[len(tools)-1].(map[string]any)["cache_control"]; !marked {
		t.Errorf("request %s marks no stable tools boundary", up.bodies[0])
	}
}

// Every protocol reports cached tokens as a breakdown of the prompt, never as an
// addition to it: a report that summed them would bill the same tokens twice.
func TestCacheTokensAreReportedAsPartOfThePrompt(t *testing.T) {
	tests := []struct {
		provider string
		body     string
		want     cllm.Usage
	}{
		{
			provider: cllm.ProviderAnthropic,
			body: mustJSON(map[string]any{
				"id": "msg_1", "type": "message", "role": "assistant", "model": "m",
				"content": []any{map[string]any{"type": "text", "text": "ok"}},
				"usage": map[string]any{
					"input_tokens": 10, "output_tokens": 4,
					"cache_read_input_tokens": 80, "cache_creation_input_tokens": 10,
				},
			}),
			// This protocol reports cached input apart from input_tokens, so the
			// adapter adds it back: 10 fresh + 80 read + 10 written.
			want: cllm.Usage{
				PromptTokens: 100, CompletionTokens: 4, TotalTokens: 104,
				CacheReadTokens: 80, CacheWriteTokens: 10,
			},
		},
		{
			provider: cllm.ProviderOpenAI,
			body: mustJSON(map[string]any{
				"id": "resp_1", "object": "response", "status": "completed", "model": "m",
				"output": []any{map[string]any{
					"type": "message", "role": "assistant",
					"content": []any{map[string]any{"type": "output_text", "text": "ok"}},
				}},
				"usage": map[string]any{
					"input_tokens": 100, "output_tokens": 4, "total_tokens": 104,
					"input_tokens_details": map[string]any{"cached_tokens": 80},
				},
			}),
			// Here the cached part is already inside input_tokens.
			want: cllm.Usage{
				PromptTokens: 100, CompletionTokens: 4, TotalTokens: 104,
				CacheReadTokens: 80,
			},
		},
		{
			provider: cllm.ProviderOpenAICompatible,
			body: mustJSON(map[string]any{
				"id": "chatcmpl-1", "object": "chat.completion",
				"choices": []any{map[string]any{
					"index": 0, "finish_reason": "stop",
					"message": map[string]any{"role": "assistant", "content": "ok"},
				}},
				"usage": map[string]any{
					"prompt_tokens": 100, "completion_tokens": 4, "total_tokens": 104,
					"prompt_tokens_details": map[string]any{"cached_tokens": 80},
				},
			}),
			want: cllm.Usage{
				PromptTokens: 100, CompletionTokens: 4, TotalTokens: 104,
				CacheReadTokens: 80,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			up := newUpstreamWithBody(t, tc.body)
			client := newTestClient(t, tc.provider, up.server.URL)

			completion, err := client.ChatCompletionBlocking(context.Background(),
				cllm.Request{Messages: []cllm.Message{{Role: "user", Content: "hi"}}})
			if err != nil {
				t.Fatalf("ChatCompletionBlocking: %v", err)
			}
			if completion.Usage != tc.want {
				t.Errorf("usage = %+v, want %+v", completion.Usage, tc.want)
			}
			if completion.Usage.CacheReadTokens > completion.Usage.PromptTokens {
				t.Error("cached tokens exceed the prompt they are part of")
			}
		})
	}
}

// An unknown effort level is refused at construction rather than sent, so a
// typo in settings.yaml names itself instead of arriving as a provider error.
func TestUnknownReasoningEffortIsRejected(t *testing.T) {
	_, err := NewClient(Config{
		Provider: cllm.ProviderAnthropic, APIKey: "k", Model: "m", Reasoning: "maximum",
	})
	if err == nil {
		t.Fatal("expected an error for an unknown effort level")
	}
	for _, level := range config.ReasoningEfforts() {
		if !strings.Contains(err.Error(), level) {
			t.Errorf("error %q should name the supported level %q", err.Error(), level)
		}
	}
}

// The streaming path reports usage from a different place in every protocol —
// a trailing chunk, a completed event, a message_start — so the blocking table
// above proves nothing about it. A run that streams is the normal case, and a
// cache count that only survives the blocking path would leave every real turn
// looking uncached.
func TestCacheTokensSurviveStreaming(t *testing.T) {
	tests := []struct {
		provider string
		body     string
		want     cllm.Usage
	}{
		{
			provider: cllm.ProviderAnthropic,
			body:     anthropicCacheSSE(),
			// 10 fresh + 80 read + 10 written, the same addition the blocking
			// path makes.
			want: cllm.Usage{
				PromptTokens: 100, CompletionTokens: 4, TotalTokens: 104,
				CacheReadTokens: 80, CacheWriteTokens: 10,
			},
		},
		{
			provider: cllm.ProviderOpenAI,
			body:     responsesCacheSSE(),
			want: cllm.Usage{
				PromptTokens: 100, CompletionTokens: 4, TotalTokens: 104,
				CacheReadTokens: 80,
			},
		},
		{
			provider: cllm.ProviderOpenAICompatible,
			body:     openAIChatCacheSSE(),
			want: cllm.Usage{
				PromptTokens: 100, CompletionTokens: 4, TotalTokens: 104,
				CacheReadTokens: 80,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			up := newStreamingUpstream(t, tc.body)
			client := newTestClient(t, tc.provider, up.server.URL)

			completion, err := client.ChatCompletionStreaming(context.Background(),
				cllm.Request{Messages: []cllm.Message{{Role: "user", Content: "hi"}}}, func(string) {})
			if err != nil {
				t.Fatalf("ChatCompletionStreaming: %v", err)
			}
			if completion.Usage != tc.want {
				t.Errorf("usage = %+v, want %+v", completion.Usage, tc.want)
			}
			if completion.Usage.CacheReadTokens+completion.Usage.CacheWriteTokens > completion.Usage.PromptTokens {
				t.Error("cached tokens exceed the prompt they are part of")
			}
		})
	}
}

// newStreamingUpstream answers with one fixed SSE body regardless of the
// request, so a fixture can pin a usage shape the shared harness does not
// produce.
func newStreamingUpstream(t *testing.T, body string) *upstream {
	t.Helper()
	up := &upstream{}
	up.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		up.requests++
		requestBody, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		up.bodies = append(up.bodies, string(requestBody))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(up.server.Close)
	return up
}

// anthropicCacheSSE carries the cached counts on message_start, which is where
// this protocol reports input tokens for a streamed message.
func anthropicCacheSSE() string {
	var b strings.Builder
	event := func(name string, payload map[string]any) {
		payload["type"] = name
		b.WriteString("event: " + name + "\ndata: " + mustJSON(payload) + "\n\n")
	}
	event("message_start", map[string]any{"message": map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "m",
		"content": []any{},
		"usage": map[string]any{
			"input_tokens": 10, "output_tokens": 0,
			"cache_read_input_tokens": 80, "cache_creation_input_tokens": 10,
		},
	}})
	event("content_block_start", map[string]any{
		"index": 0, "content_block": map[string]any{"type": "text", "text": ""},
	})
	event("content_block_delta", map[string]any{
		"index": 0, "delta": map[string]any{"type": "text_delta", "text": "ok"},
	})
	event("content_block_stop", map[string]any{"index": 0})
	event("message_delta", map[string]any{
		"delta": map[string]any{"stop_reason": "end_turn"},
		"usage": map[string]any{"output_tokens": 4},
	})
	event("message_stop", map[string]any{})
	return b.String()
}

// responsesCacheSSE reports usage on the terminal response.completed event.
func responsesCacheSSE() string {
	var b strings.Builder
	b.WriteString("data: " + mustJSON(map[string]any{
		"type": "response.output_text.delta", "delta": "ok",
	}) + "\n\n")
	b.WriteString("data: " + mustJSON(map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id": "resp_1", "object": "response", "status": "completed", "model": "m",
			"output": []any{map[string]any{
				"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": "ok"}},
			}},
			"usage": map[string]any{
				"input_tokens": 100, "output_tokens": 4, "total_tokens": 104,
				"input_tokens_details": map[string]any{"cached_tokens": 80},
			},
		},
	}) + "\n\n")
	return b.String()
}

// openAIChatCacheSSE reports usage on a trailing chunk with no choices, which
// the usage-capturing transport reads off the raw stream.
func openAIChatCacheSSE() string {
	var b strings.Builder
	b.WriteString("data: " + mustJSON(map[string]any{
		"id": "chatcmpl-1", "object": "chat.completion.chunk",
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "ok"}}},
	}) + "\n\n")
	b.WriteString("data: " + mustJSON(map[string]any{
		"id": "chatcmpl-1", "object": "chat.completion.chunk", "choices": []any{},
		"usage": map[string]any{
			"prompt_tokens": 100, "completion_tokens": 4, "total_tokens": 104,
			"prompt_tokens_details": map[string]any{"cached_tokens": 80},
		},
	}) + "\n\n")
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

// The acceptance criterion from docs/design/prompt-cache-control.md section 9,
// phase 2: a sequential tool loop over unchanged static input sees a write and
// then a read.
//
// What it protects is the property that makes the write worth buying. The
// static breakpoint has to land on the same bytes both times — if the system
// prompt or the tool definitions moved between turns, the second call would
// write a second entry rather than read the first, and the run would pay twice
// for a saving it never gets.
func TestASequentialLoopWritesThenReads(t *testing.T) {
	var bodies []string
	// The first call writes the prefix; the second reads it back.
	usages := []string{
		`"usage":{"input_tokens":12,"output_tokens":4,"cache_creation_input_tokens":900}`,
		`"usage":{"input_tokens":30,"output_tokens":4,"cache_read_input_tokens":900}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		bodies = append(bodies, string(body))
		usage := usages[min(len(bodies)-1, len(usages)-1)]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"m",` +
			`"content":[{"type":"text","text":"ok"}],` + usage + `}`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, cllm.ProviderAnthropic, server.URL)
	history := []cllm.Message{
		{Role: "system", Content: "you are a careful assistant"},
		{Role: "user", Content: "start"},
	}

	first, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{
		Messages: history, Tools: conformanceTools(), Profile: cllm.ProfileAgentTurn,
	})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if first.Usage.CacheWriteTokens != 900 || first.Usage.CacheReadTokens != 0 {
		t.Errorf("first call usage = %+v, want a write and no read", first.Usage)
	}

	// The next iteration of a loop: the same system prompt and tools, more
	// history behind them.
	history = append(history,
		cllm.Message{Role: "assistant", Content: "ok"},
		cllm.Message{Role: "user", Content: "continue"})
	second, err := client.ChatCompletionBlocking(context.Background(), cllm.Request{
		Messages: history, Tools: conformanceTools(), Profile: cllm.ProfileAgentTurn,
	})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if second.Usage.CacheReadTokens != 900 || second.Usage.CacheWriteTokens != 0 {
		t.Errorf("second call usage = %+v, want a read and no write", second.Usage)
	}

	if len(bodies) != 2 {
		t.Fatalf("upstream saw %d requests, want 2", len(bodies))
	}
	for i, body := range bodies {
		var request map[string]any
		if err := json.Unmarshal([]byte(body), &request); err != nil {
			t.Fatalf("decode request %d: %v", i+1, err)
		}
		system, ok := request["system"].([]any)
		if !ok || len(system) == 0 {
			t.Fatalf("request %d has no system blocks: %s", i+1, body)
		}
		if _, marked := system[len(system)-1].(map[string]any)["cache_control"]; !marked {
			t.Errorf("request %d carries no static breakpoint: %s", i+1, body)
		}
		if _, marked := request["cache_control"]; !marked {
			t.Errorf("request %d carries no rolling breakpoint: %s", i+1, body)
		}
	}
	if cacheablePrefix(t, bodies[0]) != cacheablePrefix(t, bodies[1]) {
		t.Error("the cacheable prefix moved between turns, so the second call writes rather than reads")
	}
}

// cacheablePrefix is everything before the conversation: the parts a reused
// cache entry is keyed on.
func cacheablePrefix(t *testing.T, body string) string {
	t.Helper()
	var request map[string]any
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	prefix, err := json.Marshal(map[string]any{
		"system": request["system"],
		"tools":  request["tools"],
	})
	if err != nil {
		t.Fatalf("encode prefix: %v", err)
	}
	return string(prefix)
}
