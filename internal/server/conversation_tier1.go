// Tier 1 conversation implementations for the portal: adapter and pass-through engine.
// See design/007-two-tier-agent.md and .vibe/092-design.md.
package server

import (
	"context"
	"errors"
	"fmt"

	"buildmax/internal/conversation"
	"buildmax/internal/storage/entity"
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

// PassThroughEngine implements conversation.ConversationEngine. It always creates
// exactly one chat run with the turn message and returns its chat_run_id in TaskIDs.
type PassThroughEngine struct {
	chatRuns entity.ChatRunStore
}

// NewPassThroughEngine returns a pass-through conversation engine that uses the
// given ChatRunStore to create runs.
func NewPassThroughEngine(chatRuns entity.ChatRunStore) *PassThroughEngine {
	return &PassThroughEngine{chatRuns: chatRuns}
}

// Process creates one chat run with turn.Message as input and returns its
// chat_run_id in ConversationResult.TaskIDs. On error (e.g. ErrRunInProgress),
// returns (zero result, err).
func (e *PassThroughEngine) Process(ctx context.Context, workspaceID, chatID string, turn conversation.ConversationTurn) (conversation.ConversationResult, error) {
	run, err := e.chatRuns.CreateChatRun(ctx, chatID, turn.Message, turn.UserID)
	if err != nil {
		return conversation.ConversationResult{}, err
	}
	return conversation.ConversationResult{
		TaskIDs: []string{run.ChatRunID},
	}, nil
}
