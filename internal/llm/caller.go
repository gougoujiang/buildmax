// Package llm provides LLM client abstractions and implementations (OpenRouter/OpenAI-compatible).

package llm

import "context"

// LLMCaller can perform chat-with-tools. *Client implements this.
// Used by agent, conversation, and tools.
type LLMCaller interface {
	ChatWithTools(ctx context.Context, messages []Message, tools []ToolDef) (content string, toolCalls []ToolCall, usage Usage, err error)
	ChatWithToolsStream(ctx context.Context, messages []Message, tools []ToolDef, onDelta func(string)) (content string, toolCalls []ToolCall, usage Usage, err error)
}

// StreamSink receives content deltas during streaming. Implementations may write to stdout, send to a TUI, or buffer for SSE.
type StreamSink interface {
	OnDelta(delta string)
}
