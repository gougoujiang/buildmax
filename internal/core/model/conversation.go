package model

import "context"

// Conversation is the Tier 1 conversation container.
type Conversation struct {
	ID             uint   `json:"-"`
	ConversationID string `json:"conversation_id"`
	UserID         string `json:"user_id"`
	TeamID         string `json:"team_id,omitempty"`
	Channel        string `json:"channel"`
	Title          string `json:"title,omitempty"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      int64  `json:"created_at"`
}

// ConversationMessage is one message in a Tier 1 conversation.
type ConversationMessage struct {
	ID                    uint    `json:"-"`
	ConversationMessageID string  `json:"conversation_message_id"`
	ConversationID        string  `json:"conversation_id"`
	Role                  string  `json:"role"`
	Content               string  `json:"content"`
	Channel               *string `json:"channel,omitempty"`
	ToolCallID            *string `json:"tool_call_id,omitempty"`
	ToolCallsJSON         *string `json:"tool_calls,omitempty"`
	CreatedAt             int64   `json:"created_at"`
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
	AppendMessage(ctx context.Context, conversationID, role, content string, channel *string, toolCallID *string, toolCallsJSON *string) (*ConversationMessage, error)
	ListMessages(ctx context.Context, conversationID string) ([]ConversationMessage, error)
}
