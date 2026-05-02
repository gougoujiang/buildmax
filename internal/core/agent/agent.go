// Package agent provides the core AI agent logic: task planning, tool invocation, and conversation.
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

// DefaultMaxIterations is the default cap on agent loop iterations.
const DefaultMaxIterations = 200

// Agent runs the agent loop: LLM call → execute tool_calls if any → repeat until final reply.
type Agent struct {
	llmClient    model.LLMClient
	maxIter      int
	systemPrompt string
	tools        model.ToolRegistry
}

// AgentConfigurer configures an Agent.
type AgentConfigurer func(*Agent)

// WithMaxIterations sets the maximum number of loop iterations.
func WithMaxIterations(n int) AgentConfigurer {
	return func(a *Agent) {
		a.maxIter = n
	}
}

// WithSystemPrompt overrides the default system prompt used by the agent loop.
// This allows sub-agents to use role-specific prompts instead of DefaultSystemPrompt.
func WithSystemPrompt(prompt string) AgentConfigurer {
	return func(a *Agent) {
		a.systemPrompt = prompt
	}
}

// NewAgent builds an agent with the given LLM client and tools.
func NewAgent(llmClient model.LLMClient, tools model.ToolRegistry, opts ...AgentConfigurer) *Agent {
	a := &Agent{
		llmClient:    llmClient,
		maxIter:      DefaultMaxIterations,
		systemPrompt: DefaultSystemPrompt,
		tools:        tools,
	}
	configureAgent(a, opts...)
	return a
}

func configureAgent(a *Agent, opts ...AgentConfigurer) {
	for _, opt := range opts {
		opt(a)
	}
}

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

// processConfig holds per-call options for Process/ProcessAfterUserAppended.
type processConfig struct {
	streamSink model.StreamSink
}

// ProcessConfigurer configures a single agent run (e.g. streaming).
type ProcessConfigurer func(*processConfig)

// WithStreamSink sets the sink that receives content deltas during the LLM stream.
func WithStreamSink(sink model.StreamSink) ProcessConfigurer {
	return func(c *processConfig) {
		c.streamSink = sink
	}
}

func buildProcessConfig(opts ...ProcessConfigurer) processConfig {
	var cfg processConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// Process runs the agent loop for one user message using the given message history:
// appends the user message, then runs the loop (LLM call → tool calls if any → append assistant/tool messages),
// and returns the final assistant reply along with run statistics.
func (a *Agent) Process(ctx context.Context, history MessageHistory, userMessage string, opts ...ProcessConfigurer) (reply string, stats RunStats, err error) {
	slog.Info("agent process with message history started")
	if err := history.Append(model.Message{Role: "user", Content: userMessage}); err != nil {
		return "", RunStats{}, err
	}
	slog.Info("user message", "content", userMessage)
	return a.processLoop(ctx, history, buildProcessConfig(opts...))
}

// ProcessAfterUserAppended runs the agent loop when the last message in history is already the user message.
// It does not append the user message; use this when the caller (e.g. TUI) has already appended it and refreshed the view.
// Returns an error if history is empty or the last message is not from the user.
func (a *Agent) ProcessAfterUserAppended(ctx context.Context, history MessageHistory, opts ...ProcessConfigurer) (reply string, stats RunStats, err error) {
	msgs := history.Messages()
	if len(msgs) == 0 {
		return "", RunStats{}, errors.New("agent: message history is empty")
	}
	last := msgs[len(msgs)-1]
	if last.Role != "user" {
		return "", RunStats{}, fmt.Errorf("agent: last message is %q, not user", last.Role)
	}
	slog.Info("agent process after user appended", "content", last.Content)
	return a.processLoop(ctx, history, buildProcessConfig(opts...))
}

// processLoop runs the agent loop using the shared RunLoop.
func (a *Agent) processLoop(ctx context.Context, history MessageHistory, cfg processConfig) (reply string, stats RunStats, err error) {
	return RunLoop(ctx, RunLoopOpts{
		LLMClient:    a.llmClient,
		SystemPrompt: a.systemPrompt,
		ToolRegistry: a.tools,
		MaxIter:      a.maxIter,
		History:      history,
		StreamSink:   cfg.streamSink,
	})
}
