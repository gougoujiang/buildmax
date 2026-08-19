// Package agent provides the core AI agent logic: task planning, tool invocation, and conversation.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

const denyMsgPolicy = "error: tool call %q denied by policy"
const denyMsgLoopGuard = "error: tool call %q blocked — repeated identical call detected (loop guard)"
const denyMsgUser = "error: tool call %q denied by user"
const denyMsgHook = "error: tool call %q denied by hook: %s"

// DefaultMaxIterations is the default cap on agent loop iterations.
const DefaultMaxIterations = 200

// RunStats holds statistics collected during a single agent run.
type RunStats struct {
	ToolCalls        int
	PromptTokens     int
	CompletionTokens int
}

// MessageHistory is the minimal interface for the agent loop: read the conversation so far and append one message.
// The loop uses it so the same logic works with in-memory session or DB-backed conversation.
type MessageHistory interface {
	HistoryMessages() []llm.Message
	Append(m llm.Message) error
}

// CompactionHistory is an optional extension of MessageHistory implemented by persistent histories
// that can record a compaction boundary across turns. When RunLoop compacts the context, it calls
// AddCompaction so the next turn starts from the compacted view without re-summarizing.
//
// The history owns the summary, and RunLoop reads it back through PriorSummary rather than
// expecting the caller to have folded it into SystemPrompt. That keeps one owner for the
// block: two owners is how the same summary ends up in the prompt twice.
type CompactionHistory interface {
	MessageHistory
	// PriorSummary returns the summary stored by the most recent compaction, or "" when the
	// history has never been compacted.
	PriorSummary() string
	// AddCompaction advances the compaction boundary by summarizedCount messages and stores summary.
	AddCompaction(summary string, summarizedCount int)
}

// ContextCompactor summarizes a slice of messages into a short text that can replace them.
// The returned summary is injected into the system prompt so the LLM retains prior context.
type ContextCompactor interface {
	Compact(ctx context.Context, msgs []llm.Message) (summary string, err error)
}

// StateCheckpointer is handed the messages a compaction is about to discard, and gets one turn
// to move anything still needed into durable session state before they are gone.
//
// This exists because a tool the model has to remember to call will be forgotten exactly when
// context pressure is highest. Compaction is the one moment where the runtime knows
// information is being destroyed, so it is the runtime, not the model, that decides the
// checkpoint happens.
//
// Failure is not fatal: a checkpoint that errors is logged and compaction proceeds.
type StateCheckpointer interface {
	Checkpoint(ctx context.Context, discarded []llm.Message) error
}

// RunLoopOpts configures a single run of the shared agent loop (used by both CLI agent and conversation).
type RunLoopOpts struct {
	LLMClient    llm.LLMClient
	SystemPrompt string
	ToolRegistry llm.ToolRegistry
	MaxIter      int
	History      MessageHistory
	StreamSink   llm.StreamSink
	// Policy is consulted before each tool execution. Nil defaults to AllowAllPolicy.
	Policy ToolPolicy
	// Approval is invoked when Policy returns ToolActionAsk.
	// Nil approval with ToolActionAsk falls through to Allow for backward compatibility.
	Approval ApprovalHandler
	// Compactor summarizes old messages when the context window is filling up.
	// Nil disables compaction; TrimHistory is used as a fallback.
	Compactor ContextCompactor
	// Checkpointer is given one turn to save durable state before a compaction discards
	// messages. Nil skips the checkpoint; compaction is unaffected either way.
	Checkpointer StateCheckpointer
	// EventSink receives structured runtime events from the agent loop.
	// Nil disables event emission entirely (zero overhead).
	// The callback is invoked synchronously from the RunLoop goroutine; it must not block.
	EventSink func(Event)
	// Hooks runs lifecycle hooks at fixed points (PreToolUse, PostToolUse,
	// PostToolUseFailure, Notification, PreCompact, PostCompact, Stop /
	// SubagentStop / StopFailure). Nil or NoopHookRunner disables hooks.
	// PreToolUse and PreCompact hooks may block their respective actions.
	Hooks HookRunner
	// SessionID is forwarded to hook payloads so external scripts can correlate runs.
	// Optional; an empty value is omitted from hook input.
	SessionID string
	// Workspace is forwarded to hook payloads so external scripts can locate files
	// under the active workspace. Optional.
	Workspace string
	// IsSubagent is true when this RunLoop is a subagent execution. It
	// flips the lifecycle event on success from Stop to SubagentStop and
	// is stamped on every event from this run for audit attribution.
	IsSubagent bool
	// AgentType is the subagent definition name when IsSubagent is true.
	// Empty for main-agent runs.
	AgentType string
}

