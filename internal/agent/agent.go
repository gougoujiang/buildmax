// Package agent provides the core AI agent logic: task planning, tool invocation, and conversation.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"buildmax/internal/llm"
	"buildmax/internal/session"
	"buildmax/internal/core"
)

// DefaultMaxIterations is the default cap on agent loop iterations.
const DefaultMaxIterations = 200

// DefaultSystemPrompt is the default system message sent at the start of every agent run.
// It declares the assistant role and behavioral guidelines so the LLM behaves consistently.
const DefaultSystemPrompt = `You are BuildMax, an interactive CLI tool that helps users with software engineering tasks. Use the instructions below and the tools available to you to assist the user.

# Professional objectivity
Prioritize technical accuracy and truthfulness over validating the user's beliefs. Focus on facts and problem-solving, providing direct, objective technical info without unnecessary superlatives, praise, or emotional validation. Apply the same rigorous standards to all ideas and disagree when necessary. Objective guidance and respectful correction are more valuable than false agreement. When uncertain, investigate first rather than confirming the user's beliefs. Avoid over-the-top validation or excessive praise (e.g. "You're absolutely right").

# Tone and style
Be concise, direct, and to the point. Prefer fewer than 4 lines (not including tool use or code) unless the user asks for detail. Minimize output tokens while staying helpful and accurate. Only address the specific query or task; skip tangential information unless critical. Do not add unnecessary preamble or postamble (e.g. "Here is what I did...", "Based on the information..."). After working on a file, stop rather than summarizing. Answer directly; one-word or short answers are fine when appropriate. Avoid introductions and conclusions. Do not wrap answers in phrases like "The answer is <answer>." or "Here is the content of the file...".

Examples:
- user: 2 + 2 → assistant: 4
- user: is 11 a prime number? → assistant: Yes
- user: what command lists files in the current directory? → assistant: ls

When you run a non-trivial bash command, briefly explain what it does and why so the user understands. Output is shown on a command-line interface. You may use GitHub-flavored markdown; it is rendered in monospace (CommonMark). Use output text to communicate; do not use tools (e.g. bash or code comments) to talk to the user. If you cannot or will not help, keep the response to 1–2 sentences and offer alternatives if possible. Use emojis only if the user explicitly asks. Keep responses short for CLI display.

# Proactiveness
Be proactive only when the user asks you to do something. Balance doing the right thing (including follow-up actions) with not surprising the user with unasked actions. If the user asks how to approach something, answer first before taking actions.

# Following conventions
When changing files, follow the file's existing conventions: mimic code style, use existing libraries and utilities, and follow existing patterns. Do not assume a library is available; check the codebase (e.g. package.json, go.mod, neighboring files) first. When creating a new component, look at existing components for framework choice, naming, typing, and conventions. When editing code, check surrounding context and imports, then make the change in the most idiomatic way. Follow security best practices: do not expose or log secrets and keys; never commit secrets or keys to the repository.

# Code style
Do not add comments unless the user asks for them.

# Task management
Use the TodoWrite tool to plan and track tasks. Use it often so the user sees progress and so you do not skip steps. Break larger tasks into smaller steps. Mark todos completed as soon as each task is done; do not batch completion.

# Doing tasks
For software engineering tasks (bugs, new features, refactoring, explaining code, etc.):
- Use TodoWrite to plan when the task is non-trivial.
- Use search tools to understand the codebase and the user's query.
- Implement using the tools available to you.
- Verify when possible (e.g. run tests). Do not assume a specific test framework; check README or codebase for how tests are run.
- If the user or project provides lint/typecheck commands (e.g. npm run lint, go vet), run them after completing the task. If you cannot find the command, you may ask the user.
- Do not commit changes unless the user explicitly asks you to.

Tool results and user messages may include <system_reminder> tags; those are internal reminders and are not part of the user's input or the tool result.

# Tool usage
- Prefer batching: when multiple independent pieces of information are needed, call multiple tools in a single message so they can run in parallel (e.g. one message with two tool calls for "git status" and "git diff").
- When webfetch indicates a redirect to a different host, make a new request to the redirect URL given in the response.

# Code references
When referring to specific code, use the pattern file_path:line_number so the user can jump to the source (e.g. "Handled in src/services/process.ts:712.").`

