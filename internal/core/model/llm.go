package model

import "context"

// Message represents a chat message for the API (user, assistant, or tool).
type Message struct {
	Role       string     `json:"role"`                   // "user", "assistant", "system", or "tool"
	Content    string     `json:"content,omitempty"`      // message content
	ToolCallID string     `json:"tool_call_id,omitempty"` // for role "tool": the ID of the tool call this result answers
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // for role "assistant": tool calls made by the model
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

// LLMClient can perform chat completions with tools.
type LLMClient interface {
	ChatCompletionBlocking(ctx context.Context, messages []Message, tools []ToolDef) (content string, toolCalls []ToolCall, usage Usage, err error)
	ChatCompletionStreaming(ctx context.Context, messages []Message, tools []ToolDef, onDelta func(string)) (content string, toolCalls []ToolCall, usage Usage, err error)
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