// RunLoop runs the LLM loop once: build messages from history, call LLM, handle tool_calls, append to history, repeat until final reply.
// It is used by Agent.processLoop (with session history) and by conversation.Run (with DB-backed history).
// When ctx is cancelled mid-run, RunLoop returns the last assistant content produced (if any) and a nil error,
// so callers receive a partial result rather than an empty failure.
func RunLoop(ctx context.Context, opts RunLoopOpts) (reply string, stats RunStats, err error) {
	var s RunStats
	guard := newLoopGuard(defaultMaxRepeatedCalls)
	// Most recent compaction summary, rendered into the system prompt when non-empty.
	// Seeded from the history so a session compacted in an earlier turn keeps its summary
	// and feeds it back into the next compaction.
	compactionSummary := ""
	if ch, ok := opts.History.(CompactionHistory); ok {
		compactionSummary = ch.PriorSummary()
	}
	lastContent := "" // last non-empty assistant content; returned on cancellation

	for i := 0; i < opts.MaxIter; i++ {
		slog.Debug("agent run loop iteration", "iter", i+1, "max", opts.MaxIter)
		emit(opts.EventSink, Event{Kind: EventIterStart, Iter: i + 1})

		history := opts.History.HistoryMessages()

		// Context compaction: summarize old messages before the context window fills up.
		if opts.Compactor != nil {
			if cw := opts.LLMClient.ContextWindow(); cw > 0 {
				sysTokens := EstimateMessageTokens(llm.Message{Role: "system", Content: opts.SystemPrompt})
				if sysTokens+EstimateTokens(history) > int(float64(cw)*compactionThreshold) {
					reserveTokens := int(float64(cw) * compactionReserve)
					toSummarize, toKeep := splitForCompaction(history, reserveTokens)
					if len(toSummarize) > 0 {
						pre := baseHookInput(opts, HookPreCompact)
						pre.Summarized = len(toSummarize)
						pre.Kept = len(toKeep)
						preOut := runHook(ctx, opts.Hooks, pre)
						if preOut.Blocked() {
							slog.Info("context compaction skipped by hook", "iter", i+1, "reason", preOut.Reason)
						} else if summary, cerr := checkpointAndCompact(ctx, opts, i+1, compactionSummary, toSummarize); cerr == nil {
							limit := maxSummaryChars(cw)
							if clamped := clampSummary(summary, limit); clamped != summary {
								slog.Warn("compaction summary exceeded its budget, clamped", "iter", i+1, "limit_chars", limit, "got_chars", len(summary))
								summary = clamped
							}
							compactionSummary = summary
							if ch, ok := opts.History.(CompactionHistory); ok {
								// summarizedCount counts real history messages; the prior summary
								// prepended above is synthetic and never entered the history.
								ch.AddCompaction(summary, len(toSummarize))
							}
							history = toKeep
							slog.Info("context compacted", "iter", i+1, "summarized", len(toSummarize), "kept", len(toKeep))
							emit(opts.EventSink, Event{
								Kind:       EventContextCompacted,
								Iter:       i + 1,
								Summarized: len(toSummarize),
								Kept:       len(toKeep),
							})
							post := baseHookInput(opts, HookPostCompact)
							post.Summarized = len(toSummarize)
							post.Kept = len(toKeep)
							post.Summary = summary
							runHook(ctx, opts.Hooks, post)
						} else {
							slog.Warn("context compaction failed, falling back to trim", "err", cerr)
						}
					}
				}
			}
		}

		// Build effective system prompt: base + compaction summary when present.
		// SystemPrompt must not already carry the block — RunLoop is its only renderer.
		effectiveSysPrompt := opts.SystemPrompt + RenderCompactionBlock(compactionSummary)

		content, toolCalls, usage, err := callLLM(ctx, opts, history, effectiveSysPrompt, i+1, s)
		if err != nil {
			if ctx.Err() != nil {
				slog.Warn("agent run interrupted by context cancellation", "iter", i+1, "last_content_len", len(lastContent))
				emit(opts.EventSink, Event{Kind: EventRunEnd, Stats: s})
				fireRunEndHook(ctx, opts, s, nil)
				return lastContent, s, nil
			}
			slog.Error("LLM call failed", "err", err)
			runErr := fmt.Errorf("llm call: %w", err)
			emit(opts.EventSink, Event{Kind: EventRunEnd, Stats: s, Err: runErr})
			fireRunEndHook(ctx, opts, s, runErr)
			return "", s, runErr
		}
		s.PromptTokens += usage.PromptTokens
		s.CompletionTokens += usage.CompletionTokens

		if content != "" {
			lastContent = content
		}

		emit(opts.EventSink, Event{
			Kind:             EventLLMEnd,
			Iter:             i + 1,
			Content:          content,
			HasToolCalls:     len(toolCalls) > 0,
			PromptTokens:     s.PromptTokens,
			CompletionTokens: s.CompletionTokens,
		})

		if len(toolCalls) == 0 {
			slog.Debug("agent reply", "content", content)
			if err := opts.History.Append(llm.Message{Role: "assistant", Content: content}); err != nil {
				return "", s, err
			}
			emit(opts.EventSink, Event{Kind: EventRunEnd, Stats: s})
			fireRunEndHook(ctx, opts, s, nil)
			return content, s, nil
		}

		slog.Debug("tool calls", "n", len(toolCalls), "content", content, "calls", toolCallsSummary(toolCalls))
		if err := opts.History.Append(llm.Message{Role: "assistant", Content: content, ToolCalls: toolCalls}); err != nil {
			return "", s, err
		}

		// Tools that write durable state stamp entries with the iteration they were written at.
		n, err := executeToolCalls(CtxWithIteration(ctx, i+1), opts, toolCalls, guard)
		s.ToolCalls += n
		if err != nil {
			return "", s, err
		}
	}
	slog.Warn("agent max iterations exceeded")
	maxErr := errors.New("agent: max iterations exceeded")
	emit(opts.EventSink, Event{Kind: EventRunEnd, Stats: s, Err: maxErr})
	fireRunEndHook(ctx, opts, s, maxErr)
	return "", s, maxErr
}

