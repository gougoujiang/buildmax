// Package llm provides LLM client abstractions and implementations (OpenRouter/OpenAI-compatible).

package llm

// Message represents a chat message for the API (user, assistant, or tool).
type Message struct {
	Role       string     // "user", "assistant", "system", or "tool"
	Content    string     // message content
	ToolCallID string     // for role "tool": the ID of the tool call this result answers
	ToolCalls  []ToolCall // for role "assistant": tool calls made by the model
}

// ToolDef describes a tool (function) the model can call.
type ToolDef struct {
	Name        string // tool name
	Description string // description for the model
	Parameters  any    // JSON schema for arguments (e.g. map[string]any or jsonschema.Definition)
}

// ToolCall is a tool invocation returned by the model.
type ToolCall struct {
	ID        string // unique id for this call
	Name      string // tool name to invoke
	Arguments string // JSON object of arguments
}
