package conversation

import (
	"context"
	"errors"

	"buildmax/internal/core/model"
	taskapp "buildmax/internal/core/task"
)

var ErrChannelNotWebhook = errors.New("rule-based engine only accepts webhook channel")

// RuleBasedEngine implements ConversationEngine for webhook turns: no LLM, create one TaskRun.
type RuleBasedEngine struct {
	Task          *taskapp.Service
	Conversations model.ConversationStore
}

// Process creates exactly one TaskRun for webhook turns. Rejects other channels.
func (e *RuleBasedEngine) Process(ctx context.Context, conversationID, taskID string, turn ConversationTurn) (ConversationResult, error) {
	if turn.Channel != ChannelWebhook {
		return ConversationResult{}, ErrChannelNotWebhook
	}
	if e.Task == nil {
		return ConversationResult{}, taskapp.ErrTaskRunsNotConfigured
	}
	if e.Conversations == nil {
		return ConversationResult{}, errors.New("conversation store not configured")
	}
	if conversationID == "" {
		return ConversationResult{}, errors.New("conversation_id required")
	}
	if turn.Message == "" {
		return ConversationResult{}, errors.New("message required")
	}
	userID := turn.UserID
	if userID == "" {
		userID = DefaultWebhookUserID
	}
	conv, err := e.Conversations.GetConversation(ctx, conversationID)
	if err != nil {
		return ConversationResult{}, err
	}
	if conv == nil {
		return ConversationResult{}, errors.New("conversation not found")
	}
	if taskID == "" {
		result, err := e.Task.StartBackgroundTask(ctx, taskapp.CreateTaskCmd{
			ConversationID: conversationID,
			UserID:         userID,
			TeamID:         conv.TeamID,
			Input:          turn.Message,
			AgentID:        nil,
			CreatedByType:  model.RunCreatedByTypeWebhook,
			TriggerSource:  model.RunTriggerSourceWebhook,
		})
		if err != nil {
			return ConversationResult{}, err
		}
		return ConversationResult{TaskIDs: []string{result.RunID}}, nil
	}
	run, err := e.Task.CreateRun(ctx, taskapp.CreateRunCmd{
		UserID:        userID,
		TaskID:        taskID,
		Input:         turn.Message,
		CreatedByType: model.RunCreatedByTypeWebhook,
		TriggerSource: model.RunTriggerSourceWebhook,
	})
	if err != nil {
		return ConversationResult{}, err
	}
	return ConversationResult{TaskIDs: []string{run.TaskRunID}}, nil
}
