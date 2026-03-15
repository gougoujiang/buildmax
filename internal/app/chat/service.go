package chat

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
	ErrChatsNotConfigured    = errors.New("chats not configured")
	ErrTaskRunsNotConfigured = errors.New("task runs not configured")
	ErrAgentNotFound         = errors.New("agent not found or not in workspace")
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
	Chats          entity.TaskStore
	TaskRuns       entity.TaskRunStore
	QuotaChecker   QuotaChecker
	TitleGenerator TitleGenerator
}

// CreateChatCmd creates a new task and its first run.
type CreateChatCmd struct {
	WorkspaceID    string
	UserID         string
	Input          string
	AgentID        *string
	ConversationID *string
}

// CreateRunCmd creates a new run on an existing task.
type CreateRunCmd struct {
	UserID string
	ChatID string
	Input  string
}

// StartBackgroundChatResult is returned when a background task is created.
type StartBackgroundChatResult struct {
	ChatID string
	RunID  string
}

// CreateChat resolves input, applies title/quota rules, and persists a new task.
func (s *Service) CreateChat(ctx context.Context, cmd CreateChatCmd) (*entity.Chat, error) {
	if s.Chats == nil {
		return nil, ErrChatsNotConfigured
	}
	input, agentID, err := s.resolveInput(ctx, cmd.WorkspaceID, cmd.Input, cmd.AgentID)
	if err != nil {
		return nil, err
	}
	title, promptTokens, completionTokens := s.resolveTitle(ctx, input)
	if err := s.checkQuota(ctx, cmd.UserID, promptTokens+completionTokens); err != nil {
		return nil, err
	}
	return s.Chats.CreateChat(ctx, &entity.CreateChatInput{
		WorkspaceID:           cmd.WorkspaceID,
		Input:                 input,
		Title:                 title,
		CreatedBy:             cmd.UserID,
		TitlePromptTokens:     promptTokens,
		TitleCompletionTokens: completionTokens,
		AgentID:               agentID,
		ConversationID:        cmd.ConversationID,
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
	return s.TaskRuns.CreateTaskRun(ctx, cmd.ChatID, cmd.Input, cmd.UserID)
}

// StartBackgroundChat creates a task and returns its task/run ids.
func (s *Service) StartBackgroundChat(ctx context.Context, cmd CreateChatCmd) (*StartBackgroundChatResult, error) {
	chat, err := s.CreateChat(ctx, cmd)
	if err != nil {
		return nil, err
	}
	runID := ""
	if chat.LastRunID != nil {
		runID = *chat.LastRunID
	}
	return &StartBackgroundChatResult{
		ChatID: chat.ChatID,
		RunID:  runID,
	}, nil
}

func (s *Service) resolveInput(ctx context.Context, workspaceID, input string, agentID *string) (string, *string, error) {
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
	if agent == nil || agent.WorkspaceID != workspaceID {
		return "", nil, ErrAgentNotFound
	}
	if input != "" {
		return input, agentID, nil
	}
	return buildChatInputFromAgent(agent, ""), agentID, nil
}

func (s *Service) resolveTitle(ctx context.Context, input string) (string, int, int) {
	title := truncateChatTitle(input, defaultTitleRunes)
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

func buildChatInputFromAgent(agent *entity.Agent, userInput string) string {
	out := fmt.Sprintf("Agent: %s\nDescription: %s\nInstructions:\n%s", agent.Name, agent.Description, agent.Instructions)
	if userInput != "" {
		out = out + "\n\n" + userInput
	}
	return out
}

func truncateChatTitle(input string, maxRunes int) string {
	runes := []rune(input)
	if len(runes) <= maxRunes {
		return input
	}
	return string(runes[:maxRunes]) + "…"
}
