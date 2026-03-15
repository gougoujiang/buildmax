package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	chatapp "buildmax/internal/app/chat"
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
	ChatService       *chatapp.Service
	ConversationStore entity.ConversationStore
	MessageStore      entity.ConversationMessageStore
	LLMCaller         llm.LLMCaller
	TitleGenerator    TitleGenerator
}

// HandleTurnCmd describes one normalized portal turn.
type HandleTurnCmd struct {
	UserID         string
	Channel        string
	Message        string
	ChatID         string
	ConversationID string
	StreamSink     llm.StreamSink
}

// HandleTurnResult returns either a direct reply and/or spawned task ids.
type HandleTurnResult struct {
	Reply   string
	TaskIDs []string
}

// HandleTurn routes the turn to either a chat-run creation flow or a conversation LLM flow.
func (s *Service) HandleTurn(ctx context.Context, cmd HandleTurnCmd) (HandleTurnResult, error) {
	switch {
	case cmd.ChatID != "":
		return s.handleTaskRunTurn(ctx, cmd)
	case cmd.ConversationID != "":
		return s.handleConversationTurn(ctx, cmd)
	default:
		return HandleTurnResult{}, ErrInvalidTarget
	}
}

func (s *Service) handleTaskRunTurn(ctx context.Context, cmd HandleTurnCmd) (HandleTurnResult, error) {
	if s.ChatService == nil {
		return HandleTurnResult{}, chatapp.ErrTaskRunsNotConfigured
	}
	run, err := s.ChatService.CreateRun(ctx, chatapp.CreateRunCmd{
		UserID: cmd.UserID,
		ChatID: cmd.ChatID,
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

	runInput := coreconv.RunInput{
		ConversationID:     cmd.ConversationID,
		UserContent:        cmd.Message,
		Channel:            cmd.Channel,
		ScopeID:            cmd.ConversationID,
		UserID:             cmd.UserID,
		Runners:            s.conversationToolRunners(cmd.ConversationID, cmd.UserID),
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
	if s.ChatService == nil {
		return nil
	}
	return &startTaskRunner{
		chatService:    s.ChatService,
		userID:         userID,
		conversationID: conversationID,
	}
}

type startTaskRunner struct {
	chatService    *chatapp.Service
	userID         string
	conversationID string
}

func (r *startTaskRunner) StartTask(ctx context.Context, _, _, input string, agentID *string) (taskID, runID string, err error) {
	result, err := r.chatService.StartBackgroundChat(ctx, chatapp.CreateChatCmd{
		ConversationID: r.conversationID,
		UserID:         r.userID,
		Input:          input,
		AgentID:        agentID,
	})
	if err != nil {
		return "", "", err
	}
	return result.ChatID, result.RunID, nil
}

func (s *Service) listTasksRunner() tools.ListTasksRunner {
	if s.ChatService == nil || s.ChatService.Chats == nil {
		return nil
	}
	return &listChatsRunner{chats: s.ChatService.Chats}
}

type listChatsRunner struct {
	chats entity.TaskStore
}

func (r *listChatsRunner) ListTasks(ctx context.Context, conversationID string) (string, error) {
	list, _, err := r.chats.ListChatsByConversationPaginated(ctx, conversationID, false, 10, 0)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "No recent tasks in this conversation.", nil
	}
	var lines []string
	for i, c := range list {
		snippet := chatTitleOrSnippet(&c, 60)
		ts := formatCreatedAt(c.CreatedAt)
		lines = append(lines, fmt.Sprintf("%d. %s | %s | %s | %s", i+1, c.ChatID, snippet, c.Status, ts))
	}
	return strings.Join(lines, "\n"), nil
}

func (s *Service) getTaskRunner() tools.GetTaskRunner {
	if s.ChatService == nil || s.ChatService.Chats == nil {
		return nil
	}
	return &getTaskRunner{chats: s.ChatService.Chats}
}

type getTaskRunner struct {
	chats entity.TaskStore
}

func (r *getTaskRunner) GetTask(ctx context.Context, conversationID, chatID string) (string, error) {
	chat, err := r.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", err
	}
	if chat == nil {
		return "", fmt.Errorf("task not found or not in this conversation")
	}
	if chat.ConversationID != conversationID {
		return "", fmt.Errorf("task not found or not in this conversation")
	}
	inputTrunc := truncateRunes(chat.Input, 500)
	outputLine := ""
	if chat.Output != nil && *chat.Output != "" {
		outputLine = "output_snippet: " + truncateRunes(*chat.Output, 200) + "\n"
	}
	lastRun := ""
	if chat.LastRunID != nil {
		lastRun = *chat.LastRunID
	}
	return fmt.Sprintf("task_id: %s\ntitle: %s\ninput: %s\nstatus: %s\ncreated_at: %s\nlast_run_id: %s\n%s",
		chat.ChatID, chat.Title, inputTrunc, chat.Status, formatCreatedAt(chat.CreatedAt), lastRun, outputLine), nil
}

func (s *Service) continueTaskRunner() tools.ContinueTaskRunner {
	if s.ChatService == nil {
		return nil
	}
	return &continueTaskRunner{chatService: s.ChatService}
}

type continueTaskRunner struct {
	chatService *chatapp.Service
}

func (r *continueTaskRunner) ContinueTask(ctx context.Context, conversationID, userID, chatID, input string) (runID string, err error) {
	if r.chatService.Chats == nil {
		return "", fmt.Errorf("tasks not configured")
	}
	chat, err := r.chatService.Chats.GetChat(ctx, chatID)
	if err != nil {
		return "", err
	}
	if chat == nil || chat.ConversationID != conversationID {
		return "", fmt.Errorf("task not found or not in this conversation")
	}
	run, err := r.chatService.CreateRun(ctx, chatapp.CreateRunCmd{
		UserID: userID,
		ChatID: chatID,
		Input:  input,
	})
	if err != nil {
		return "", err
	}
	return run.TaskRunID, nil
}

func (s *Service) recentTasksSnippet(ctx context.Context, conversationID string) string {
	if s.ChatService == nil || s.ChatService.Chats == nil {
		return ""
	}
	list, _, err := s.ChatService.Chats.ListChatsByConversationPaginated(ctx, conversationID, false, 5, 0)
	if err != nil || len(list) == 0 {
		return "Recent tasks in this conversation (latest 5): No recent tasks."
	}
	var lines []string
	for _, c := range list {
		snippet := chatTitleOrSnippet(&c, 60)
		lines = append(lines, fmt.Sprintf("%s | %s | %s | %s", c.ChatID, snippet, c.Status, formatCreatedAt(c.CreatedAt)))
	}
	return "Recent tasks in this conversation (latest 5):\n" + strings.Join(lines, "\n")
}

func chatTitleOrSnippet(c *entity.Chat, maxRunes int) string {
	if c.Title != "" {
		return truncateRunes(c.Title, maxRunes)
	}
	return truncateRunes(c.Input, maxRunes)
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
