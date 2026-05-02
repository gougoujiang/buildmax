package conversation

import (
	"context"
	"errors"

	taskapp "buildmax/internal/app/task"
	coreconv "buildmax/internal/conversation"
	"buildmax/internal/storage/entity"
)

var ErrChannelNotWebhook = errors.New("rule-based engine only accepts webhook channel")

// RuleBasedEngine implements coreconv.ConversationEngine for webhook turns: no LLM, create one TaskRun.
type RuleBasedEngine struct {
	Task          *taskapp.Service
	Conversations entity.ConversationStore
}

// Process creates exactly one TaskRun for webhook turns. Rejects other channels.
func (e *RuleBasedEngine) Process(ctx context.Context, conversationID, taskID string, turn coreconv.ConversationTurn) (coreconv.ConversationResult, error) {
	if turn.Channel != coreconv.ChannelWebhook {
		return coreconv.ConversationResult{}, ErrChannelNotWebhook
	}
	if e.Task == nil {
		return coreconv.ConversationResult{}, taskapp.ErrTaskRunsNotConfigured
	}
	if e.Conversations == nil {
		return coreconv.ConversationResult{}, errors.New("conversation store not configured")
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
	conv, err := e.Conversations.GetConversation(ctx, conversationID)
	if err != nil {
		return coreconv.ConversationResult{}, err
	}
	if conv == nil {
		return coreconv.ConversationResult{}, errors.New("conversation not found")
	}
	if taskID == "" {
		result, err := e.Task.StartBackgroundTask(ctx, taskapp.CreateTaskCmd{
			ConversationID: conversationID,
			UserID:         userID,
			TeamID:         conv.TeamID,
			Input:          turn.Message,
			AgentID:        nil,
			CreatedByType:  entity.RunCreatedByTypeWebhook,
			TriggerSource:  entity.RunTriggerSourceWebhook,
		})
		if err != nil {
			return coreconv.ConversationResult{}, err
		}
		return coreconv.ConversationResult{TaskIDs: []string{result.RunID}}, nil
	}
	run, err := e.Task.CreateRun(ctx, taskapp.CreateRunCmd{
		UserID:        userID,
		TaskID:        taskID,
		Input:         turn.Message,
		CreatedByType: entity.RunCreatedByTypeWebhook,
		TriggerSource: entity.RunTriggerSourceWebhook,
	})
	if err != nil {
		return coreconv.ConversationResult{}, err
	}
	return coreconv.ConversationResult{TaskIDs: []string{run.TaskRunID}}, nil
}
