// Package agent provides the core AI agent logic: task planning, tool invocation, and conversation.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/llm"
)

// DefaultMaxIterations is the default cap on agent loop iterations.
const DefaultMaxIterations = 10

// Tool is a capability the agent can invoke by name.
type Tool interface {
	Name() string
	Description() string
	Parameters() any // JSON schema for arguments (e.g. map[string]any)
	Execute(ctx context.Context, args map[string]any) (result string, err error)
}

// LLMCaller can perform chat-with-tools. *llm.Client implements this.
type LLMCaller interface {
	ChatWithTools(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (content string, toolCalls []llm.ToolCall, err error)
}

// Agent runs the agent loop: LLM call → execute tool_calls if any → repeat until final reply.
type Agent struct {
	caller     LLMCaller
	tools      []Tool
	maxIter    int
	toolDefs   []llm.ToolDef // cached from tools for each request
	toolsByName map[string]Tool
}

// Option configures an Agent.
type Option func(*Agent)

// MaxIterations sets the maximum number of loop iterations (default 10).
func MaxIterations(n int) Option {
	return func(a *Agent) {
		a.maxIter = n
	}
}

// NewAgent builds an agent with the given LLM caller and tools.
func NewAgent(caller LLMCaller, tools []Tool, opts ...Option) *Agent {
	toolDefs := make([]llm.ToolDef, 0, len(tools))
	byName := make(map[string]Tool, len(tools))
	for _, t := range tools {
		toolDefs = append(toolDefs, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
		byName[t.Name()] = t
	}
	a := &Agent{
		caller:      caller,
		tools:       tools,
		maxIter:     DefaultMaxIterations,
		toolDefs:    toolDefs,
		toolsByName: byName,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Process runs the agent loop for one user message and returns the final assistant reply.
func (a *Agent) Process(ctx context.Context, userMessage string) (reply string, err error) {
	messages := []llm.Message{
		{Role: "user", Content: userMessage},
	}
	for i := 0; i < a.maxIter; i++ {
		content, toolCalls, err := a.caller.ChatWithTools(ctx, messages, a.toolDefs)
		if err != nil {
			return "", fmt.Errorf("llm call: %w", err)
		}
		if len(toolCalls) == 0 {
			return content, nil
		}
		// Append assistant message (content + tool_calls)
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   content,
			ToolCalls: toolCalls,
		})
		// Execute each tool and append tool result messages
		for _, tc := range toolCalls {
			tool, ok := a.toolsByName[tc.Name]
			if !ok {
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    fmt.Sprintf("error: unknown tool %q", tc.Name),
					ToolCallID: tc.ID,
				})
				continue
			}
			var args map[string]any
			if tc.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
					messages = append(messages, llm.Message{
						Role:       "tool",
						Content:    fmt.Sprintf("error: invalid arguments: %v", err),
						ToolCallID: tc.ID,
					})
					continue
				}
			}
			if args == nil {
				args = make(map[string]any)
			}
			result, err := tool.Execute(ctx, args)
			if err != nil {
				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    fmt.Sprintf("error: %v", err),
					ToolCallID: tc.ID,
				})
				continue
			}
			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}
	return "", errors.New("agent: max iterations exceeded")
}
