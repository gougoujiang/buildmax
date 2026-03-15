package conversation

import (
	"context"
	"errors"

	chatapp "buildmax/internal/app/chat"
	coreconv "buildmax/internal/conversation"
)

var ErrChannelNotWebhook = errors.New("rule-based engine only accepts webhook channel")

// RuleBasedEngine implements coreconv.ConversationEngine for webhook turns: no LLM, create one TaskRun.
type RuleBasedEngine struct {
	Chat *chatapp.Service
}

// Process creates exactly one TaskRun for webhook turns. Rejects other channels.
func (e *RuleBasedEngine) Process(ctx context.Context, conversationID, chatID string, turn coreconv.ConversationTurn) (coreconv.ConversationResult, error) {
	if turn.Channel != coreconv.ChannelWebhook {
		return coreconv.ConversationResult{}, ErrChannelNotWebhook
	}
	if e.Chat == nil {
		return coreconv.ConversationResult{}, chatapp.ErrTaskRunsNotConfigured
	}
	if conversationID == "" {
		return coreconv.ConversationResult{}, errors.New("conversation_id required")
	}
	if turn.Message == "" {
		return coreconv.ConversationResult{}, errors.New("message required")
	}
	userID := turn.UserID
	if userID == "" {
		userID = coreconv.DefaultWebhookUserID
	}
	if chatID == "" {
		result, err := e.Chat.StartBackgroundChat(ctx, chatapp.CreateChatCmd{
			ConversationID: conversationID,
			UserID:         userID,
			Input:          turn.Message,
			AgentID:        nil,
		})
		if err != nil {
			return coreconv.ConversationResult{}, err
		}
		return coreconv.ConversationResult{TaskIDs: []string{result.RunID}}, nil
	}
	run, err := e.Chat.CreateRun(ctx, chatapp.CreateRunCmd{
		UserID: userID,
		ChatID: chatID,
		Input:  turn.Message,
	})
	if err != nil {
		return coreconv.ConversationResult{}, err
	}
	return coreconv.ConversationResult{TaskIDs: []string{run.TaskRunID}}, nil
}
