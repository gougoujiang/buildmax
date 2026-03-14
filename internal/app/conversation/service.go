package conversation

import (
	"context"
	"errors"
	"fmt"

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
	WorkspaceID    string
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
		return s.handleChatRunTurn(ctx, cmd)
	case cmd.ConversationID != "":
		return s.handleConversationTurn(ctx, cmd)
	default:
		return HandleTurnResult{}, ErrInvalidTarget
	}
}

func (s *Service) handleChatRunTurn(ctx context.Context, cmd HandleTurnCmd) (HandleTurnResult, error) {
	if s.ChatService == nil {
		return HandleTurnResult{}, chatapp.ErrChatRunsNotConfigured
	}
	run, err := s.ChatService.CreateRun(ctx, chatapp.CreateRunCmd{
		UserID: cmd.UserID,
		ChatID: cmd.ChatID,
		Input:  cmd.Message,
	})
	if err != nil {
		return HandleTurnResult{}, err
	}
	return HandleTurnResult{TaskIDs: []string{run.ChatRunID}}, nil
}

func (s *Service) handleConversationTurn(ctx context.Context, cmd HandleTurnCmd) (HandleTurnResult, error) {
	if s.ConversationStore == nil || s.MessageStore == nil {
		return HandleTurnResult{}, fmt.Errorf("conversation stores not configured")
	}
	if s.LLMCaller == nil {
		return HandleTurnResult{}, ErrLLMRequired
	}

	runner := s.startChatRunner(cmd.WorkspaceID, cmd.UserID, cmd.ConversationID)
	if cmd.StreamSink != nil {
		reply, err := coreconv.RunLoopStream(
			ctx,
			s.ConversationStore,
			s.MessageStore,
			s.LLMCaller,
			cmd.ConversationID,
			cmd.Message,
			cmd.Channel,
			nil,
			cmd.WorkspaceID,
			cmd.UserID,
			runner,
			titleGeneratorAdapter{s.TitleGenerator},
			cmd.StreamSink,
		)
		return HandleTurnResult{Reply: reply}, err
	}

	reply, err := coreconv.RunLoop(
		ctx,
		s.ConversationStore,
		s.MessageStore,
		s.LLMCaller,
		cmd.ConversationID,
		cmd.Message,
		cmd.Channel,
		nil,
		cmd.WorkspaceID,
		cmd.UserID,
		runner,
		titleGeneratorAdapter{s.TitleGenerator},
	)
	return HandleTurnResult{Reply: reply}, err
}

func (s *Service) startChatRunner(workspaceID, userID, conversationID string) tools.StartChatRunner {
	if s.ChatService == nil {
		return nil
	}
	return &startChatRunner{
		chatService:    s.ChatService,
		workspaceID:    workspaceID,
		userID:         userID,
		conversationID: conversationID,
	}
}

type startChatRunner struct {
	chatService    *chatapp.Service
	workspaceID    string
	userID         string
	conversationID string
}

func (r *startChatRunner) StartChat(ctx context.Context, _, _, input string, agentID *string) (chatID, runID string, err error) {
	var conversationID *string
	if r.conversationID != "" {
		conversationID = &r.conversationID
	}
	result, err := r.chatService.StartBackgroundChat(ctx, chatapp.CreateChatCmd{
		WorkspaceID:    r.workspaceID,
		UserID:         r.userID,
		Input:          input,
		AgentID:        agentID,
		ConversationID: conversationID,
	})
	if err != nil {
		return "", "", err
	}
	return result.ChatID, result.RunID, nil
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
