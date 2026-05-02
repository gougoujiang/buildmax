// Package agent provides the core AI agent logic: task planning, tool invocation, and conversation.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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

// Process runs the agent loop for one user message using the given message buffer:
// appends the user message to the buffer, then runs the loop (LLM call → tool calls if any → append to buffer),
// and returns the final assistant reply along with run statistics.
func (a *Agent) Process(ctx context.Context, buffer MessageBuffer, userMessage string, opts ...ProcessConfigurer) (reply string, stats RunStats, err error) {
	slog.Info("agent process with message buffer started")
	if err := buffer.Append(model.Message{Role: "user", Content: userMessage}); err != nil {
		return "", RunStats{}, err
	}
	slog.Info("user message", "content", userMessage)
	return a.processLoop(ctx, buffer, buildProcessConfig(opts...))
}

// ProcessAfterUserAppended runs the agent loop when the last message in the buffer is already the user message.
// It does not append the user message; use this when the caller (e.g. TUI) has already appended it and refreshed the view.
// Returns an error if the buffer is empty or the last message is not from the user.
func (a *Agent) ProcessAfterUserAppended(ctx context.Context, buffer MessageBuffer, opts ...ProcessConfigurer) (reply string, stats RunStats, err error) {
	msgs := buffer.Messages()
	if len(msgs) == 0 {
		return "", RunStats{}, errors.New("agent: message buffer has no messages")
	}
	last := msgs[len(msgs)-1]
	if last.Role != "user" {
		return "", RunStats{}, fmt.Errorf("agent: last message is %q, not user", last.Role)
	}
	slog.Info("agent process after user appended", "content", last.Content)
	return a.processLoop(ctx, buffer, buildProcessConfig(opts...))
}

// processLoop runs the agent loop using the shared RunLoop.
func (a *Agent) processLoop(ctx context.Context, buffer MessageBuffer, cfg processConfig) (reply string, stats RunStats, err error) {
	return RunLoop(ctx, RunLoopOpts{
		LLMClient:    a.llmClient,
		SystemPrompt: a.systemPrompt,
		Tools:        a.tools,
		MaxIter:      a.maxIter,
		Buffer:       buffer,
		StreamSink:   cfg.streamSink,
	})
}
