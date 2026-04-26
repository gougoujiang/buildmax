package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	taskapp "buildmax/internal/app/task"
	coreconv "buildmax/internal/conversation"
	"buildmax/internal/llm"
	"buildmax/internal/storage/entity"
	"buildmax/internal/tools"
)

var (
	ErrInvalidTarget = errors.New("conversation turn target required")
	ErrLLMRequired   = errors.New("conversation LLM not configured")
)

// TitleGenerator generates a conversation title from the first user message.
type TitleGenerator interface {
	GenerateTitle(ctx context.Context, input string) (string, error)
}

// Service is the single Tier 1 orchestration entry point for portal turns.
type Service struct {
	TaskService       *taskapp.Service
	ConversationStore entity.ConversationStore
	MessageStore      entity.ConversationMessageStore
	LLMCaller         llm.LLMCaller
	TitleGenerator    TitleGenerator
	AgentStore        entity.AgentStore
}

// HandleTurnCmd describes one normalized portal turn.
type HandleTurnCmd struct {
	UserID         string
	Channel        string
	Message        string
	TaskID         string
	ConversationID string
	StreamSink     llm.StreamSink
}

// HandleTurnResult returns either a direct reply and/or spawned task ids.
type HandleTurnResult struct {
	Reply   string
	TaskIDs []string
}

// HandleTurn routes the turn to either a task-run creation flow or a conversation LLM flow.
func (s *Service) HandleTurn(ctx context.Context, cmd HandleTurnCmd) (HandleTurnResult, error) {
	switch {
	case cmd.TaskID != "":
		return s.handleTaskRunTurn(ctx, cmd)
	case cmd.ConversationID != "":
		return s.handleConversationTurn(ctx, cmd)
	default:
		return HandleTurnResult{}, ErrInvalidTarget
	}
}

func (s *Service) handleTaskRunTurn(ctx context.Context, cmd HandleTurnCmd) (HandleTurnResult, error) {
	if s.TaskService == nil {
		return HandleTurnResult{}, taskapp.ErrTaskRunsNotConfigured
	}
	run, err := s.TaskService.CreateRun(ctx, taskapp.CreateRunCmd{
		UserID: cmd.UserID,
		TaskID: cmd.TaskID,
		Input:  cmd.Message,
	})
	if err != nil {
		return HandleTurnResult{}, err
	}
	return HandleTurnResult{TaskIDs: []string{run.TaskRunID}}, nil
}

func (s *Service) handleConversationTurn(ctx context.Context, cmd HandleTurnCmd) (HandleTurnResult, error) {
	if s.ConversationStore == nil || s.MessageStore == nil {
		return HandleTurnResult{}, fmt.Errorf("conversation stores not configured")
	}
	if s.LLMCaller == nil {
		return HandleTurnResult{}, ErrLLMRequired
	}

	runners := s.conversationToolRunners(cmd.ConversationID, cmd.UserID)
	runners.AgentSummaries = s.fetchAgentSummaries(ctx, cmd.UserID, cmd.ConversationID)

	runInput := coreconv.RunInput{
		ConversationID:     cmd.ConversationID,
		UserContent:        cmd.Message,
		Channel:            cmd.Channel,
		ScopeID:            cmd.ConversationID,
		UserID:             cmd.UserID,
		Runners:            runners,
		TitleGenerator:     titleGeneratorAdapter{s.TitleGenerator},
		RecentChatsSnippet: s.recentTasksSnippet(ctx, cmd.ConversationID),
		StreamSink:         cmd.StreamSink,
	}
	reply, err := coreconv.Run(ctx, s.ConversationStore, s.MessageStore, s.LLMCaller, runInput)
	return HandleTurnResult{Reply: reply}, err
}

func (s *Service) conversationToolRunners(conversationID, userID string) *coreconv.ConversationToolRunners {
	return &coreconv.ConversationToolRunners{
		StartTask:    s.startTaskRunner(conversationID, userID),
		ListTasks:    s.listTasksRunner(),
		GetTask:      s.getTaskRunner(),
		ContinueTask: s.continueTaskRunner(),
	}
}

func (s *Service) startTaskRunner(conversationID, userID string) tools.StartTaskRunner {
	if s.TaskService == nil {
		return nil
	}
	return &startTaskRunner{
		taskService:    s.TaskService,
		userID:         userID,
		conversationID: conversationID,
	}
}

type startTaskRunner struct {
	taskService    *taskapp.Service
	userID         string
	conversationID string
}

