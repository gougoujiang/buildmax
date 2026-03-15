package portal

import (
	"context"
	"errors"
	"net/http"

	taskapp "buildmax/internal/app/task"
	convapp "buildmax/internal/app/conversation"
	"buildmax/internal/server/httputil"
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
		TaskService:       h.taskService(),
		ConversationStore: h.cfg.ConversationStore,
		MessageStore:      h.cfg.ConversationMessageStore,
		LLMCaller:         h.cfg.ConversationLLMCaller,
		TitleGenerator:    conversationTitleGeneratorAdapter{gen: h.cfg.ChatTitleGenerator},
	}
}

func (h *Handler) writeConversationServiceError(w http.ResponseWriter, r *http.Request, err error, agentID *string) bool {
	if h.writeTaskServiceError(w, r, err, agentID) {
		return true
	}
	switch {
	case errors.Is(err, convapp.ErrInvalidTarget):
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid conversation target")
		return true
	case errors.Is(err, convapp.ErrLLMRequired):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "conversation LLM not configured")
		return true
	}
	var quotaErr *taskapp.QuotaExceededError
	if errors.As(err, &quotaErr) {
		httputil.WriteQuotaExceeded(w, quotaErr.Reason)
		return true
	}
	return false
}
