package llm

import (
	"context"
	"encoding/json"
)

// Message represents a chat message for the API (user, assistant, or tool).
type Message struct {
	Role       string     `json:"role"`                   // "user", "assistant", "system", or "tool"
	Content    string     `json:"content,omitempty"`      // message content
	ToolCallID string     `json:"tool_call_id,omitempty"` // for role "tool": the ID of the tool call this result answers
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // for role "assistant": tool calls made by the model
	// ProviderState is opaque reasoning state the producing protocol requires
	// back on later turns. For role "assistant" only. See ProviderState.
	ProviderState *ProviderState `json:"provider_state,omitempty"`
}

// ProviderState is provider-owned content that a protocol produces and then
// requires unchanged on subsequent requests: Anthropic thinking blocks, OpenAI
// Responses reasoning items. Nothing outside the adapter that produced it may
// interpret it, and nothing rewrites it — a signature over edited content is
// worse than no state at all.
//
// Protocol names the producer so a session continued under a different one
// drops what it cannot use, rather than sending a payload that protocol would
// reject. That is what lets history stay portable while carrying state that is
// not.
type ProviderState struct {
	Protocol string          `json:"protocol"`
	Data     json.RawMessage `json:"data"`
}

// Belongs reports whether this state was produced by the named protocol. A nil
// state belongs to none, so callers can test the result directly.
func (p *ProviderState) Belongs(protocol string) bool {
	return p != nil && p.Protocol == protocol && len(p.Data) > 0
}

// Completion is one model turn: what the assistant said, what it asked to run,
// what it cost, and any reasoning state the protocol needs back.
//
// It is a struct rather than a longer return list because every capability the
// contract has gained wanted another slot, and a fifth positional value is
// where that stops being readable.
type Completion struct {
	Content   string
	ToolCalls []ToolCall
	Usage     Usage
	// ProviderState is set only by a protocol that carries reasoning state and
	// only when the model produced some.
	ProviderState *ProviderState
}

// AssistantMessage is the history entry this completion becomes. The agent loop
// appends it verbatim, so reasoning state reaches the next request without any
// layer between here and there having to know it exists.
func (c Completion) AssistantMessage() Message {
	return Message{
		Role:          "assistant",
		Content:       c.Content,
		ToolCalls:     c.ToolCalls,
		ProviderState: c.ProviderState,
	}
}

// ToolDef describes a tool (function) the model can call.
type ToolDef struct {
	Name        string // tool name
	Description string // description for the model
	Parameters  any    // JSON schema for arguments (e.g. map[string]any or jsonschema.Definition)
}

// ToolCall is a tool invocation returned by the model.
type ToolCall struct {
	ID        string `json:"id"`                  // unique id for this call
	Name      string `json:"name"`                // tool name to invoke
	Arguments string `json:"arguments,omitempty"` // JSON object of arguments
}

// Usage holds token counts from the API (same shape for non-stream and stream responses).
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// LLMClient can perform chat completions with tools and exposes its configuration.
type LLMClient interface {
	ChatCompletionBlocking(ctx context.Context, messages []Message, tools []ToolDef) (Completion, error)
	ChatCompletionStreaming(ctx context.Context, messages []Message, tools []ToolDef, onDelta func(string)) (Completion, error)
	ContextWindow() int // 0 = no windowing configured
}

// StreamSink receives content deltas during streaming. Implementations may write to stdout, send to a TUI, or buffer for SSE.
type StreamSink interface {
	OnDelta(delta string)
}

// TitleGenerator generates a short title from an input string, e.g. via LLM.
// Returns token usage for metering; on error or when nil, callers fall back to truncated input.
type TitleGenerator interface {
	GenerateTitle(ctx context.Context, input string) (title string, promptTokens, completionTokens int, err error)
}