// checkpointAndCompact gives the checkpointer one turn to move anything still needed out of the
// messages about to be discarded, then summarizes them.
//
// The checkpoint runs first because after Compact returns, the material is only reachable
// through a lossy summary. Its failure is logged and ignored: losing the checkpoint costs some
// context, but skipping the compaction it guards would cost the run.
func checkpointAndCompact(ctx context.Context, opts RunLoopOpts, iter int, priorSummary string, toSummarize []llm.Message) (string, error) {
	if opts.Checkpointer != nil {
		// The iteration goes on the context so notes written here are stamped like any other.
		if err := opts.Checkpointer.Checkpoint(CtxWithIteration(ctx, iter), toSummarize); err != nil {
			slog.Warn("state checkpoint before compaction failed", "iter", iter, "err", err)
		}
	}
	return opts.Compactor.Compact(ctx, withPriorSummary(priorSummary, toSummarize))
}

// fireRunEndHook invokes the lifecycle hook for a finished run. Routes to
// HookStop (main success), HookSubagentStop (subagent success), or
// HookStopFailure (any error). Decisions are ignored — these events are
// advisory; hook failures are swallowed by the runner.
func fireRunEndHook(ctx context.Context, opts RunLoopOpts, stats RunStats, err error) {
	if opts.Hooks == nil {
		return
	}
	event := HookStop
	switch {
	case err != nil:
		event = HookStopFailure
	case opts.IsSubagent:
		event = HookSubagentStop
	}
	in := HookInput{
		Event:      event,
		SessionID:  opts.SessionID,
		Workspace:  opts.Workspace,
		IsSubagent: opts.IsSubagent,
		AgentType:  opts.AgentType,
		Stats:      &stats,
	}
	if err != nil {
		in.Error = err.Error()
	}
	runHook(ctx, opts.Hooks, in)
}

