// Package llm provides LLM client abstractions and implementations (OpenRouter/OpenAI-compatible).
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"buildmax/internal/config"

	openai "github.com/sashabaranov/go-openai"
)

// contextKey type for stream usage context value.
type contextKey struct{}

var streamUsageKey = &contextKey{}

// Client calls an OpenAI-compatible API (e.g. OpenRouter).
type Client struct {
	client *openai.Client
	model  string
}

// NewClient builds an LLM client from config.
func NewClient(cfg config.LLM) *Client {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	clientConfig.BaseURL = cfg.BaseURL
	base := clientConfig.HTTPClient
	if base == nil {
		base = http.DefaultClient
	}
	transport := base.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	clientConfig.HTTPClient = &http.Client{
		Transport: &usageCaptureTransport{base: transport},
		Timeout:   base.Timeout,
	}
	return &Client{
		client: openai.NewClientWithConfig(clientConfig),
		model:  cfg.Model,
	}
}

func buildChatCompletionRequest(model string, messages []Message, tools []ToolDef) openai.ChatCompletionRequest {
	openaiMsgs := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, m := range messages {
		openaiMsgs = append(openaiMsgs, toOpenAIMessage(m))
	}
	openaiTools := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		openaiTools = append(openaiTools, toOpenAITool(t))
	}
	return openai.ChatCompletionRequest{
		Model:    model,
		Messages: openaiMsgs,
		Tools:    openaiTools,
	}
}

func toOpenAIMessage(m Message) openai.ChatCompletionMessage {
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

func toOpenAITool(t ToolDef) openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		},
	}
}

func toToolCalls(toolCalls []openai.ToolCall) []ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		out = append(out, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return out
}

// ChatWithTools sends messages and tool definitions, returns assistant content, any tool calls, and usage.
func (c *Client) ChatWithTools(ctx context.Context, messages []Message, tools []ToolDef) (content string, toolCalls []ToolCall, usage Usage, err error) {
	req := buildChatCompletionRequest(c.model, messages, tools)
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", nil, Usage{}, fmt.Errorf("chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", nil, Usage{}, fmt.Errorf("no choices in response")
	}
	msg := resp.Choices[0].Message
	content = msg.Content
	toolCalls = toToolCalls(msg.ToolCalls)
	usage = Usage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}
	return content, toolCalls, usage, nil
}

// ChatWithToolsStream sends messages and tool definitions, streams content deltas via onDelta,
// and returns full content, any tool calls, and usage (when the provider sends it in a chunk). If onDelta is nil, it is not called.
func (c *Client) ChatWithToolsStream(ctx context.Context, messages []Message, tools []ToolDef, onDelta func(delta string)) (content string, toolCalls []ToolCall, usage Usage, err error) {
	req := buildChatCompletionRequest(c.model, messages, tools)
	var streamUsage Usage
	ctx = context.WithValue(ctx, streamUsageKey, &streamUsage)
	stream, err := c.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return "", nil, Usage{}, fmt.Errorf("chat completion stream: %w", err)
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
			return fullContent.String(), nil, Usage{}, fmt.Errorf("stream recv: %w", err)
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
	for i := 0; i <= maxIdx; i++ {
		if acc := toolCallAccum[i]; acc != nil && acc.id != "" {
			toolCalls = append(toolCalls, ToolCall{
				ID:        acc.id,
				Name:      acc.name,
				Arguments: acc.arguments,
			})
		}
	}
	return fullContent.String(), toolCalls, streamUsage, nil
}

// streamRespUsage extracts usage from a stream response when the provider includes it.
// go-openai ChatCompletionStreamResponse does not currently expose Usage; capture is done via usageCaptureTransport.
func streamRespUsage(resp openai.ChatCompletionStreamResponse) Usage {
	return Usage{}
}

// usageCaptureTransport wraps the response body for SSE streams to capture usage from raw chunks.
type usageCaptureTransport struct {
	base http.RoundTripper
}

func (t *usageCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		return resp, nil
	}
	if v := req.Context().Value(streamUsageKey); v != nil {
		if usage, ok := v.(*Usage); ok && usage != nil {
			resp.Body = &usageCaptureReader{body: resp.Body, usage: usage}
		}
	}
	return resp, nil
}

// usageCaptureReader tees the stream and parses SSE "data:" lines for usage.
type usageCaptureReader struct {
	body  io.ReadCloser
	usage *Usage
	buf   []byte
	mu    sync.Mutex
}

func (r *usageCaptureReader) Read(p []byte) (n int, err error) {
	n, err = r.body.Read(p)
	if n > 0 {
		r.mu.Lock()
		r.buf = append(r.buf, p[:n]...)
		r.parseUsageLocked()
		r.mu.Unlock()
	}
	return n, err
}

func (r *usageCaptureReader) Close() error {
	return r.body.Close()
}

func (r *usageCaptureReader) parseUsageLocked() {
	const dataPrefix = "data: "
	for {
		idx := bytes.Index(r.buf, []byte("\n\n"))
		if idx < 0 {
			break
		}
		event := r.buf[:idx]
		r.buf = r.buf[idx+2:]
		lines := bytes.Split(event, []byte("\n"))
		for _, line := range lines {
			line = bytes.TrimSpace(line)
			if !bytes.HasPrefix(line, []byte(dataPrefix)) {
				continue
			}
			jsonBytes := line[len(dataPrefix):]
			if string(jsonBytes) == "[DONE]" {
				continue
			}
			var obj struct {
				Usage *struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage,omitempty"`
			}
			if json.Unmarshal(jsonBytes, &obj) != nil || obj.Usage == nil {
				continue
			}
			r.usage.PromptTokens = obj.Usage.PromptTokens
			r.usage.CompletionTokens = obj.Usage.CompletionTokens
			r.usage.TotalTokens = obj.Usage.TotalTokens
		}
	}
}

// Ensure *Client implements LLMCaller.
var _ LLMCaller = (*Client)(nil)
