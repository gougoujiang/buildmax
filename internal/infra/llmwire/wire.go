// Package llmwire is the versioned wire contract for BuildMax managed
// inference.
//
// It is the single definition of the protocol: the server handler and the
// remote client both marshal these types, so a field can never mean one thing
// on one side and something else on the other.
//
// The contract is deliberately narrower than any provider API. It carries
// messages, tool definitions, tool calls, and usage — the semantic content of
// internal/core/llm — and nothing about where a call goes. Upstream URLs,
// provider credentials, provider model identifiers, and free-form generation
// parameters are not part of it in either direction.
//
// Mirrors the design in docs/design/llm-gateway.md section 8.
package llmwire

import "encoding/json"

// Version is the protocol version these types describe. A breaking change to
// any shape here needs a new version, not a silent edit.
const Version = "1"

// Paths, relative to the server base URL.
const (
	// ModelsPath lists the aliases a team may use.
	ModelsPath = "/api/teams/%s/llm/models"
	// CompletionsPath runs one managed call.
	CompletionsPath = "/api/teams/%s/llm/completions"
)

// Message is one chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCall is a tool invocation.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// Tool is a tool the model may call.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// Metadata is correlation context. It is never authorization input: team and
// user identity come from authentication, not from here.
type Metadata struct {
	Surface   string  `json:"surface,omitempty"`
	SessionID *string `json:"session_id,omitempty"`
}

// CompletionRequest is one managed completion request.
type CompletionRequest struct {
	// CallID is the caller's idempotency key. It identifies one logical
	// invocation; it is not permission to replay a generation.
	CallID   *string   `json:"call_id,omitempty"`
	Model    string    `json:"model,omitempty"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
	Stream   bool      `json:"stream,omitempty"`
	Metadata *Metadata `json:"metadata,omitempty"`
}

// Usage is the token usage for one call.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CompletionResponse is a finished managed call.
type CompletionResponse struct {
	LLMCallID string     `json:"llm_call_id"`
	Model     string     `json:"model"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// Usage is absent when the provider reported none. An absent usage is not
	// the same fact as zero tokens.
	Usage *Usage `json:"usage,omitempty"`
}

// Model is one alias a team may use. It carries no endpoint, credential,
// provider type, or upstream model identifier.
type Model struct {
	Alias        string   `json:"alias"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
	Default      bool     `json:"default"`
}

// ModelsResponse lists a team's available models.
type ModelsResponse struct {
	Models []Model `json:"models"`
}

// ErrorResponse is a failed managed call. Code is the stable classification;
// Error is a safe message. Neither carries a provider's own error body.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}
