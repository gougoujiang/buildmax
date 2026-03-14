package adapter

import (
	"context"

	"buildmax/internal/tools"
)

// StartChatFunc creates a chat and its first run. Same signature as server's doStartChat.
// conversationID is optional; when non-nil the chat record stores it (e.g. for Tier 1).
type StartChatFunc func(ctx context.Context, workspaceID, userID, input string, agentID *string, conversationID *string) (chatID, runID string, err error)

// NewStartChatRunner returns a tools.StartChatRunner that calls fn with the bound
// conversationID when StartChat is invoked. Use empty conversationID when not in a Tier 1 flow.
func NewStartChatRunner(fn StartChatFunc, workspaceID, userID, conversationID string) tools.StartChatRunner {
	if fn == nil {
		return nil
	}
	return &startChatRunner{
		fn:              fn,
		workspaceID:     workspaceID,
		userID:          userID,
		conversationID:  conversationID,
	}
}

type startChatRunner struct {
	fn             StartChatFunc
	workspaceID    string
	userID         string
	conversationID string
}

func (r *startChatRunner) StartChat(ctx context.Context, _, _, input string, agentID *string) (chatID, runID string, err error) {
	var convID *string
	if r.conversationID != "" {
		convID = &r.conversationID
	}
	return r.fn(ctx, r.workspaceID, r.userID, input, agentID, convID)
}
