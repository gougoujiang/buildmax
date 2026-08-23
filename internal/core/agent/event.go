package agent

import (
	"sync"
	"time"
)

// EventKind identifies the type of a runtime event emitted during a RunLoop execution.
type EventKind uint8

const (
	// EventIterStart fires at the top of each agent loop iteration.
	EventIterStart EventKind = iota

	// EventLLMStart fires just before an LLM API call is dispatched.
	EventLLMStart

	// EventLLMDelta fires for each content delta when streaming is active.
	// Only emitted when StreamSink is also set (streaming mode).
	EventLLMDelta

	// EventLLMEnd fires after the LLM call returns with the full content
	// and whether the response contains tool calls.
	EventLLMEnd

	// EventToolStart fires before a tool call is evaluated (allow, deny, or execute).
	EventToolStart

	// EventToolEnd fires after a tool executes — whether it returned a result or an error string.
	EventToolEnd

	// EventToolDenied fires when a tool call is blocked before execution.
	EventToolDenied

	// EventContextCompacted fires when the message history is compacted to free context space.
	EventContextCompacted

	// EventRunEnd fires when RunLoop exits, whether successfully, on error, or on cancellation.
	EventRunEnd

	// EventUserInput fires when a message the user submitted while the run was
	// working is appended to the history at an iteration boundary. Content holds
	// the message.
	EventUserInput

	// EventUserInputBlocked fires when a UserPromptSubmit hook refuses such a
	// message. Content holds the message and DenyReason the hook's reason; the
	// message is not appended to the history.
	EventUserInputBlocked
)

// Deny reason labels for EventToolDenied.
const (
	DenyReasonPolicy    = "policy"     // blocked by ToolPolicy or tool's own DefaultAction
	DenyReasonUser      = "user"       // user denied the approval prompt
	DenyReasonLoopGuard = "loop_guard" // repeated identical call blocked by loop guard
	DenyReasonHook      = "hook"       // blocked by a PreToolUse hook
	DenyReasonUnknown   = "unknown_tool"
)

// Event is a structured runtime event emitted by RunLoop.
// Passed by value to EventSink; fields not relevant to a given Kind are zero.
type Event struct {
	Kind EventKind

	// EventIterStart, EventLLMStart, EventLLMEnd, EventUserInput, EventUserInputBlocked
	Iter int

	// EventLLMDelta, EventLLMEnd, EventUserInput, EventUserInputBlocked
	Content string

	// EventLLMEnd
	HasToolCalls bool

	// EventLLMStart, EventLLMEnd
	ContextTokens    int
	ContextWindow    int
	PromptTokens     int
	CompletionTokens int
	// CacheReadTokens and CacheWriteTokens are the run's cached prompt so far.
	// They are a breakdown of PromptTokens, not an addition to it.
	CacheReadTokens  int
	CacheWriteTokens int

	// EventToolStart, EventToolEnd, EventToolDenied
	ToolName   string
	ToolCallID string
	ToolArgs   string // JSON-encoded arguments

	// EventToolEnd
	ToolResult   string
	ToolDuration time.Duration

	// EventToolDenied, EventUserInputBlocked
	DenyReason string

	// EventContextCompacted
	Summarized int
	Kept       int

	// EventRunEnd
	Stats RunStats
	Err   error // nil on success or graceful cancellation
}

// emit calls sink with e when sink is non-nil.
func emit(sink func(Event), e Event) {
	if sink != nil {
		sink(e)
	}
}

// serializedSink guards a sink so tool workers and the loop goroutine cannot
// call it at once. The guarantee lives here rather than in each consumer --
// the TUI, Desktop, the trace recorder, and Portal would otherwise each have to
// solve it, and one of them would get it wrong.
//
// A nil sink stays nil: no allocation, no lock, no cost for a run that emits
// nothing.
func serializedSink(sink func(Event)) func(Event) {
	if sink == nil {
		return nil
	}
	var mu sync.Mutex
	return func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		sink(e)
	}
}
