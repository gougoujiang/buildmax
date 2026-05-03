package task

import (
	"context"
	"errors"
	"fmt"

	"buildmax/internal/core/model"
)

const defaultTitleRunes = 50

var (
	ErrInputRequired         = errors.New("input required")
	ErrAgentsNotConfigured   = errors.New("agents not configured")
	ErrTasksNotConfigured    = errors.New("tasks not configured")
	ErrTaskRunsNotConfigured = errors.New("task runs not configured")
	ErrAgentNotFound         = errors.New("agent not found or not owned by team")
)

// QuotaChecker is the narrow quota surface needed by task workflows.
type QuotaChecker interface {
	Check(ctx context.Context, teamID string, runsToAdd, tokensToAdd int) (allowed bool, reason string)
}

// QuotaExceededError is returned when quota blocks a task operation.
type QuotaExceededError struct {
	Reason string
}

func (e *QuotaExceededError) Error() string {
	return "quota exceeded: " + e.Reason
}

// TaskService owns task-related application workflows.
type TaskService struct {
	Agents         model.AgentStore
	Tasks          model.TaskStore
	TaskRuns       model.TaskRunStore
	QuotaChecker   QuotaChecker
	TitleGenerator model.TitleGenerator
}

// CreateTaskCmd creates a new task and its first run.
type CreateTaskCmd struct {
	ConversationID string
	UserID         string
	TeamID         string
	Input          string
	AgentID        *string
	IssueID        *string
	CreatedByType  string
	TriggerSource  string
}

// CreateRunCmd creates a new run on an existing task.
type CreateRunCmd struct {
	UserID        string
	TaskID        string
	Input         string
	CreatedByType string
	TriggerSource string
}

// StartBackgroundTaskResult is returned when a background task is created.
type StartBackgroundTaskResult struct {
	TaskID string
	RunID  string
}

// CreateTask resolves input, applies title/quota rules, and persists a new task.
func (s *TaskService) CreateTask(ctx context.Context, cmd CreateTaskCmd) (*model.Task, error) {
	if s.Tasks == nil {
		return nil, ErrTasksNotConfigured
	}
	input, agentID, err := s.resolveInput(ctx, cmd.TeamID, cmd.UserID, cmd.Input, cmd.AgentID)
	if err != nil {
		return nil, err
	}
	createdByType, triggerSource := normalizeCreateTaskProvenance(cmd.CreatedByType, cmd.TriggerSource)
	title, promptTokens, completionTokens := s.resolveTitle(ctx, input)
	if err := s.checkQuota(ctx, cmd.TeamID, promptTokens+completionTokens); err != nil {
		return nil, err
	}
	return s.Tasks.CreateTask(ctx, &model.CreateTaskInput{
		ConversationID:          cmd.ConversationID,
		TeamID:                  cmd.TeamID,
		Input:                   input,
		Title:                   title,
		CreatedBy:               cmd.UserID,
		InitialRunCreatedBy:     cmd.UserID,
		InitialRunCreatedByType: createdByType,
		InitialRunTriggerSource: triggerSource,
		TitlePromptTokens:       promptTokens,
		TitleCompletionTokens:   completionTokens,
		AgentID:                 agentID,
		IssueID:                 cmd.IssueID,
	})
}

// CreateRun enforces basic run creation rules and delegates to TaskRunStore.
func (s *TaskService) CreateRun(ctx context.Context, cmd CreateRunCmd) (*model.TaskRun, error) {
	if s.TaskRuns == nil {
		return nil, ErrTaskRunsNotConfigured
	}
	if cmd.Input == "" {
		return nil, ErrInputRequired
	}
	if s.QuotaChecker != nil && s.Tasks != nil {
		task, err := s.Tasks.GetTask(ctx, cmd.TaskID)
		if err != nil {
			return nil, err
		}
		if task != nil {
			if err := s.checkQuota(ctx, task.TeamID, 0); err != nil {
				return nil, err
			}
		}
	}
	createdByType, triggerSource := normalizeCreateRunProvenance(cmd.CreatedByType, cmd.TriggerSource)
	return s.TaskRuns.CreateTaskRun(ctx, cmd.TaskID, cmd.Input, cmd.UserID, createdByType, triggerSource)
}

// StartBackgroundTask creates a task and returns its task/run ids.
func (s *TaskService) StartBackgroundTask(ctx context.Context, cmd CreateTaskCmd) (*StartBackgroundTaskResult, error) {
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

func (s *TaskService) resolveInput(ctx context.Context, teamID, userID, input string, agentID *string) (string, *string, error) {
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
	if agent == nil || agent.TeamID != teamID {
		return "", nil, ErrAgentNotFound
	}
	if input != "" {
		return input, agentID, nil
	}
	return buildTaskInputFromAgent(agent, ""), agentID, nil
}

func (s *TaskService) resolveTitle(ctx context.Context, input string) (string, int, int) {
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

func (s *TaskService) checkQuota(ctx context.Context, teamID string, tokens int) error {
	if s.QuotaChecker == nil {
		return nil
	}
	if teamID == "" {
		return nil
	}
	allowed, reason := s.QuotaChecker.Check(ctx, teamID, 1, tokens)
	if allowed {
		return nil
	}
	return &QuotaExceededError{Reason: reason}
}

func buildTaskInputFromAgent(agent *model.AgentDefinition, userInput string) string {
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

func normalizeCreateTaskProvenance(createdByType, triggerSource string) (string, string) {
	if createdByType == "" {
		createdByType = model.RunCreatedByTypeUser
	}
	if triggerSource == "" {
		triggerSource = model.RunTriggerSourceTaskCreate
	}
	return createdByType, triggerSource
}

func normalizeCreateRunProvenance(createdByType, triggerSource string) (string, string) {
	if createdByType == "" {
		createdByType = model.RunCreatedByTypeUser
	}
	if triggerSource == "" {
		triggerSource = model.RunTriggerSourceTaskRerun
	}
	return createdByType, triggerSource
}
