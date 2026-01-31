// Package llm provides LLM client abstractions and implementations (OpenRouter/OpenAI-compatible).
package llm

import (
	"context"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/config"
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
