package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"

	openai "github.com/sashabaranov/go-openai"
)

// openAIChatAdapter speaks OpenAI Chat Completions. It is the protocol served
// by OpenRouter, LiteLLM, vLLM, and local inference servers, and the default
// for a model entry that names no provider.
type openAIChatAdapter struct {
	client    *openai.Client
	model     string
	maxTokens int
}

func newOpenAIChatAdapter(cfg Config) *openAIChatAdapter {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		clientConfig.BaseURL = cfg.BaseURL
	}
	var base httpDoer = clientConfig.HTTPClient
	if cfg.HTTPClient != nil {
		base = cfg.HTTPClient
	}
	if base == nil {
		base = http.DefaultClient
	}
	// This protocol's library does not surface usage from stream chunks, so the
	// transport reads it off the raw SSE. The workaround stays here rather than
	// in the shared layer: the other two protocols report usage in their own
	// event streams and need nothing like it.
	clientConfig.HTTPClient = &usageCaptureHTTPClient{base: base}
	return &openAIChatAdapter{
		client:    openai.NewClientWithConfig(clientConfig),
		model:     cfg.Model,
		maxTokens: cfg.MaxTokens,
	}
}

func (a *openAIChatAdapter) name() string { return config.LLMProviderOpenAICompatible }

func (a *openAIChatAdapter) buildRequest(messages []cllm.Message, tools []cllm.ToolDef) openai.ChatCompletionRequest {
	openaiMsgs := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, m := range messages {
		openaiMsgs = append(openaiMsgs, toOpenAIMessage(m))
	}
	openaiTools := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		openaiTools = append(openaiTools, toOpenAITool(t))
	}
	return openai.ChatCompletionRequest{
		Model:     a.model,
		Messages:  openaiMsgs,
		Tools:     openaiTools,
		MaxTokens: a.maxTokens,
	}
}

func toOpenAIMessage(m cllm.Message) openai.ChatCompletionMessage {
	msg := openai.ChatCompletionMessage{
		Role:       m.Role,
		Content:    m.Content,
		ToolCallID: m.ToolCallID,
	}
	if len(m.ToolCalls) > 0 {
		msg.ToolCalls = make([]openai.ToolCall, 0, len(m.ToolCalls))
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, openai.ToolCall{
				ID:   tc.ID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
	}
	return msg
}

func toOpenAITool(t cllm.ToolDef) openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		},
	}
}

func toToolCalls(toolCalls []openai.ToolCall) []cllm.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	out := make([]cllm.ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		out = append(out, cllm.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return out
}

// openAIAPIError converts a go-openai failure into the neutral error the shared
// retry and classification logic reads. Both OpenAI protocols use it, because
// both are served by the same library.
func openAIAPIError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		return &apiError{status: apiErr.HTTPStatusCode, message: apiErr.Message, err: err}
	}
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) {
		return &apiError{status: reqErr.HTTPStatusCode, err: err}
	}
	return &apiError{err: err}
}

func (a *openAIChatAdapter) blocking(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef) (string, []cllm.ToolCall, cllm.Usage, error) {
	resp, err := a.client.CreateChatCompletion(ctx, a.buildRequest(messages, tools))
	if err != nil {
		return "", nil, cllm.Usage{}, fmt.Errorf("chat completion: %w", openAIAPIError(err))
	}
	if len(resp.Choices) == 0 {
		return "", nil, cllm.Usage{}, fmt.Errorf("no choices in response")
	}
	msg := resp.Choices[0].Message
	usage := cllm.Usage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}
	return msg.Content, toToolCalls(msg.ToolCalls), usage, nil
}

func (a *openAIChatAdapter) streaming(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef, onDelta func(string)) (string, []cllm.ToolCall, cllm.Usage, error) {
	var streamUsage cllm.Usage
	ctx = context.WithValue(ctx, streamUsageKey, &streamUsage)
	stream, err := a.client.CreateChatCompletionStream(ctx, a.buildRequest(messages, tools))
	if err != nil {
		return "", nil, cllm.Usage{}, fmt.Errorf("chat completion stream: %w", openAIAPIError(err))
	}
	defer func() { _ = stream.Close() }()

	var fullContent strings.Builder
	accum := newToolCallAccumulator()
	for {
		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fullContent.String(), nil, cllm.Usage{}, fmt.Errorf("stream recv: %w", openAIAPIError(err))
		}
		if len(resp.Choices) == 0 {
			continue
		}
		// Usage is captured from raw SSE by usageCaptureTransport when the provider sends it.
		delta := resp.Choices[0].Delta
		if delta.Content != "" {
			fullContent.WriteString(delta.Content)
			if onDelta != nil {
				onDelta(delta.Content)
			}
		}
		for _, tc := range delta.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			accum.add(idx, tc.ID, tc.Function.Name, tc.Function.Arguments)
		}
	}
	return fullContent.String(), accum.toolCalls(), streamUsage, nil
}
