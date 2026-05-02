package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"buildmax/internal/core/model"
)

// RunStats holds statistics collected during a single agent run.
type RunStats struct {
	ToolCalls        int // total number of individual tool invocations
	PromptTokens     int // accumulated prompt tokens from LLM calls
	CompletionTokens int // accumulated completion tokens from LLM calls
}

// MessageHistory is the minimal interface for the agent loop: read the conversation so far and append one message.
// The loop uses it so the same logic works with in-memory session or DB-backed conversation.
type MessageHistory interface {
	Messages() []model.Message
	Append(m model.Message) error
}

// RunLoopOpts configures a single run of the shared agent loop (used by both CLI agent and conversation).
type RunLoopOpts struct {
	LLMClient    model.LLMClient
	SystemPrompt string
	ToolRegistry model.ToolRegistry
	MaxIter      int
	History      MessageHistory
	StreamSink   model.StreamSink
}

// RunLoop runs the LLM loop once: build messages from history, call LLM, handle tool_calls, append to history, repeat until final reply.
// It is used by Agent.processLoop (with session history) and by conversation.Run (with DB-backed history).
func RunLoop(ctx context.Context, opts RunLoopOpts) (reply string, stats RunStats, err error) {
	var totalToolCalls, totalPrompt, totalCompletion int
	for i := 0; i < opts.MaxIter; i++ {
		slog.Debug("agent run loop iteration", "iter", i+1, "max", opts.MaxIter)
		messages := append([]model.Message{{Role: "system", Content: opts.SystemPrompt}}, opts.History.Messages()...)
		var content string
		var toolCalls []model.ToolCall
		var usage model.Usage
		if opts.StreamSink != nil {
			content, toolCalls, usage, err = opts.LLMClient.ChatCompletionStreaming(ctx, messages, opts.ToolRegistry.GetDefs(), opts.StreamSink.OnDelta)
		} else {
			content, toolCalls, usage, err = opts.LLMClient.ChatCompletionBlocking(ctx, messages, opts.ToolRegistry.GetDefs())
		}
		if err != nil {
			slog.Error("LLM call failed", "err", err)
			return "", RunStats{ToolCalls: totalToolCalls, PromptTokens: totalPrompt, CompletionTokens: totalCompletion}, fmt.Errorf("llm call: %w", err)
		}
		totalPrompt += usage.PromptTokens
		totalCompletion += usage.CompletionTokens
		if len(toolCalls) == 0 {
			slog.Debug("agent reply", "content", content)
			if err := opts.History.Append(model.Message{Role: "assistant", Content: content}); err != nil {
				return "", RunStats{ToolCalls: totalToolCalls, PromptTokens: totalPrompt, CompletionTokens: totalCompletion}, err
			}
			return content, RunStats{ToolCalls: totalToolCalls, PromptTokens: totalPrompt, CompletionTokens: totalCompletion}, nil
		}
		slog.Debug("tool calls", "n", len(toolCalls), "content", content, "calls", toolCallsSummary(toolCalls))
		if err := opts.History.Append(model.Message{Role: "assistant", Content: content, ToolCalls: toolCalls}); err != nil {
			return "", RunStats{ToolCalls: totalToolCalls, PromptTokens: totalPrompt, CompletionTokens: totalCompletion}, err
		}
		for _, tc := range toolCalls {
			tool := opts.ToolRegistry.Lookup(tc.Name)
			var result string
			if tool == nil {
				result = fmt.Sprintf("error: unknown tool %q", tc.Name)
			} else {
				result = ExecuteTool(ctx, tool, tc)
			}
			if len(result) > 500 {
				slog.Debug("tool result", "tool", tc.Name, "content", result[:500]+"...")
			} else {
				slog.Debug("tool result", "tool", tc.Name, "content", result)
			}
			if err := opts.History.Append(model.Message{Role: "tool", Content: result, ToolCallID: tc.ID}); err != nil {
				return "", RunStats{ToolCalls: totalToolCalls, PromptTokens: totalPrompt, CompletionTokens: totalCompletion}, err
			}
			totalToolCalls++
		}
	}
	slog.Warn("agent max iterations exceeded")
	return "", RunStats{ToolCalls: totalToolCalls, PromptTokens: totalPrompt, CompletionTokens: totalCompletion}, errors.New("agent: max iterations exceeded")
}

// ExecuteTool runs one tool call: parses tc.Arguments, calls tool.Execute, and returns the result or an error string.
func ExecuteTool(ctx context.Context, t model.Tool, tc model.ToolCall) string {
	var args map[string]any
	if tc.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			return fmt.Sprintf("error: invalid arguments: %v", err)
		}
	}
	if args == nil {
		args = make(map[string]any)
	}
	result, err := t.Execute(ctx, args)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return result
}

func toolCallsSummary(calls []model.ToolCall) []string {
	s := make([]string, 0, len(calls))
	for _, tc := range calls {
		args := tc.Arguments
		if len(args) > 80 {
			args = args[:80] + "..."
		}
		s = append(s, tc.Name+": "+strings.TrimSpace(args))
	}
	return s
}
