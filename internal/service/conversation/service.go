package conversation

import (
	"context"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/core/apierr"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	coreconv "github.com/gougoujiang/buildmax/internal/core/conversation"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	convchannel "github.com/gougoujiang/buildmax/internal/service/conversation/channel"
	"github.com/gougoujiang/buildmax/internal/service/task"
)

var (
	ErrInvalidTarget = apierr.New(apierr.KindInvalid, "invalid conversation target")
	ErrLLMRequired   = apierr.New(apierr.KindNotConfigured, "conversation LLM not configured")
)

// Service is the single Tier 1 orchestration entry point for portal turns.
type Service struct {
	TaskService       *task.Service
	ConversationStore coreconv.Store
	MessageStore      coreconv.MessageStore
	LLMClient         llm.LLMClient
	TitleGenerator    llm.TitleGenerator
	AgentStore        agentdef.Store
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
	createdByType := coretask.RunCreatedByTypeUser
	triggerSource := coretask.RunTriggerSourcePortalTaskRerun
	if cmd.Channel == convchannel.ChannelWebhook {
		createdByType = coretask.RunCreatedByTypeWebhook
		triggerSource = coretask.RunTriggerSourceWebhook
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
	return ConversationResult{Runs: []SpawnedRun{{TaskID: cmd.TaskID, RunID: run.ID}}}, nil
}

func (s *Service) handleConversationTurn(ctx context.Context, cmd HandleTurnCmd) (ConversationResult, error) {
	if s.ConversationStore == nil || s.MessageStore == nil {
		return ConversationResult{}, fmt.Errorf("conversation stores not configured")
	}
	if s.LLMClient == nil {
		return ConversationResult{}, ErrLLMRequired
	}

	teamID := s.fetchTeamID(ctx, cmd.ConversationID, cmd.Channel)

	runInput := turnRunInput{
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
	reply, err := runConversationTurn(ctx, s.ConversationStore, s.MessageStore, s.LLMClient, runInput)
	return ConversationResult{Reply: reply}, err
}

func (s *Service) taskServiceForChannel(channel string) *task.Service {
	if channel == convchannel.ChannelSystem {
		return nil
	}
	return s.TaskService
}

// fetchTeamID looks up the conversation's team once so StartTask and agent listing share it.
// Returns "" when the channel is system or no TaskService is configured (task tools disabled).
func (s *Service) fetchTeamID(ctx context.Context, conversationID, channel string) string {
	if channel == convchannel.ChannelSystem || s.TaskService == nil || s.ConversationStore == nil {
		return ""
	}
	conv, err := s.ConversationStore.GetConversation(ctx, conversationID)
	if err != nil || conv == nil {
		return ""
	}
	return conv.TeamID
}

func (s *Service) fetchAgentSummaries(ctx context.Context, teamID, channel string) []agentSummary {
	if s.AgentStore == nil || teamID == "" || channel == convchannel.ChannelSystem {
		return nil
	}
	agents, err := s.AgentStore.ListAgentsByTeam(ctx, teamID)
	if err != nil || len(agents) == 0 {
		return nil
	}
	summaries := make([]agentSummary, len(agents))
	for i, a := range agents {
		summaries[i] = agentSummary{ID: a.ID, Name: a.Name, Description: a.Description}
	}
	return summaries
}
