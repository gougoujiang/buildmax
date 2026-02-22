// Package llm provides LLM client abstractions and implementations (OpenRouter/OpenAI-compatible).
package llm

import (
	"context"
	"fmt"
	"io"
	"strings"

	"buildmax/internal/config"

	openai "github.com/sashabaranov/go-openai"
)

// Client calls an OpenAI-compatible API (e.g. OpenRouter).
type Client struct {
	client *openai.Client
	model  string
}

// NewClient builds an LLM client from config.
func NewClient(cfg config.LLM) *Client {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	clientConfig.BaseURL = cfg.BaseURL
	return &Client{
		client: openai.NewClientWithConfig(clientConfig),
		model:  cfg.Model,
	}
}

// ChatWithTools sends messages and tool definitions, returns assistant content and any tool calls.
func (c *Client) ChatWithTools(ctx context.Context, messages []Message, tools []ToolDef) (content string, toolCalls []ToolCall, err error) {
	openaiMsgs := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, m := range messages {
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
		openaiMsgs = append(openaiMsgs, msg)
	}
	openaiTools := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		openaiTools = append(openaiTools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	req := openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: openaiMsgs,
		Tools:    openaiTools,
	}
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", nil, fmt.Errorf("no choices in response")
	}
	msg := resp.Choices[0].Message
	content = msg.Content
	if len(msg.ToolCalls) > 0 {
		toolCalls = make([]ToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			toolCalls = append(toolCalls, ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
	}
	return content, toolCalls, nil
}

// ChatWithToolsStream sends messages and tool definitions, streams content deltas via onDelta,
// and returns full content and any tool calls at stream end. If onDelta is nil, it is not called.
func (c *Client) ChatWithToolsStream(ctx context.Context, messages []Message, tools []ToolDef, onDelta func(delta string)) (content string, toolCalls []ToolCall, err error) {
	openaiMsgs := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, m := range messages {
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
		openaiMsgs = append(openaiMsgs, msg)
	}
	openaiTools := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		openaiTools = append(openaiTools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}
	req := openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: openaiMsgs,
		Tools:    openaiTools,
	}
	stream, err := c.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("chat completion stream: %w", err)
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
			return fullContent.String(), nil, fmt.Errorf("stream recv: %w", err)
		}
		if len(resp.Choices) == 0 {
			continue
		}
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
	return fullContent.String(), toolCalls, nil
}
