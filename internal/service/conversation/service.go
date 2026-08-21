package conversation

import (
	"context"
	"fmt"
	"github.com/gougoujiang/buildmax/internal/core/apierr"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	convruntime "github.com/gougoujiang/buildmax/internal/service/conversation/runtime"
	convtool "github.com/gougoujiang/buildmax/internal/service/conversation/tool"
	"github.com/gougoujiang/buildmax/internal/service/task"
)

var (
	ErrInvalidTarget = apierr.New(apierr.KindInvalid, "invalid conversation target")
	ErrLLMRequired   = apierr.New(apierr.KindNotConfigured, "conversation LLM not configured")
)

// Service is the single Tier 1 orchestration entry point for portal turns.
type Service struct {
	TaskService       *task.Service
	ConversationStore model.ConversationStore
	MessageStore      model.ConversationMessageStore
	LLMClient         llm.LLMClient
	TitleGenerator    llm.TitleGenerator
	AgentStore        model.AgentStore
}

// HandleTurnCmd describes one normalized portal conversation turn.
type HandleTurnCmd struct {
	UserID         string
	Channel        string
	Message        string
	ConversationID string
	StreamSink     llm.StreamSink
}

// RerunTaskCmd describes a direct task-rerun request (bypasses the LLM layer).
type RerunTaskCmd struct {
	UserID  string
	Channel string
	Message string
	TaskID  string
}

// HandleTurn runs one conversation turn through the Tier 1 LLM loop.
func (s *Service) HandleTurn(ctx context.Context, cmd HandleTurnCmd) (ConversationResult, error) {
	if cmd.ConversationID == "" {
		return ConversationResult{}, ErrInvalidTarget
	}
	return s.handleConversationTurn(ctx, cmd)
}

// RerunTask creates a new task run for an existing task, bypassing the LLM layer.
func (s *Service) RerunTask(ctx context.Context, cmd RerunTaskCmd) (ConversationResult, error) {
	if s.TaskService == nil {
		return ConversationResult{}, task.ErrTaskRunsNotConfigured
	}
	createdByType := model.RunCreatedByTypeUser
	triggerSource := model.RunTriggerSourcePortalTaskRerun
	if cmd.Channel == ChannelWebhook {
		createdByType = model.RunCreatedByTypeWebhook
		triggerSource = model.RunTriggerSourceWebhook
	}
	run, err := s.TaskService.CreateRun(ctx, task.CreateRunCmd{
		UserID:        cmd.UserID,
		TaskID:        cmd.TaskID,
		Input:         cmd.Message,
		CreatedByType: createdByType,
		TriggerSource: triggerSource,
	})
	if err != nil {
		return ConversationResult{}, err
	}
	return ConversationResult{TaskRunIDs: []string{run.TaskRunID}}, nil
}

func (s *Service) handleConversationTurn(ctx context.Context, cmd HandleTurnCmd) (ConversationResult, error) {
	if s.ConversationStore == nil || s.MessageStore == nil {
		return ConversationResult{}, fmt.Errorf("conversation stores not configured")
	}
	if s.LLMClient == nil {
		return ConversationResult{}, ErrLLMRequired
	}

	teamID := s.fetchTeamID(ctx, cmd.ConversationID, cmd.Channel)

	runInput := convruntime.TurnRunInput{
		ConversationID: cmd.ConversationID,
		Message:        cmd.Message,
		Channel:        cmd.Channel,
		UserID:         cmd.UserID,
		TeamID:         teamID,
		TaskService:    s.taskServiceForChannel(cmd.Channel),
		AgentSummaries: s.fetchAgentSummaries(ctx, teamID, cmd.Channel),
		TitleGenerator: s.TitleGenerator,
		StreamSink:     cmd.StreamSink,
	}
	reply, err := convruntime.Run(ctx, s.ConversationStore, s.MessageStore, s.LLMClient, runInput)
	return ConversationResult{Reply: reply}, err
}

func (s *Service) taskServiceForChannel(channel string) *task.Service {
	if channel == ChannelSystem {
		return nil
	}
	return s.TaskService
}

// fetchTeamID looks up the conversation's team once so StartTask and agent listing share it.
// Returns "" when the channel is system or no TaskService is configured (task tools disabled).
func (s *Service) fetchTeamID(ctx context.Context, conversationID, channel string) string {
	if channel == ChannelSystem || s.TaskService == nil || s.ConversationStore == nil {
		return ""
	}
	conv, err := s.ConversationStore.GetConversation(ctx, conversationID)
	if err != nil || conv == nil {
		return ""
	}
	return conv.TeamID
}

func (s *Service) fetchAgentSummaries(ctx context.Context, teamID, channel string) []convtool.AgentSummary {
	if s.AgentStore == nil || teamID == "" || channel == ChannelSystem {
		return nil
	}
	agents, err := s.AgentStore.ListAgentsByTeam(ctx, teamID)
	if err != nil || len(agents) == 0 {
		return nil
	}
	summaries := make([]convtool.AgentSummary, len(agents))
	for i, a := range agents {
		summaries[i] = convtool.AgentSummary{ID: a.AgentID, Name: a.Name, Description: a.Description}
	}
	return summaries
}
