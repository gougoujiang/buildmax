// Package agent provides the core AI agent logic: task planning, tool invocation, and conversation.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"buildmax/internal/llm"
	"buildmax/internal/session"
)

// DefaultMaxIterations is the default cap on agent loop iterations.
const DefaultMaxIterations = 10

// DefaultSystemPrompt is the default system message sent at the start of every agent run.
// It declares the assistant role and behavioral guidelines so the LLM behaves consistently.
const DefaultSystemPrompt = `You are BuildMax, an interactive CLI tool that helps users with software engineering tasks. Use the instructions below and the tools available to you to assist the user.

# Professional objectivity
Prioritize technical accuracy and truthfulness over validating the user's beliefs. Focus on facts and problem-solving, providing direct, objective technical info without any unnecessary superlatives, praise, or emotional validation. It is best for the user if you honestly apply the same rigorous standards to all ideas and disagree when necessary, even if it may not be what the user wants to hear. Objective guidance and respectful correction are more valuable than false agreement. Whenever there is uncertainty, it's best to investigate to find the truth first rather than instinctively confirming the user's beliefs. Avoid using over-the-top validation or excessive praise when responding to users such as "You're absolutely right" or similar phrases.`

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
	caller      LLMCaller
	tools       []Tool
	maxIter     int
	toolDefs    []llm.ToolDef // cached from tools for each request
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

// Process runs the agent loop for one user message using the given session:
// appends the user message to the session, then runs the loop (LLM call → tool calls if any → append to session),
// and returns the final assistant reply.
func (a *Agent) Process(ctx context.Context, sess *session.Session, userMessage string) (reply string, err error) {
	slog.Info("agent process with session started")
	sess.Append(llm.Message{Role: "user", Content: userMessage})
	slog.Info("user message", "content", userMessage)
	return a.processLoop(ctx, sess)
}

// ProcessAfterUserAppended runs the agent loop when the last message in the session is already the user message.
// It does not append the user message; use this when the caller (e.g. TUI) has already appended it and refreshed the view.
// Returns an error if the session is empty or the last message is not from the user.
func (a *Agent) ProcessAfterUserAppended(ctx context.Context, sess *session.Session) (reply string, err error) {
	msgs := sess.Messages()
	if len(msgs) == 0 {
		return "", errors.New("agent: session has no messages")
	}
	last := msgs[len(msgs)-1]
	if last.Role != "user" {
		return "", fmt.Errorf("agent: last message is %q, not user", last.Role)
	}
	slog.Info("agent process after user appended", "content", last.Content)
	return a.processLoop(ctx, sess)
}

// processLoop runs the LLM loop: build messages (system + session), call LLM, handle tool_calls, append to session, repeat until final reply.
func (a *Agent) processLoop(ctx context.Context, sess *session.Session) (reply string, err error) {
	for i := 0; i < a.maxIter; i++ {
		ctx = session.CtxWithSessionID(ctx, sess.ID())
		slog.Debug("agent iteration", "iter", i+1, "max", a.maxIter)
		messages := append([]llm.Message{{Role: "system", Content: DefaultSystemPrompt}}, sess.Messages()...)
		content, toolCalls, err := a.caller.ChatWithTools(ctx, messages, a.toolDefs)
		if err != nil {
			slog.Error("LLM call failed", "err", err)
			return "", fmt.Errorf("llm call: %w", err)
		}
		if len(toolCalls) == 0 {
			slog.Debug("agent reply", "content", content)
			sess.Append(llm.Message{Role: "assistant", Content: content})
			return content, nil
		}
		slog.Debug("tool calls", "n", len(toolCalls), "content", content, "calls", toolCallsSummary(toolCalls))
		sess.Append(llm.Message{
			Role:      "assistant",
			Content:   content,
			ToolCalls: toolCalls,
		})
		for _, tc := range toolCalls {
			processOneToolCall(ctx, a, sess, tc)
		}
	}
	slog.Warn("agent max iterations exceeded")
	return "", errors.New("agent: max iterations exceeded")
}

// processOneToolCall resolves the tool by name, parses arguments, executes, and appends
// exactly one tool message (result or error) to the session. The assistant message with
// toolCalls is already appended by processLoop before the loop.
func processOneToolCall(ctx context.Context, a *Agent, sess *session.Session, tc llm.ToolCall) {
	tool, ok := a.toolsByName[tc.Name]
	if !ok {
		sess.Append(llm.Message{
			Role:       "tool",
			Content:    fmt.Sprintf("error: unknown tool %q", tc.Name),
			ToolCallID: tc.ID,
		})
		return
	}
	var args map[string]any
	if tc.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			sess.Append(llm.Message{
				Role:       "tool",
				Content:    fmt.Sprintf("error: invalid arguments: %v", err),
				ToolCallID: tc.ID,
			})
			return
		}
	}
	if args == nil {
		args = make(map[string]any)
	}
	result, err := tool.Execute(ctx, args)
	if err != nil {
		slog.Debug("tool result", "tool", tc.Name, "error", err.Error())
		sess.Append(llm.Message{
			Role:       "tool",
			Content:    fmt.Sprintf("error: %v", err),
			ToolCallID: tc.ID,
		})
		return
	}
	resultPreview := result
	if len(resultPreview) > 500 {
		resultPreview = resultPreview[:500] + "..."
	}
	slog.Debug("tool result", "tool", tc.Name, "content", resultPreview)
	sess.Append(llm.Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: tc.ID,
	})
}

// toolCallsSummary returns a short summary of tool calls for logging.
func toolCallsSummary(calls []llm.ToolCall) []string {
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
