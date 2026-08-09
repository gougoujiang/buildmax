// Package llm provides LLM client implementations (OpenRouter/OpenAI-compatible).
package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"

	openai "github.com/sashabaranov/go-openai"
)

// Config holds OpenAI-compatible LLM client settings.
type Config struct {
	APIKey        string
	BaseURL       string
	Model         string
	ContextWindow int           // 0 = no windowing
	CallTimeout   time.Duration // 0 = use DefaultCallTimeoutSecs
}

// LLMClient calls an OpenAI-compatible API (e.g. OpenRouter) and holds configuration for those calls.
type LLMClient struct {
	client        *openai.Client
	model         string
	contextWindow int
	callTimeout   time.Duration
}

// ContextWindow returns the configured context window size (0 = no windowing).
func (c *LLMClient) ContextWindow() int { return c.contextWindow }

// NewClient builds an LLM client from the provided settings.
func NewClient(cfg Config) *LLMClient {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	clientConfig.BaseURL = cfg.BaseURL
	base := clientConfig.HTTPClient
	if base == nil {
		base = http.DefaultClient
	}
	clientConfig.HTTPClient = &usageCaptureHTTPClient{base: base}
	cw := cfg.ContextWindow
	if cw == 0 {
		cw = lookupContextWindow(cfg.Model)
	}
	if cw == 0 {
		cw = config.DefaultContextWindow
	}
	callTimeout := cfg.CallTimeout
	if callTimeout == 0 {
		callTimeout = time.Duration(config.DefaultCallTimeoutSecs) * time.Second
	}
	return &LLMClient{
		client:        openai.NewClientWithConfig(clientConfig),
		model:         cfg.Model,
		contextWindow: cw,
		callTimeout:   callTimeout,
	}
}

func buildChatCompletionRequest(modelName string, messages []cllm.Message, tools []cllm.ToolDef) openai.ChatCompletionRequest {
	openaiMsgs := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, m := range messages {
		openaiMsgs = append(openaiMsgs, toOpenAIMessage(m))
	}
	openaiTools := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		openaiTools = append(openaiTools, toOpenAITool(t))
	}
	return openai.ChatCompletionRequest{
		Model:    modelName,
		Messages: openaiMsgs,
		Tools:    openaiTools,
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

// ChatCompletionBlocking sends messages and tool definitions, returns assistant content, any tool calls, and usage.
// Retries on rate-limit and transient server errors up to maxRetryAttempts times.
// Errors are wrapped with a human-readable classification before being returned.
func (c *LLMClient) ChatCompletionBlocking(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef) (content string, toolCalls []cllm.ToolCall, usage cllm.Usage, err error) {
	for attempt := range maxRetryAttempts {
		if attempt > 0 {
			logRetry(attempt, err)
			if sleepErr := sleepWithContext(ctx, retryBackoff[attempt-1]); sleepErr != nil {
				return "", nil, cllm.Usage{}, wrapLLMError(sleepErr)
			}
		}
		content, toolCalls, usage, err = c.doBlocking(ctx, messages, tools)
		if err == nil || !isRetryableError(err) {
			break
		}
	}
	if err != nil {
		err = wrapLLMError(err)
	}
	return
}

func (c *LLMClient) doBlocking(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef) (string, []cllm.ToolCall, cllm.Usage, error) {
	if c.callTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.callTimeout)
		defer cancel()
	}
	req := buildChatCompletionRequest(c.model, messages, tools)
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", nil, cllm.Usage{}, fmt.Errorf("chat completion: %w", err)
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

// ChatCompletionStreaming sends messages and tool definitions, streams content deltas via onDelta,
// and returns full content, any tool calls, and usage (when the provider sends it in a chunk).
// Retries on transient errors only when no delta has been emitted yet, to avoid duplicate output.
// Errors are wrapped with a human-readable classification before being returned.
// If onDelta is nil, it is not called.
func (c *LLMClient) ChatCompletionStreaming(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef, onDelta func(delta string)) (content string, toolCalls []cllm.ToolCall, usage cllm.Usage, err error) {
	for attempt := range maxRetryAttempts {
		if attempt > 0 {
			logRetry(attempt, err)
			if sleepErr := sleepWithContext(ctx, retryBackoff[attempt-1]); sleepErr != nil {
				return "", nil, cllm.Usage{}, wrapLLMError(sleepErr)
			}
		}
		var deltaEmitted bool
		guardedDelta := func(delta string) {
			deltaEmitted = true
			if onDelta != nil {
				onDelta(delta)
			}
		}
		content, toolCalls, usage, err = c.doStreaming(ctx, messages, tools, guardedDelta)
		if err == nil || !isRetryableError(err) || deltaEmitted {
			break
		}
	}
	if err != nil {
		err = wrapLLMError(err)
	}
	return
}

func (c *LLMClient) doStreaming(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef, onDelta func(string)) (string, []cllm.ToolCall, cllm.Usage, error) {
	if c.callTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.callTimeout)
		defer cancel()
	}
	req := buildChatCompletionRequest(c.model, messages, tools)
	var streamUsage cllm.Usage
	ctx = context.WithValue(ctx, streamUsageKey, &streamUsage)
	stream, err := c.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return "", nil, cllm.Usage{}, fmt.Errorf("chat completion stream: %w", err)
	}
	defer stream.Close()

	var fullContent strings.Builder
	// Accumulate tool calls from stream deltas (index -> id, name, arguments).
	toolCallAccum := make(map[int]*struct {
		id        string
		name      string
		arguments string
	})
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fullContent.String(), nil, cllm.Usage{}, fmt.Errorf("stream recv: %w", err)
		}
		if len(resp.Choices) == 0 {
			continue
		}
		// cllm.Usage is captured from raw SSE by usageCaptureTransport when the provider sends it.
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
			if toolCallAccum[idx] == nil {
				toolCallAccum[idx] = &struct {
					id        string
					name      string
					arguments string
				}{}
			}
			acc := toolCallAccum[idx]
			if tc.ID != "" {
				acc.id = tc.ID
			}
			if tc.Function.Name != "" {
				acc.name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				acc.arguments += tc.Function.Arguments
			}
		}
	}
	// Build final toolCalls in index order.
	maxIdx := -1
	for i := range toolCallAccum {
		if i > maxIdx {
			maxIdx = i
		}
	}
	var toolCalls []cllm.ToolCall
	for i := 0; i <= maxIdx; i++ {
		if acc := toolCallAccum[i]; acc != nil && acc.id != "" {
			toolCalls = append(toolCalls, cllm.ToolCall{
				ID:        acc.id,
				Name:      acc.name,
				Arguments: acc.arguments,
			})
		}
	}
	return fullContent.String(), toolCalls, streamUsage, nil
}

// Ensure *LLMClient implements cllm.LLMClient.
var _ cllm.LLMClient = (*LLMClient)(nil)
