package llm

// Reasoning state is the one thing a protocol produces that BuildMax stores
// without understanding: an Anthropic thinking block or an OpenAI Responses
// reasoning item has to come back on the next request, unchanged, or the model
// loses the thread — and with tools in play, some protocols reject the turn
// outright. These tests pin the capture, the replay, and the boundary that
// keeps state from one protocol out of another's request.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

const (
	thinkingText      = "The file is probably in internal/."
	thinkingSignature = "ErUBCkYIBRgCIkAT9lM0signature"
	encryptedReasonig = "gAAAAABn0encrypted-reasoning-payload"
)

// anthropicThinkingBody is a reply that thought before answering.
func anthropicThinkingBody() string {
	return mustJSON(map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "m",
		"content": []any{
			map[string]any{"type": "thinking", "thinking": thinkingText, "signature": thinkingSignature},
			map[string]any{"type": "text", "text": "It is in internal/."},
		},
		"usage": map[string]any{"input_tokens": 5, "output_tokens": 3},
	})
}

// responsesReasoningBody is the same reply in the Responses shape.
func responsesReasoningBody() string {
	return mustJSON(map[string]any{
		"id": "resp_1", "object": "response", "status": "completed", "model": "m",
		"output": []any{
			map[string]any{
				"type":              "reasoning",
				"id":                "rs_1",
				"summary":           []any{},
				"encrypted_content": encryptedReasonig,
			},
			map[string]any{
				"type": "message", "role": "assistant",
				"content": []any{map[string]any{"type": "output_text", "text": "It is in internal/."}},
			},
		},
		"usage": map[string]any{"input_tokens": 5, "output_tokens": 3, "total_tokens": 8},
	})
}

// reasoningUpstream answers every request with the same body and records what
// it was sent, so a test can assert on the replayed request.
func reasoningUpstream(t *testing.T, body string) *upstream {
	t.Helper()
	return newUpstreamWithBody(t, body)
}

func TestAnthropicCapturesThinkingAsProviderState(t *testing.T) {
	up := reasoningUpstream(t, anthropicThinkingBody())
	client := newReasoningClient(t, config.LLMProviderAnthropic, up.server.URL)

	completion, err := client.ChatCompletionBlocking(context.Background(),
		[]cllm.Message{{Role: "user", Content: "where is it"}}, nil)
	if err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}

	// Thinking is not an answer. It must not reach the text a user reads, or a
	// transcript cannot be told apart from the model's conclusion.
	if completion.Content != "It is in internal/." {
		t.Errorf("content = %q, want the text block alone", completion.Content)
	}
	if strings.Contains(completion.Content, thinkingText) {
		t.Error("thinking leaked into the content")
	}
	if !completion.ProviderState.Belongs(config.LLMProviderAnthropic) {
		t.Fatalf("provider state = %+v, want anthropic state", completion.ProviderState)
	}
	if !strings.Contains(string(completion.ProviderState.Data), thinkingSignature) {
		t.Errorf("state %s does not carry the signature", completion.ProviderState.Data)
	}

	// The request asked for thinking in the mode current models accept.
	var request map[string]any
	if err := json.Unmarshal([]byte(up.bodies[0]), &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	thinking, ok := request["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("request %s did not enable thinking", up.bodies[0])
	}
	if thinking["type"] != "adaptive" || thinking["display"] != "omitted" {
		t.Errorf("thinking = %+v, want adaptive with display omitted", thinking)
	}
	output, ok := request["output_config"].(map[string]any)
	if !ok || output["effort"] != config.ReasoningMedium {
		t.Errorf("output_config = %+v, want the configured effort", request["output_config"])
	}
}

func TestAnthropicReplaysThinkingOnTheNextTurn(t *testing.T) {
	up := reasoningUpstream(t, anthropicThinkingBody())
	client := newReasoningClient(t, config.LLMProviderAnthropic, up.server.URL)

	first, err := client.ChatCompletionBlocking(context.Background(),
		[]cllm.Message{{Role: "user", Content: "where is it"}}, nil)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	history := []cllm.Message{
		{Role: "user", Content: "where is it"},
		first.AssistantMessage(),
		{Role: "user", Content: "are you sure"},
	}
	if _, err := client.ChatCompletionBlocking(context.Background(), history, nil); err != nil {
		t.Fatalf("second call: %v", err)
	}

	replayed := up.bodies[1]
	if !strings.Contains(replayed, thinkingSignature) {
		t.Fatalf("replayed request %s dropped the signature", replayed)
	}
	// Order matters: this protocol expects the thinking that produced a turn to
	// come before the turn's visible content.
	if strings.Index(replayed, thinkingSignature) > strings.Index(replayed, "It is in internal/.") {
		t.Error("the thinking block was replayed after the text it preceded")
	}
}