// StreamSink receives content deltas during streaming. Implementations may write to stdout or send to a TUI.
type StreamSink interface {
	OnDelta(delta string)
}

// LLMCaller can perform chat-with-tools. *llm.Client implements this.
type LLMCaller interface {
	ChatWithTools(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (content string, toolCalls []llm.ToolCall, usage llm.Usage, err error)
	ChatWithToolsStream(ctx context.Context, messages []llm.Message, tools []llm.ToolDef, onDelta func(string)) (content string, toolCalls []llm.ToolCall, usage llm.Usage, err error)
}

// processConfig holds per-call options for Process/ProcessAfterUserAppended.
type processConfig struct {
	streamSink StreamSink
}

// ProcessOption configures a single agent run (e.g. streaming).
type ProcessOption func(*processConfig)

// WithStreamSink sets the sink that receives content deltas during the LLM stream.
func WithStreamSink(sink StreamSink) ProcessOption {
	return func(c *processConfig) {
		c.streamSink = sink
	}
}

// RunStats holds statistics collected during a single agent run.
type RunStats struct {
	ToolCalls        int // total number of individual tool invocations
	PromptTokens     int // accumulated prompt tokens from LLM calls
	CompletionTokens int // accumulated completion tokens from LLM calls
}

// Agent runs the agent loop: LLM call → execute tool_calls if any → repeat until final reply.
type Agent struct {
	caller       LLMCaller
	tools        []core.Tool
	maxIter      int
	systemPrompt string
	toolDefs     []llm.ToolDef // cached from tools for each request
	toolsByName  map[string]core.Tool
}

// Option configures an Agent.
type Option func(*Agent)

// MaxIterations sets the maximum number of loop iterations (default 10).
func MaxIterations(n int) Option {
	return func(a *Agent) {
		a.maxIter = n
	}
}

// SystemPrompt overrides the default system prompt used by the agent loop.
// This allows sub-agents to use role-specific prompts instead of DefaultSystemPrompt.
func SystemPrompt(prompt string) Option {
	return func(a *Agent) {
		a.systemPrompt = prompt
	}
}

// ToolDefs builds LLM tool definitions from a slice of Tool. Used by both the CLI agent and the conversation loop.
func ToolDefs(toolList []core.Tool) []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(toolList))
	for _, t := range toolList {
		defs = append(defs, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return defs
}

// ExecuteTool runs one tool call: parses tc.Arguments, calls tool.Execute, and returns the result or an error string.
// Used by both the CLI agent (which then appends to session) and the conversation loop.
func ExecuteTool(ctx context.Context, t core.Tool, tc llm.ToolCall) string {
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

// NewAgent builds an agent with the given LLM caller and tools.
func NewAgent(caller LLMCaller, toolList []core.Tool, opts ...Option) *Agent {
	toolDefs := ToolDefs(toolList)
	byName := make(map[string]core.Tool, len(toolList))
	for _, t := range toolList {
		byName[t.Name()] = t
	}
	a := &Agent{
		caller:       caller,
		tools:        toolList,
		maxIter:      DefaultMaxIterations,
		systemPrompt: DefaultSystemPrompt,
		toolDefs:     toolDefs,
		toolsByName:  byName,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Process runs the agent loop for one user message using the given session:
// appends the user message to the session, then runs the loop (LLM call → tool calls if any → append to session),
// and returns the final assistant reply along with run statistics.
func (a *Agent) Process(ctx context.Context, sess *session.Session, userMessage string, opts ...ProcessOption) (reply string, stats RunStats, err error) {
	slog.Info("agent process with session started")
	sess.Append(llm.Message{Role: "user", Content: userMessage})
	slog.Info("user message", "content", userMessage)
	cfg := processConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return a.processLoop(ctx, sess, cfg)
}

// ProcessAfterUserAppended runs the agent loop when the last message in the session is already the user message.
// It does not append the user message; use this when the caller (e.g. TUI) has already appended it and refreshed the view.
// Returns an error if the session is empty or the last message is not from the user.
func (a *Agent) ProcessAfterUserAppended(ctx context.Context, sess *session.Session, opts ...ProcessOption) (reply string, stats RunStats, err error) {
	msgs := sess.Messages()
	if len(msgs) == 0 {
		return "", RunStats{}, errors.New("agent: session has no messages")
	}
	last := msgs[len(msgs)-1]
	if last.Role != "user" {
		return "", RunStats{}, fmt.Errorf("agent: last message is %q, not user", last.Role)
	}
	slog.Info("agent process after user appended", "content", last.Content)
	cfg := processConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return a.processLoop(ctx, sess, cfg)
}

// processLoop runs the LLM loop: build messages (system + session), call LLM, handle tool_calls, append to session, repeat until final reply.
func (a *Agent) processLoop(ctx context.Context, sess *session.Session, cfg processConfig) (reply string, stats RunStats, err error) {
	var totalToolCalls, totalPrompt, totalCompletion int
	for i := 0; i < a.maxIter; i++ {
		ctx = session.CtxWithSessionID(ctx, sess.ID())
		slog.Debug("agent iteration", "iter", i+1, "max", a.maxIter)
		messages := append([]llm.Message{{Role: "system", Content: a.systemPrompt}}, sess.Messages()...)
		var content string
		var toolCalls []llm.ToolCall
		var usage llm.Usage
		if cfg.streamSink != nil {
			content, toolCalls, usage, err = a.caller.ChatWithToolsStream(ctx, messages, a.toolDefs, cfg.streamSink.OnDelta)
		} else {
			content, toolCalls, usage, err = a.caller.ChatWithTools(ctx, messages, a.toolDefs)
		}
		if err != nil {
			slog.Error("LLM call failed", "err", err)
			return "", RunStats{ToolCalls: totalToolCalls, PromptTokens: totalPrompt, CompletionTokens: totalCompletion}, fmt.Errorf("llm call: %w", err)
		}
		totalPrompt += usage.PromptTokens
		totalCompletion += usage.CompletionTokens
		if len(toolCalls) == 0 {
			slog.Debug("agent reply", "content", content)
			sess.Append(llm.Message{Role: "assistant", Content: content})
			return content, RunStats{ToolCalls: totalToolCalls, PromptTokens: totalPrompt, CompletionTokens: totalCompletion}, nil
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
		totalToolCalls += len(toolCalls)
	}
	slog.Warn("agent max iterations exceeded")
	return "", RunStats{ToolCalls: totalToolCalls, PromptTokens: totalPrompt, CompletionTokens: totalCompletion}, errors.New("agent: max iterations exceeded")
}

// processOneToolCall resolves the tool by name, executes via ExecuteTool, and appends
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
	result := ExecuteTool(ctx, tool, tc)
	if len(result) > 500 {
		slog.Debug("tool result", "tool", tc.Name, "content", result[:500]+"...")
	} else {
		slog.Debug("tool result", "tool", tc.Name, "content", result)
	}
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

// AgentsMdFilename is the name of the workspace-level agent instructions file
// per the agents.md convention (https://agents.md/).
const AgentsMdFilename = "AGENTS.md"

// ReadAgentsMd reads AGENTS.md from the given workspace root directory.
// It returns the file contents (trimmed) and nil when the file exists and is readable.
// It returns ("", nil) when the file does not exist (so callers can treat missing as no extra prompt).
// On other read errors it logs a warning and returns ("", err).
func ReadAgentsMd(dir string) (string, error) {
	path := filepath.Join(dir, AgentsMdFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		slog.Warn("read AGENTS.md failed", "path", path, "err", err)
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
