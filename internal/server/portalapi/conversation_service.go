package portalapi

import (
	"errors"
	"net/http"

	convapp "buildmax/internal/core/conversation"
	taskapp "buildmax/internal/core/task"
	"buildmax/internal/server/httputil"
)

func (h *Handler) conversationService() *convapp.Service {
	return &convapp.Service{
		TaskService:       h.taskService(),
		ConversationStore: h.cfg.ConversationStore,
		MessageStore:      h.cfg.ConversationMessageStore,
		LLMClient:         h.cfg.ConversationLLMClient,
		TitleGenerator:    h.cfg.TitleGenerator,
		AgentStore:        h.cfg.AgentStore,
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
