package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	convruntime "buildmax/internal/core/conversation/runtime"
	convtool "buildmax/internal/core/conversation/tool"
	"buildmax/internal/core/model"
	"buildmax/internal/core/task"
)

var (
	ErrInvalidTarget = errors.New("conversation turn target required")
	ErrLLMRequired   = errors.New("conversation LLM not configured")
)

// ConversationService is the single Tier 1 orchestration entry point for portal turns.
type ConversationService struct {
	TaskService       *task.TaskService
	ConversationStore model.ConversationStore
	MessageStore      model.ConversationMessageStore
	LLMClient         model.LLMClient
	TitleGenerator    model.TitleGenerator
	AgentStore        model.AgentStore
}

// HandleTurnCmd describes one normalized portal turn.
type HandleTurnCmd struct {
	UserID         string
	Channel        string
	Message        string
	TaskID         string
	ConversationID string
	StreamSink     model.StreamSink
}

// HandleTurn routes the turn to either a task-run creation flow or a conversation LLM flow.
func (s *ConversationService) HandleTurn(ctx context.Context, cmd HandleTurnCmd) (ConversationResult, error) {
	switch {
	case cmd.TaskID != "":
		return s.handleTaskRunTurn(ctx, cmd)
	case cmd.ConversationID != "":
		return s.handleConversationTurn(ctx, cmd)
	default:
		return ConversationResult{}, ErrInvalidTarget
	}
}

func (s *ConversationService) handleTaskRunTurn(ctx context.Context, cmd HandleTurnCmd) (ConversationResult, error) {
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

func (s *ConversationService) handleConversationTurn(ctx context.Context, cmd HandleTurnCmd) (ConversationResult, error) {
	if s.ConversationStore == nil || s.MessageStore == nil {
		return ConversationResult{}, fmt.Errorf("conversation stores not configured")
	}
	if s.LLMClient == nil {
		return ConversationResult{}, ErrLLMRequired
	}

	runners := s.conversationToolRunners(cmd.ConversationID, cmd.UserID, cmd.Channel)
	if runners != nil {
		runners.AgentSummaries = s.fetchAgentSummaries(ctx, cmd.UserID, cmd.ConversationID)
	}

	runInput := convruntime.TurnRunInput{
		ConversationID:      cmd.ConversationID,
		Message:             cmd.Message,
		Channel:             cmd.Channel,
		ConversationScopeID: cmd.ConversationID,
		UserID:              cmd.UserID,
		Runners:             runners,
		TitleGenerator:      s.TitleGenerator,
		RecentTasksSnippet:  s.recentTasksSnippet(ctx, cmd.ConversationID),
		StreamSink:          cmd.StreamSink,
	}
	reply, err := convruntime.Run(ctx, s.ConversationStore, s.MessageStore, s.LLMClient, runInput)
	return ConversationResult{Reply: reply}, err
}

func (s *ConversationService) conversationToolRunners(conversationID, userID, channel string) *convruntime.TurnToolRunners {
	if channel == ChannelSystem {
		return nil
	}
	return &convruntime.TurnToolRunners{
		StartTask:    s.newStartTaskRunner(conversationID, userID),
		ListTasks:    s.newListTasksRunner(),
		GetTask:      s.newGetTaskRunner(),
		ContinueTask: s.newContinueTaskRunner(),
	}
}

func (s *ConversationService) newStartTaskRunner(conversationID, userID string) convtool.StartTaskRunner {
	return convtool.NewStartTaskServiceRunner(s.TaskService, s.ConversationStore, conversationID, userID)
}

func (s *ConversationService) newListTasksRunner() convtool.ListTasksRunner {
	if s.TaskService == nil {
		return nil
	}
	return convtool.NewListTasksStoreRunner(s.TaskService.Tasks)
}

func (s *ConversationService) newGetTaskRunner() convtool.GetTaskRunner {
	if s.TaskService == nil {
		return nil
	}
	return convtool.NewGetTaskStoreRunner(s.TaskService.Tasks)
}

func (s *ConversationService) newContinueTaskRunner() convtool.ContinueTaskRunner {
	return convtool.NewContinueTaskServiceRunner(s.TaskService)
}

func (s *ConversationService) fetchAgentSummaries(ctx context.Context, userID, conversationID string) []convtool.AgentSummary {
	if s.AgentStore == nil {
		return nil
	}
	agents, err := s.AgentStore.ListAgentsByUser(ctx, userID)
	if s.ConversationStore != nil && conversationID != "" {
		conv, convErr := s.ConversationStore.GetConversation(ctx, conversationID)
		if convErr == nil && conv != nil && conv.TeamID != "" {
			agents, err = s.AgentStore.ListAgentsByTeam(ctx, conv.TeamID)
		}
	}
	if err != nil || len(agents) == 0 {
		return nil
	}
	summaries := make([]convtool.AgentSummary, len(agents))
	for i, a := range agents {
		summaries[i] = convtool.AgentSummary{ID: a.AgentID, Name: a.Name, Description: a.Description}
	}
	return summaries
}

func (s *ConversationService) recentTasksSnippet(ctx context.Context, conversationID string) string {
	if s.TaskService == nil || s.TaskService.Tasks == nil {
		return ""
	}
	list, _, err := s.TaskService.Tasks.ListTasksByConversationPaginated(ctx, conversationID, false, 5, 0)
	if err != nil || len(list) == 0 {
		return "Recent tasks in this conversation (latest 5): No recent tasks."
	}
	var lines []string
	for _, c := range list {
		snippet := recentTaskTitleOrSnippet(&c, 60)
		lines = append(lines, fmt.Sprintf("%s | %s | %s | %s", c.TaskID, snippet, c.Status, formatRecentTaskCreatedAt(c.CreatedAt)))
	}
	return "Recent tasks in this conversation (latest 5):\n" + strings.Join(lines, "\n")
}

func recentTaskTitleOrSnippet(taskItem *model.Task, maxRunes int) string {
	if taskItem.Title != "" {
		return truncateRecentTaskRunes(taskItem.Title, maxRunes)
	}
	return truncateRecentTaskRunes(taskItem.Input, maxRunes)
}

func truncateRecentTaskRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

func formatRecentTaskCreatedAt(unixSec int64) string {
	return time.Unix(unixSec, 0).Format("2006-01-02 15:04")
}
