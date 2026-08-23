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
	// Source records non-user provenance for a user-role message. Model
	// providers share no portable mid-history event role, so a background
	// event travels on the wire as a user message — but the persisted history
	// and the trace must not claim the user said it. Empty means genuinely
	// user-authored. Wire adapters map fields explicitly and never send it.
	Source string `json:"source,omitempty"`
	// ProviderState is opaque reasoning state the producing protocol requires
	// back on later turns. For role "assistant" only. See ProviderState.
	ProviderState *ProviderState `json:"provider_state,omitempty"`
	// Parts is non-text content this message carries. Content stays the text
	// projection of the same message, so nothing that reads it has to know
	// parts exist. See ContentPart.
	Parts []ContentPart `json:"parts,omitempty"`
}

// Content part kinds.
const (
	ContentPartText  = "text"
	ContentPartImage = "image"
)

// Message sources for background-event provenance. See Message.Source; the
// set mirrors docs/design/local-background-jobs.md.
const (
	MessageSourceCommandResult  = "command_result"
	MessageSourceSubagentResult = "subagent_result"
	MessageSourceMonitorEvent   = "monitor_event"
)

// ContentPart is one piece of a message's content.
//
// Parts sit beside Content rather than replacing it. A message with an image
// still carries text saying what the image is, so token estimation, trimming,
// compaction, traces, and the terminal renderer keep working unchanged and a
// protocol that cannot take images still receives a sensible turn.
type ContentPart struct {
	Type string `json:"type"`
	// Text is set on a text part.
	Text string `json:"text,omitempty"`
	// MediaType and Data are set on an image part. Data is base64 with no
	// data: prefix, which is the form every protocol here wants.
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
}

// Images returns the image parts of a message, or nil when it has none.
func (m Message) Images() []ContentPart {
	var out []ContentPart
	for _, part := range m.Parts {
		if part.Type == ContentPartImage && part.Data != "" {
			out = append(out, part)
		}
	}
	return out
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
	// CacheReadTokens is the part of the prompt served from a provider's cache,
	// and CacheWriteTokens the part written into it. Both are subsets of the
	// prompt: adding them to PromptTokens would count the same tokens twice.
	// Zero means the provider reported none, which is also what a provider
	// without caching reports.
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// CallProfile is what a call is for. It is the caller's statement of intent,
// not a provider setting: a title and a tool-calling turn send the same shape
// of request and have nothing in common in how they will be reused.
//
// It exists because prompt caching is charged. A cache write costs more than
// ordinary input and only repays itself if a later call reads it, so whether to
// ask for one cannot be decided from the request alone — a one-shot utility
// call and the first turn of a long run look identical on the wire. Carrying
// the answer in an untyped context value, or guessing it from prompt text,
// would hide a billed behavior from the callers and tests that have to reason
// about it.
type CallProfile string

const (
	// ProfileAgentTurn is one iteration of the agent loop: a large stable
	// prefix that the next iteration will send again.
	ProfileAgentTurn CallProfile = "agent_turn"
	// ProfileTitle is one-shot title generation.
	ProfileTitle CallProfile = "title"
	// ProfileCompaction is summarizing history the run is about to discard.
	ProfileCompaction CallProfile = "compaction"
	// ProfileEvaluation is a harness call made about a run rather than by one.
	ProfileEvaluation CallProfile = "evaluation"
	// ProfileProbe is a single question with no expectation of reuse: a
	// connectivity check, a tool's own model call.
	ProfileProbe CallProfile = "probe"
)

// Valid reports whether p is a profile this build knows. An unknown profile is
// refused rather than defaulted, because the default it would fall to is the
// one that spends money.
func (p CallProfile) Valid() bool {
	switch p {
	case ProfileAgentTurn, ProfileTitle, ProfileCompaction, ProfileEvaluation, ProfileProbe:
		return true
	}
	return false
}

// Request is one completion request.
//
// Profile travels with the messages rather than beside them so a caller cannot
// forget it at one call site and get a different charge than at another.
type Request struct {
	Messages []Message
	Tools    []ToolDef
	Profile  CallProfile
	// CacheScope is an opaque discriminator that keeps unrelated prompt
	// populations out of one another's provider cache bucket. Empty means the
	// call is not scoped beyond the credential it uses.
	//
	// It exists because a provider cache key is a routing hint, not an
	// authorization boundary: two callers sharing one credential share a bucket
	// unless something separates them. For managed inference the gateway sets
	// it from the authenticated team, so one team's prefix cannot be bucketed
	// with another's. It is never accepted from a client, never persisted, and
	// never logged — it is an input to a hash and nothing else.
	CacheScope string
}

// LLMClient can perform chat completions with tools and exposes its configuration.
type LLMClient interface {
	ChatCompletionBlocking(ctx context.Context, req Request) (Completion, error)
	ChatCompletionStreaming(ctx context.Context, req Request, onDelta func(string)) (Completion, error)
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
