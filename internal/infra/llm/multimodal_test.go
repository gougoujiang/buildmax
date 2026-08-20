package llm

// Non-text content is where the three protocols diverge most: Anthropic takes an
// image inside a tool result, and neither OpenAI protocol does. The canonical
// message says the same thing in all three cases, so these tests pin what each
// adapter turns it into — and what happens when the model cannot read images at
// all, which is the common case and must not become a rejected request.

import (
	"context"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// A one-pixel PNG, so a fixture carries a real image without carrying a payload.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// imageToolHistory is a turn whose tool result came back as text plus a picture.
func imageToolHistory() []cllm.Message {
	return []cllm.Message{
		{Role: "user", Content: "screenshot the page"},
		{Role: "assistant", ToolCalls: []cllm.ToolCall{
			{ID: "call_1", Name: "CallMCPTool", Arguments: `{}`},
		}},
		{
			Role:       "tool",
			ToolCallID: "call_1",
			Content:    "(image: image/png, 70 B)",
			Parts: []cllm.ContentPart{{
				Type: cllm.ContentPartImage, MediaType: "image/png", Data: onePixelPNG,
			}},
		},
	}
}

func newVisionClient(t *testing.T, provider, baseURL string) *LLMClient {
	t.Helper()
	client, err := NewClient(Config{
		Provider: provider, APIKey: "test-key", BaseURL: baseURL,
		Model: "test-model", ContextWindow: 32_000, Vision: true,
	})
	if err != nil {
		t.Fatalf("NewClient(%s): %v", provider, err)
	}
	return client
}

// Every protocol must get the image across somehow, and every one must keep the
// text that describes it.
func TestEveryProtocolSendsAToolImage(t *testing.T) {
	for _, p := range protocols {
		t.Run(p.provider, func(t *testing.T) {
			up := newUpstreamWithBody(t, p.blocking(reply{text: "ok"}))
			client := newVisionClient(t, p.provider, up.server.URL)

			if _, err := client.ChatCompletionBlocking(
				context.Background(), imageToolHistory(), conformanceTools()); err != nil {
				t.Fatalf("ChatCompletionBlocking: %v", err)
			}
			body := up.bodies[0]
			if !strings.Contains(body, onePixelPNG) {
				t.Errorf("request %s did not carry the image", body)
			}
			if !strings.Contains(body, "(image: image/png") {
				t.Error("the text describing the image was dropped")
			}
		})
	}
}

// Anthropic takes the image inside the tool result, where it belongs. The other
// two cannot, so they must not try.
func TestToolImagePlacementFollowsTheProtocol(t *testing.T) {
	tests := []struct {
		provider     string
		wantFollowUp bool
	}{
		{config.LLMProviderAnthropic, false},
		{config.LLMProviderOpenAICompatible, true},
		{config.LLMProviderOpenAI, true},
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			var p protocol
			for _, candidate := range protocols {
				if candidate.provider == tc.provider {
					p = candidate
				}
			}
			up := newUpstreamWithBody(t, p.blocking(reply{text: "ok"}))
			client := newVisionClient(t, tc.provider, up.server.URL)

			if _, err := client.ChatCompletionBlocking(
				context.Background(), imageToolHistory(), conformanceTools()); err != nil {
				t.Fatalf("ChatCompletionBlocking: %v", err)
			}
			gotFollowUp := strings.Contains(up.bodies[0], imageFollowUpPreamble)
			if gotFollowUp != tc.wantFollowUp {
				t.Errorf("follow-up user turn = %v, want %v; request was %s",
					gotFollowUp, tc.wantFollowUp, up.bodies[0])
			}
		})
	}
}

// This is the default, and the one that must not break: a model that cannot read
// images gets the text and nothing else. Sending the image anyway would fail the
// call rather than be ignored.
func TestWithoutVisionAnImageIsDescribedNotSent(t *testing.T) {
	for _, p := range protocols {
		t.Run(p.provider, func(t *testing.T) {
			up := newUpstreamWithBody(t, p.blocking(reply{text: "ok"}))
			client := newTestClient(t, p.provider, up.server.URL)

			if _, err := client.ChatCompletionBlocking(
				context.Background(), imageToolHistory(), conformanceTools()); err != nil {
				t.Fatalf("ChatCompletionBlocking: %v", err)
			}
			if strings.Contains(up.bodies[0], onePixelPNG) {
				t.Errorf("request %s sent an image to a model that cannot read one", up.bodies[0])
			}
			if !strings.Contains(up.bodies[0], "(image: image/png") {
				t.Error("the text describing the image was dropped too, leaving nothing")
			}
		})
	}
}

// An image on a user turn is the shape every protocol supports directly.
func TestEveryProtocolSendsAUserImage(t *testing.T) {
	history := []cllm.Message{{
		Role:    "user",
		Content: "what is in this picture",
		Parts: []cllm.ContentPart{{
			Type: cllm.ContentPartImage, MediaType: "image/png", Data: onePixelPNG,
		}},
	}}
	for _, p := range protocols {
		t.Run(p.provider, func(t *testing.T) {
			up := newUpstreamWithBody(t, p.blocking(reply{text: "ok"}))
			client := newVisionClient(t, p.provider, up.server.URL)

			if _, err := client.ChatCompletionBlocking(context.Background(), history, nil); err != nil {
				t.Fatalf("ChatCompletionBlocking: %v", err)
			}
			if !strings.Contains(up.bodies[0], onePixelPNG) {
				t.Errorf("request %s did not carry the image", up.bodies[0])
			}
			if !strings.Contains(up.bodies[0], "what is in this picture") {
				t.Error("the user's question was dropped")
			}
		})
	}
}
