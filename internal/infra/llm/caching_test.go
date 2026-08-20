package llm

// Prompt caching splits three ways: Anthropic needs breakpoints placed in the
// request, and both OpenAI protocols cache on their own and only report it. The
// reporting has to agree anyway, because a spend report reads one shape.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

func newCachingClient(t *testing.T, provider, baseURL string) *LLMClient {
	t.Helper()
	client, err := NewClient(Config{
		Provider: provider, APIKey: "test-key", BaseURL: baseURL,
		Model: "test-model", ContextWindow: 32_000, PromptCache: true,
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
	client := newCachingClient(t, config.LLMProviderAnthropic, up.server.URL)

	if _, err := client.ChatCompletionBlocking(
		context.Background(), conformanceHistory(), conformanceTools()); err != nil {
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

func TestCachingIsOffByDefault(t *testing.T) {
	up := newUpstreamWithBody(t, anthropicBody(reply{text: "ok"}))
	client := newTestClient(t, config.LLMProviderAnthropic, up.server.URL)

	if _, err := client.ChatCompletionBlocking(
		context.Background(), conformanceHistory(), conformanceTools()); err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	if strings.Contains(up.bodies[0], "cache_control") {
		t.Errorf("request %s cached without being asked; caching changes what a call costs",
			up.bodies[0])
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
			provider: config.LLMProviderAnthropic,
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
			provider: config.LLMProviderOpenAI,
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
			provider: config.LLMProviderOpenAICompatible,
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
				[]cllm.Message{{Role: "user", Content: "hi"}}, nil)
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
		Provider: config.LLMProviderAnthropic, APIKey: "k", Model: "m", Reasoning: "maximum",
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