// callLLM dispatches to streaming or blocking LLM call based on whether a StreamSink is set.
// When EventSink is also set, content deltas are forwarded to it as EventLLMDelta events.
// history and systemPrompt are passed explicitly so the caller can inject a compacted view.
func callLLM(ctx context.Context, opts RunLoopOpts, history []llm.Message, systemPrompt string, iter int, stats RunStats) (string, []llm.ToolCall, llm.Usage, error) {
	// Durable session state is rendered fresh on every call and placed after the messages, so
	// it is never subject to trimming and never accumulates in the history. An empty block
	// renders nothing, which is what a run that keeps no state should cost.
	var stateMsg []llm.Message
	if nh, ok := opts.History.(NotesHistory); ok {
		if block := RenderSessionState(nh.Notes(), nh.Todos(), iter); block != "" {
			stateMsg = []llm.Message{{Role: "user", Content: block}}
		}
	}

	systemTokens := EstimateMessageTokens(llm.Message{Role: "system", Content: systemPrompt}) + EstimateTokens(stateMsg)
	contextWindow := opts.LLMClient.ContextWindow()
	if contextWindow > 0 {
		history = TrimHistory(history, systemTokens, contextWindow, 0)
	}
	contextTokens := systemTokens + EstimateTokens(history)
	emit(opts.EventSink, Event{
		Kind:             EventLLMStart,
		Iter:             iter,
		ContextTokens:    contextTokens,
		ContextWindow:    contextWindow,
		PromptTokens:     stats.PromptTokens,
		CompletionTokens: stats.CompletionTokens,
	})
	messages := append([]llm.Message{{Role: "system", Content: systemPrompt}}, history...)
	messages = append(messages, stateMsg...)
	defs := opts.ToolRegistry.GetDefs()
	if opts.StreamSink != nil {
		onDelta := opts.StreamSink.OnDelta
		if opts.EventSink != nil {
			sink := opts.EventSink
			onDelta = func(delta string) {
				opts.StreamSink.OnDelta(delta)
				sink(Event{Kind: EventLLMDelta, Content: delta})
			}
		}
		return opts.LLMClient.ChatCompletionStreaming(ctx, messages, defs, onDelta)
	}
	return opts.LLMClient.ChatCompletionBlocking(ctx, messages, defs)
}

// executeToolCalls runs each tool call, applying the policy and loop guard before execution.
func executeToolCalls(ctx context.Context, opts RunLoopOpts, toolCalls []llm.ToolCall, guard *loopGuard) (int, error) {
	policy := opts.Policy
	if policy == nil {
		policy = AllowAllPolicy
	}
	count := 0
	for _, tc := range toolCalls {
		var args map[string]any
		if tc.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
				result := fmt.Sprintf("error: invalid arguments: %v", err)
				if err := opts.History.Append(llm.Message{Role: "tool", Content: result, ToolCallID: tc.ID}); err != nil {
					return count, err
				}
				count++
				continue
			}
		}
		if args == nil {
			args = make(map[string]any)
		}

		tool := opts.ToolRegistry.Lookup(tc.Name)
		var result string
		emit(opts.EventSink, Event{
			Kind:       EventToolStart,
			ToolName:   tc.Name,
			ToolCallID: tc.ID,
			ToolArgs:   tc.Arguments,
		})
		switch {
		case tool == nil:
			emit(opts.EventSink, Event{Kind: EventToolDenied, ToolName: tc.Name, ToolCallID: tc.ID, DenyReason: DenyReasonUnknown})
			result = fmt.Sprintf("error: unknown tool %q", tc.Name)
		case guard.exceeded(tc.Name, args):
			slog.Warn("loop guard triggered", "tool", tc.Name)
			emit(opts.EventSink, Event{Kind: EventToolDenied, ToolName: tc.Name, ToolCallID: tc.ID, DenyReason: DenyReasonLoopGuard})
			result = fmt.Sprintf(denyMsgLoopGuard, tc.Name)
		default:
			result = applyPolicyAndExecute(ctx, opts, policy, tool, tc.Name, tc.ID, args)
		}
		logToolResult(tc.Name, result)
		if err := opts.History.Append(llm.Message{Role: "tool", Content: result, ToolCallID: tc.ID}); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// applyPolicyAndExecute resolves the policy for a tool call and executes it if allowed.
