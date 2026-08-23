package model

import "context"

// Conversation is the Tier 1 conversation container.
type Conversation struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	TeamID    string `json:"team_id,omitempty"`
	Channel   string `json:"channel"`
	Title     string `json:"title,omitempty"`
	CreatedBy string `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
}

// ConversationMessage is one message in a Tier 1 conversation.
type ConversationMessage struct {
	ID             string  `json:"id"`
	ConversationID string  `json:"conversation_id"`
	Role           string  `json:"role"`
	Content        string  `json:"content"`
	Channel        *string `json:"channel,omitempty"`
	ToolCallID     *string `json:"tool_call_id,omitempty"`
	ToolCallsJSON  *string `json:"tool_calls,omitempty"`
	// ProviderStateJSON is opaque reasoning state for an assistant message,
	// stored and replayed but never read here. See core/llm.ProviderState.
	ProviderStateJSON *string `json:"provider_state,omitempty"`
	// PartsJSON is non-text content on the message, stored as the canonical
	// part list. Content remains the text describing it.
	PartsJSON *string `json:"parts,omitempty"`
	CreatedAt int64   `json:"created_at"`
}

// AppendMessageInput is one message to store.
//
// It is a struct because the column set grows as the LLM contract does, and a
// positional list this long stops saying which nil means what.
type AppendMessageInput struct {
	ConversationID string
	Role           string
	Content        string
	Channel        *string
	ToolCallID     *string
	ToolCallsJSON  *string
	// ProviderStateJSON is set only for an assistant message from a protocol
	// that produced reasoning state.
	ProviderStateJSON *string
	// PartsJSON is set when the message carries non-text content.
	PartsJSON *string
}

// ConversationStore provides Tier 1 conversation persistence. Conversations are user-scoped.
type ConversationStore interface {
	CreateConversation(ctx context.Context, userID, channel, createdBy string) (*Conversation, error)
	CreateConversationInTeam(ctx context.Context, teamID, userID, channel, createdBy string) (*Conversation, error)
	GetConversation(ctx context.Context, conversationID string) (*Conversation, error)
	ListConversationsByUser(ctx context.Context, userID string, limit, offset int) ([]Conversation, int, error)
	ListConversationsByTeam(ctx context.Context, teamID string, limit, offset int) ([]Conversation, int, error)
	UpdateConversationTitle(ctx context.Context, conversationID, title string) error
}

// ConversationMessageStore provides Tier 1 conversation message persistence.
// For role=assistant with tool calls, toolCallsJSON should be the JSON-encoded array of tool calls (id, name, arguments).
type ConversationMessageStore interface {
	AppendMessage(ctx context.Context, in AppendMessageInput) (*ConversationMessage, error)
	ListMessages(ctx context.Context, conversationID string) ([]ConversationMessage, error)
	// GetMessage returns one message by handle, or (nil, nil) when there is no
	// such row. It exists because a run names the message that asked for it,
	// and reading a whole transcript to resolve one handle is the wrong shape
	// for a question about one run.
	GetMessage(ctx context.Context, messageID string) (*ConversationMessage, error)
}
