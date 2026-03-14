// Package adapter provides portal and conversation adapters for the HTTP server.
// See design/007-two-tier-agent.md and .vibe/092-design.md.
package adapter

import (
	"context"
	"errors"
	"fmt"

	"buildmax/internal/conversation"
)

// PortalTurnInput is the request-shaped input for the portal adapter. The handler
// builds it from the HTTP create-run request and passes it to PortalAdapter.Receive.
type PortalTurnInput struct {
	WorkspaceID string
	ChatID      string
	UserID      string
	Message     string
}

// PortalAdapter implements conversation.ChannelAdapter for the portal (HTTP). It
// normalizes create-run request data into a ConversationTurn. Send is a no-op in Phase 2.
type PortalAdapter struct{}

// Receive normalizes raw input into a ConversationTurn. raw must be *PortalTurnInput;
// returns an error if raw is nil or not the expected type.
func (PortalAdapter) Receive(ctx context.Context, raw any) (conversation.ConversationTurn, error) {
	if raw == nil {
		return conversation.ConversationTurn{}, errors.New("portal adapter: raw input is nil")
	}
	input, ok := raw.(*PortalTurnInput)
	if !ok {
		return conversation.ConversationTurn{}, fmt.Errorf("portal adapter: expected *PortalTurnInput, got %T", raw)
	}
	return conversation.ConversationTurn{
		WorkspaceID:     input.WorkspaceID,
		Channel:         conversation.ChannelPortal,
		ConversationID:  input.ChatID,
		UserID:          input.UserID,
		Message:         input.Message,
		Raw:             nil,
	}, nil
}

// Send delivers output to the channel. For Phase 2 pass-through, the create-run path
// does not call Send; implementation is a no-op.
func (PortalAdapter) Send(ctx context.Context, conversationID string, output string) error {
	return nil
}