//
// Resolution order:
//  1. Configured ToolPolicy override — if Deny or Ask, short-circuit.
//  2. Tool's ArgChecker.CheckArgs — arg-level decision declared by the tool itself.
//  3. Tool's PolicyProvider.DefaultAction — category-level default declared by the tool.
//  4. PreToolUse hook — may block the call after policy resolution.
//  5. Allow (safe default for tools that declare nothing).
//
// Ask handling: calls ApprovalHandler if set; nil handler collapses Ask to Deny.
func applyPolicyAndExecute(ctx context.Context, opts RunLoopOpts, policy ToolPolicy, tool llm.Tool, name, callID string, args map[string]any) string {
	action := resolveAction(policy, tool, name, args)
	switch action {
	case llm.ToolActionDeny:
		slog.Info("tool denied by policy", "tool", name)
		emit(opts.EventSink, Event{Kind: EventToolDenied, ToolName: name, ToolCallID: callID, DenyReason: DenyReasonPolicy})
		return fmt.Sprintf(denyMsgPolicy, name)
	case llm.ToolActionAsk:
		// Notify hooks before invoking the approval handler so external
		// systems (Slack, desktop badge, audit) see the prompt.
		fireNotification(ctx, opts, NotificationApprovalRequired, name, callID, args, "")
		if opts.Approval == nil {
			slog.Info("tool denied: Ask with no approval handler", "tool", name)
			emit(opts.EventSink, Event{Kind: EventToolDenied, ToolName: name, ToolCallID: callID, DenyReason: DenyReasonPolicy})
			fireNotification(ctx, opts, NotificationPermissionDenied, name, callID, args, "no approval handler configured")
			return fmt.Sprintf(denyMsgPolicy, name)
		}
		if !opts.Approval.RequestApproval(name, args) {
			slog.Info("tool denied by user", "tool", name)
			emit(opts.EventSink, Event{Kind: EventToolDenied, ToolName: name, ToolCallID: callID, DenyReason: DenyReasonUser})
			fireNotification(ctx, opts, NotificationPermissionDenied, name, callID, args, "denied by user")
			return fmt.Sprintf(denyMsgUser, name)
		}
		fallthrough
	case llm.ToolActionAllow:
		pre := baseHookInput(opts, HookPreToolUse)
		pre.ToolName = name
		pre.ToolCallID = callID
		pre.ToolArgs = args
		preOut := runHook(ctx, opts.Hooks, pre)
		if preOut.Blocked() {
			reason := preOut.Reason
			if reason == "" {
				reason = "blocked by hook"
			}
			slog.Info("tool denied by hook", "tool", name, "reason", reason)
			emit(opts.EventSink, Event{Kind: EventToolDenied, ToolName: name, ToolCallID: callID, DenyReason: DenyReasonHook})
			return fmt.Sprintf(denyMsgHook, name, reason)
		}
		start := time.Now()
		result, err := tool.Execute(ctx, args)
		dur := time.Since(start)
		if err != nil {
			errMsg := fmt.Sprintf("error: %v", err)
			emit(opts.EventSink, Event{Kind: EventToolEnd, ToolName: name, ToolCallID: callID, ToolResult: errMsg, ToolDuration: dur})
			fail := baseHookInput(opts, HookPostToolUseFailure)
			fail.ToolName = name
			fail.ToolCallID = callID
			fail.ToolArgs = args
			fail.ToolError = errMsg
			runHook(ctx, opts.Hooks, fail)
			return errMsg
		}
		emit(opts.EventSink, Event{Kind: EventToolEnd, ToolName: name, ToolCallID: callID, ToolResult: result, ToolDuration: dur})
		post := baseHookInput(opts, HookPostToolUse)
		post.ToolName = name
		post.ToolCallID = callID
		post.ToolArgs = args
		post.ToolResult = result
		runHook(ctx, opts.Hooks, post)
		return result
	default:
		return fmt.Sprintf("error: unknown policy action for %q", name)
	}
}

// fireNotification emits a HookNotification event tied to a tool/approval
// flow. Notification is advisory — the returned decision is ignored.
func fireNotification(ctx context.Context, opts RunLoopOpts, kind, toolName, callID string, args map[string]any, reason string) {
	if opts.Hooks == nil {
		return
	}
	in := baseHookInput(opts, HookNotification)
	in.NotificationKind = kind
	in.NotificationReason = reason
	in.ToolName = toolName
	in.ToolCallID = callID
	in.ToolArgs = args
	runHook(ctx, opts.Hooks, in)
}

// resolveAction applies the layered policy resolution for one tool call.
func resolveAction(policy ToolPolicy, tool llm.Tool, name string, args map[string]any) llm.ToolAction {
	// 1. Configured override — Deny/Ask wins immediately; Allow defers to tool.
	if policy != nil {
		if override := policy.Check(name, args); override != llm.ToolActionAllow {
			return override
		}
	}
	// 2. Tool arg-level check.
	if checker, ok := tool.(llm.ArgChecker); ok {
		if action := checker.CheckArgs(args); action != llm.ToolActionAllow {
			return action
		}
	}
	// 3. Tool category default.
	if provider, ok := tool.(llm.PolicyProvider); ok {
		return provider.DefaultAction()
	}
	return llm.ToolActionAllow
}

func logToolResult(name, result string) {
	if len(result) > 500 {
		slog.Debug("tool result", "tool", name, "content", result[:500]+"...")
	} else {
		slog.Debug("tool result", "tool", name, "content", result)
	}
}

// ExecuteTool parses tc.Arguments and calls tool.Execute, returning the result or an error string.
// Useful for testing and single-call invocations that bypass the policy layer.
func ExecuteTool(ctx context.Context, t llm.Tool, tc llm.ToolCall) string {
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