// State minted by one protocol must never reach another. It would be rejected
// there, and a session is portable precisely because the state is not.
func TestForeignReasoningStateIsNotReplayed(t *testing.T) {
	foreign := &cllm.ProviderState{
		Protocol: config.LLMProviderOpenAI,
		Data:     json.RawMessage(`[{"type":"reasoning","encrypted_content":"` + encryptedReasonig + `"}]`),
	}
	for _, p := range []struct {
		provider string
		body     string
	}{
		{config.LLMProviderAnthropic, anthropicThinkingBody()},
		{config.LLMProviderOpenAICompatible, openAIChatBody(reply{text: "ok"})},
	} {
		t.Run(p.provider, func(t *testing.T) {
			up := reasoningUpstream(t, p.body)
			client := newReasoningClient(t, p.provider, up.server.URL)

			if _, err := client.ChatCompletionBlocking(context.Background(), []cllm.Message{
				{Role: "user", Content: "hi"},
				{Role: "assistant", Content: "hello", ProviderState: foreign},
				{Role: "user", Content: "again"},
			}, nil); err != nil {
				t.Fatalf("ChatCompletionBlocking: %v", err)
			}
			if strings.Contains(up.bodies[0], encryptedReasonig) {
				t.Errorf("request %s carried another protocol's reasoning state", up.bodies[0])
			}
		})
	}
}

func TestResponsesCapturesAndReplaysReasoning(t *testing.T) {
	up := reasoningUpstream(t, responsesReasoningBody())
	client := newReasoningClient(t, config.LLMProviderOpenAI, up.server.URL)

	first, err := client.ChatCompletionBlocking(context.Background(),
		[]cllm.Message{{Role: "user", Content: "where is it"}}, nil)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if first.Content != "It is in internal/." {
		t.Errorf("content = %q", first.Content)
	}
	if !first.ProviderState.Belongs(config.LLMProviderOpenAI) {
		t.Fatalf("provider state = %+v, want openai state", first.ProviderState)
	}
	// The encrypted content is the part that cannot be reconstructed, so losing
	// it would make the replay pointless while still looking like it worked.
	if !strings.Contains(string(first.ProviderState.Data), encryptedReasonig) {
		t.Errorf("state %s dropped the encrypted content", first.ProviderState.Data)
	}

	// Without server-side storage there is no response to point at, so the
	// encrypted content has to be requested explicitly.
	if !strings.Contains(up.bodies[0], "reasoning.encrypted_content") {
		t.Errorf("request %s did not ask for encrypted reasoning", up.bodies[0])
	}

	if _, err := client.ChatCompletionBlocking(context.Background(), []cllm.Message{
		{Role: "user", Content: "where is it"},
		first.AssistantMessage(),
		{Role: "user", Content: "are you sure"},
	}, nil); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !strings.Contains(up.bodies[1], encryptedReasonig) {
		t.Errorf("replayed request %s dropped the reasoning item", up.bodies[1])
	}
}

// Reasoning is opt-in. A model that did not ask for it must not have its
// requests changed, because enabling thinking changes cost and, on some
// models, is rejected outright.
func TestReasoningIsOffByDefault(t *testing.T) {
	up := reasoningUpstream(t, anthropicThinkingBody())
	client := newTestClient(t, config.LLMProviderAnthropic, up.server.URL)

	if _, err := client.ChatCompletionBlocking(context.Background(),
		[]cllm.Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	if strings.Contains(up.bodies[0], "thinking") {
		t.Errorf("request %s enabled thinking without being asked", up.bodies[0])
	}
}

func newReasoningClient(t *testing.T, provider, baseURL string) *LLMClient {
	t.Helper()
	client, err := NewClient(Config{
		Provider:      provider,
		APIKey:        "test-key",
		BaseURL:       baseURL,
		Model:         "test-model",
		ContextWindow: 32_000,
		Reasoning:     config.ReasoningMedium,
	})
	if err != nil {
		t.Fatalf("NewClient(%s): %v", provider, err)
	}
	return client
}
