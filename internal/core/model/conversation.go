package model

import (
	"context"
	"time"
)

const (
	// ChannelWorkflow and ChannelIssueAgent mark a conversation nobody holds. A
	// workflow step and an issue agent run each need one because Task requires
	// a conversation, not because anyone is talking through it.
	//
	// They live here rather than with the transports in
	// service/conversation/channel because they are not transports: nothing
	// sends or receives through them, and the store has to know them to keep
	// them out of a list of conversations people hold.
	ChannelWorkflow   = "workflow"
	ChannelIssueAgent = "issue_agent"
)

// SyntheticChannels are the channels of conversations that exist only to
// satisfy the schema.
//
// They are kept out of the conversation list rather than deleted: a workflow
// run and an issue agent run still hold their transcript, and a link straight
// to one still opens it. What they must not do is push a person's own
// conversations off a page of the list.
//
// It is a deny-list rather than "anything that is not a transport" so that
// adding a real channel does not silently hide it.
var SyntheticChannels = []string{ChannelWorkflow, ChannelIssueAgent}

// Conversation is the Tier 1 conversation container.
type Conversation struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TeamID    string    `json:"team_id,omitempty"`
	Channel   string    `json:"channel"`
	Title     string    `json:"title,omitempty"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
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
	PartsJSON *string   `json:"parts,omitempty"`
	CreatedAt time.Time `json:"created_at"`
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
