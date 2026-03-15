package task

import (
	"context"
	"errors"
	"fmt"

	"buildmax/internal/storage/entity"
)

const defaultTitleRunes = 50

var (
	ErrInputRequired         = errors.New("input required")
	ErrAgentsNotConfigured   = errors.New("agents not configured")
	ErrTasksNotConfigured    = errors.New("tasks not configured")
	ErrTaskRunsNotConfigured = errors.New("task runs not configured")
	ErrAgentNotFound         = errors.New("agent not found or not owned by user")
)

// QuotaChecker is the narrow quota surface needed by task workflows.
type QuotaChecker interface {
	Check(ctx context.Context, userID string, runsToAdd, tokensToAdd int) (allowed bool, reason string)
}

// TitleGenerator generates a title and token usage for a task input.
type TitleGenerator interface {
	GenerateTitle(ctx context.Context, input string) (title string, promptTokens, completionTokens int, err error)
}

// QuotaExceededError is returned when quota blocks a task operation.
type QuotaExceededError struct {
	Reason string
}

func (e *QuotaExceededError) Error() string {
	return "quota exceeded: " + e.Reason
}

// Service owns task-related application workflows.
type Service struct {
	Agents         entity.AgentStore
	Tasks          entity.TaskStore
	TaskRuns       entity.TaskRunStore
	QuotaChecker   QuotaChecker
	TitleGenerator TitleGenerator
}

// CreateTaskCmd creates a new task and its first run.
type CreateTaskCmd struct {
	ConversationID string
	UserID         string
	Input          string
	AgentID        *string
}

// CreateRunCmd creates a new run on an existing task.
type CreateRunCmd struct {
	UserID string
	TaskID string
	Input  string
}

// StartBackgroundTaskResult is returned when a background task is created.
type StartBackgroundTaskResult struct {
	TaskID string
	RunID  string
}

// CreateTask resolves input, applies title/quota rules, and persists a new task.
func (s *Service) CreateTask(ctx context.Context, cmd CreateTaskCmd) (*entity.Task, error) {
	if s.Tasks == nil {
		return nil, ErrTasksNotConfigured
	}
	input, agentID, err := s.resolveInput(ctx, cmd.UserID, cmd.Input, cmd.AgentID)
	if err != nil {
		return nil, err
	}
	title, promptTokens, completionTokens := s.resolveTitle(ctx, input)
	if err := s.checkQuota(ctx, cmd.UserID, promptTokens+completionTokens); err != nil {
		return nil, err
	}
	return s.Tasks.CreateTask(ctx, &entity.CreateTaskInput{
		ConversationID:        cmd.ConversationID,
		Input:                 input,
		Title:                 title,
		CreatedBy:             cmd.UserID,
		TitlePromptTokens:     promptTokens,
		TitleCompletionTokens: completionTokens,
		AgentID:               agentID,
	})
}

// CreateRun enforces basic run creation rules and delegates to TaskRunStore.
func (s *Service) CreateRun(ctx context.Context, cmd CreateRunCmd) (*entity.TaskRun, error) {
	if s.TaskRuns == nil {
		return nil, ErrTaskRunsNotConfigured
	}
	if cmd.Input == "" {
		return nil, ErrInputRequired
	}
	if err := s.checkQuota(ctx, cmd.UserID, 0); err != nil {
		return nil, err
	}
	return s.TaskRuns.CreateTaskRun(ctx, cmd.TaskID, cmd.Input, cmd.UserID)
}

// StartBackgroundTask creates a task and returns its task/run ids.
func (s *Service) StartBackgroundTask(ctx context.Context, cmd CreateTaskCmd) (*StartBackgroundTaskResult, error) {
	task, err := s.CreateTask(ctx, cmd)
	if err != nil {
		return nil, err
	}
	runID := ""
	if task.LastRunID != nil {
		runID = *task.LastRunID
	}
	return &StartBackgroundTaskResult{
		TaskID: task.TaskID,
		RunID:  runID,
	}, nil
}

func (s *Service) resolveInput(ctx context.Context, userID, input string, agentID *string) (string, *string, error) {
	if agentID == nil || *agentID == "" {
		if input == "" {
			return "", nil, ErrInputRequired
		}
		return input, nil, nil
	}
	if s.Agents == nil {
		return "", nil, ErrAgentsNotConfigured
	}
	agent, err := s.Agents.GetAgent(ctx, *agentID)
	if err != nil {
		return "", nil, err
	}
	if agent == nil || agent.UserID != userID {
		return "", nil, ErrAgentNotFound
	}
	if input != "" {
		return input, agentID, nil
	}
	return buildTaskInputFromAgent(agent, ""), agentID, nil
}

func (s *Service) resolveTitle(ctx context.Context, input string) (string, int, int) {
	title := truncateTaskTitle(input, defaultTitleRunes)
	if s.TitleGenerator == nil {
		return title, 0, 0
	}
	genTitle, promptTokens, completionTokens, err := s.TitleGenerator.GenerateTitle(ctx, input)
	if err != nil || genTitle == "" {
		return title, 0, 0
	}
	return genTitle, promptTokens, completionTokens
}

func (s *Service) checkQuota(ctx context.Context, userID string, tokens int) error {
	if s.QuotaChecker == nil {
		return nil
	}
	allowed, reason := s.QuotaChecker.Check(ctx, userID, 1, tokens)
	if allowed {
		return nil
	}
	return &QuotaExceededError{Reason: reason}
}

func buildTaskInputFromAgent(agent *entity.Agent, userInput string) string {
	out := fmt.Sprintf("Agent: %s\nDescription: %s\nInstructions:\n%s", agent.Name, agent.Description, agent.Instructions)
	if userInput != "" {
		out = out + "\n\n" + userInput
	}
	return out
}

func truncateTaskTitle(input string, maxRunes int) string {
	runes := []rune(input)
	if len(runes) <= maxRunes {
		return input
	}
	return string(runes[:maxRunes]) + "…"
}
