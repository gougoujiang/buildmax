package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	return ConversationResult{TaskIDs: []string{run.TaskRunID}}, nil
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

	runInput := RunInput{
		ConversationID:     cmd.ConversationID,
		UserContent:        cmd.Message,
		Channel:            cmd.Channel,
		ScopeID:            cmd.ConversationID,
		UserID:             cmd.UserID,
		Runners:            runners,
		TitleGenerator:     s.TitleGenerator,
		RecentChatsSnippet: s.recentTasksSnippet(ctx, cmd.ConversationID),
		StreamSink:         cmd.StreamSink,
	}
	reply, err := Run(ctx, s.ConversationStore, s.MessageStore, s.LLMClient, runInput)
	return ConversationResult{Reply: reply}, err
}

func (s *ConversationService) conversationToolRunners(conversationID, userID, channel string) *ConversationToolRunners {
	if channel == ChannelSystem {
		return nil
	}
	return &ConversationToolRunners{
		StartTask:    s.newStartTaskRunner(conversationID, userID),
		ListTasks:    s.newListTasksRunner(),
		GetTask:      s.newGetTaskRunner(),
		ContinueTask: s.newContinueTaskRunner(),
	}
}

func (s *ConversationService) newStartTaskRunner(conversationID, userID string) StartTaskRunner {
	if s.TaskService == nil || s.ConversationStore == nil {
		return nil
	}
	return &startTaskRunner{
		taskService:       s.TaskService,
		conversationStore: s.ConversationStore,
		userID:            userID,
		conversationID:    conversationID,
	}
}

func (s *ConversationService) newListTasksRunner() ListTasksRunner {
	if s.TaskService == nil || s.TaskService.Tasks == nil {
		return nil
	}
	return &listTasksRunner{tasks: s.TaskService.Tasks}
}

func (s *ConversationService) newGetTaskRunner() GetTaskRunner {
	if s.TaskService == nil || s.TaskService.Tasks == nil {
		return nil
	}
	return &getTaskRunner{tasks: s.TaskService.Tasks}
}

func (s *ConversationService) newContinueTaskRunner() ContinueTaskRunner {
	if s.TaskService == nil {
		return nil
	}
	return &continueTaskRunner{taskService: s.TaskService}
}

func (s *ConversationService) fetchAgentSummaries(ctx context.Context, userID, conversationID string) []AgentSummary {
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
	summaries := make([]AgentSummary, len(agents))
	for i, a := range agents {
		summaries[i] = AgentSummary{ID: a.AgentID, Name: a.Name, Description: a.Description}
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
		snippet := taskTitleOrSnippet(&c, 60)
		lines = append(lines, fmt.Sprintf("%s | %s | %s | %s", c.TaskID, snippet, c.Status, formatCreatedAt(c.CreatedAt)))
	}
	return "Recent tasks in this conversation (latest 5):\n" + strings.Join(lines, "\n")
}
