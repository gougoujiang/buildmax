package portal

import (
	"context"
	"errors"
	"net/http"

	chatapp "buildmax/internal/app/chat"
	convapp "buildmax/internal/app/conversation"
)

type conversationTitleGeneratorAdapter struct {
	gen ChatTitleGenerator
}

func (a conversationTitleGeneratorAdapter) GenerateTitle(ctx context.Context, input string) (string, error) {
	if a.gen == nil {
		return "", nil
	}
	title, _, err := a.gen.GenerateChatTitle(ctx, input)
	return title, err
}

func (h *Handler) conversationService() *convapp.Service {
	return &convapp.Service{
		ChatService:       h.chatService(),
		ConversationStore: h.cfg.ConversationStore,
		MessageStore:      h.cfg.ConversationMessageStore,
		LLMCaller:         h.cfg.ConversationLLMCaller,
		TitleGenerator:    conversationTitleGeneratorAdapter{gen: h.cfg.ChatTitleGenerator},
	}
}

func (h *Handler) writeConversationServiceError(w http.ResponseWriter, r *http.Request, err error, agentID *string) bool {
	if h.writeChatServiceError(w, r, err, agentID) {
		return true
	}
	switch {
	case errors.Is(err, convapp.ErrInvalidTarget):
		writeJSONError(w, http.StatusBadRequest, "invalid conversation target")
		return true
	case errors.Is(err, convapp.ErrLLMRequired):
		writeJSONError(w, http.StatusServiceUnavailable, "conversation LLM not configured")
		return true
	}
	var quotaErr *chatapp.QuotaExceededError
	if errors.As(err, &quotaErr) {
		writeQuotaExceeded(w, quotaErr.Reason)
		return true
	}
	return false
}