func (r *startTaskRunner) StartTask(ctx context.Context, _, _, input string, agentID *string) (taskID, runID string, err error) {
	result, err := r.taskService.StartBackgroundTask(ctx, taskapp.CreateTaskCmd{
		ConversationID: r.conversationID,
		UserID:         r.userID,
		Input:          input,
		AgentID:        agentID,
	})
	if err != nil {
		return "", "", err
	}
	return result.TaskID, result.RunID, nil
}

func (s *Service) listTasksRunner() tools.ListTasksRunner {
	if s.TaskService == nil || s.TaskService.Tasks == nil {
		return nil
	}
	return &listTasksRunner{tasks: s.TaskService.Tasks}
}

type listTasksRunner struct {
	tasks entity.TaskStore
}

func (r *listTasksRunner) ListTasks(ctx context.Context, conversationID string) (string, error) {
	list, _, err := r.tasks.ListTasksByConversationPaginated(ctx, conversationID, false, 10, 0)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "No recent tasks in this conversation.", nil
	}
	var lines []string
	for i, c := range list {
		snippet := taskTitleOrSnippet(&c, 60)
		ts := formatCreatedAt(c.CreatedAt)
		lines = append(lines, fmt.Sprintf("%d. %s | %s | %s | %s", i+1, c.TaskID, snippet, c.Status, ts))
	}
	return strings.Join(lines, "\n"), nil
}

func (s *Service) getTaskRunner() tools.GetTaskRunner {
	if s.TaskService == nil || s.TaskService.Tasks == nil {
		return nil
	}
	return &getTaskRunner{tasks: s.TaskService.Tasks}
}

type getTaskRunner struct {
	tasks entity.TaskStore
}

func (r *getTaskRunner) GetTask(ctx context.Context, conversationID, taskID string) (string, error) {
	task, err := r.tasks.GetTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	if task == nil {
		return "", fmt.Errorf("task not found or not in this conversation")
	}
	if task.ConversationID != conversationID {
		return "", fmt.Errorf("task not found or not in this conversation")
	}
	inputTrunc := truncateRunes(task.Input, 500)
	outputLine := ""
	if task.Output != nil && *task.Output != "" {
		outputLine = "output_snippet: " + truncateRunes(*task.Output, 200) + "\n"
	}
	lastRun := ""
	if task.LastRunID != nil {
		lastRun = *task.LastRunID
	}
	return fmt.Sprintf("task_id: %s\ntitle: %s\ninput: %s\nstatus: %s\ncreated_at: %s\nlast_run_id: %s\n%s",
		task.TaskID, task.Title, inputTrunc, task.Status, formatCreatedAt(task.CreatedAt), lastRun, outputLine), nil
}

func (s *Service) continueTaskRunner() tools.ContinueTaskRunner {
	if s.TaskService == nil {
		return nil
	}
	return &continueTaskRunner{taskService: s.TaskService}
}

type continueTaskRunner struct {
	taskService *taskapp.Service
}

func (r *continueTaskRunner) ContinueTask(ctx context.Context, conversationID, userID, taskID, input string) (runID string, err error) {
	if r.taskService.Tasks == nil {
		return "", fmt.Errorf("tasks not configured")
	}
	task, err := r.taskService.Tasks.GetTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	if task == nil || task.ConversationID != conversationID {
		return "", fmt.Errorf("task not found or not in this conversation")
	}
	run, err := r.taskService.CreateRun(ctx, taskapp.CreateRunCmd{
		UserID: userID,
		TaskID: taskID,
		Input:  input,
	})
	if err != nil {
		return "", err
	}
	return run.TaskRunID, nil
}

func (s *Service) fetchAgentSummaries(ctx context.Context, userID, conversationID string) []tools.AgentSummary {
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
	summaries := make([]tools.AgentSummary, len(agents))
	for i, a := range agents {
		summaries[i] = tools.AgentSummary{ID: a.AgentID, Name: a.Name, Description: a.Description}
	}
	return summaries
}

func (s *Service) recentTasksSnippet(ctx context.Context, conversationID string) string {
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

func taskTitleOrSnippet(task *entity.Task, maxRunes int) string {
	if task.Title != "" {
		return truncateRunes(task.Title, maxRunes)
	}
	return truncateRunes(task.Input, maxRunes)
}

func truncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "…"
}

func formatCreatedAt(unixSec int64) string {
	return time.Unix(unixSec, 0).Format("2006-01-02 15:04")
}

type titleGeneratorAdapter struct {
	gen TitleGenerator
}

func (a titleGeneratorAdapter) GenerateTitleFromInput(ctx context.Context, input string) (string, error) {
	if a.gen == nil {
		return "", nil
	}
	return a.gen.GenerateTitle(ctx, input)
}
