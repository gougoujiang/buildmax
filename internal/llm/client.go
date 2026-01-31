// Package llm provides LLM client abstractions and implementations (OpenRouter/OpenAI-compatible).
package llm

import (
	"context"
	"fmt"

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

// Chat sends a single user message and returns the assistant reply.
func (c *Client) Chat(ctx context.Context, userMessage string) (string, error) {
	req := openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: userMessage},
		},
	}
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return resp.Choices[0].Message.Content, nil
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
					ID: tc.ID,
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
