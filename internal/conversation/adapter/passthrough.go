package adapter

import (
	"context"

	"buildmax/internal/conversation"
	"buildmax/internal/storage/entity"
)

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
