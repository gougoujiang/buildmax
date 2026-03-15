package portal

import (
	"context"
	"errors"
	"net/http"

	chatapp "buildmax/internal/app/chat"
	"buildmax/internal/server/httputil"
)

type chatTitleGeneratorAdapter struct {
	gen ChatTitleGenerator
}

func (a chatTitleGeneratorAdapter) GenerateTitle(ctx context.Context, input string) (string, int, int, error) {
	if a.gen == nil {
		return "", 0, 0, nil
	}
	title, usage, err := a.gen.GenerateChatTitle(ctx, input)
	return title, usage.PromptTokens, usage.CompletionTokens, err
}

func (h *Handler) chatService() *chatapp.Service {
	var quotaChecker chatapp.QuotaChecker
	if h.cfg.QuotaChecker != nil {
		quotaChecker = h.cfg.QuotaChecker
	}
	return &chatapp.Service{
		Agents:         h.cfg.AgentStore,
		Chats:          h.cfg.TaskStore,
		TaskRuns:       h.cfg.TaskRunStore,
		QuotaChecker:   quotaChecker,
		TitleGenerator: chatTitleGeneratorAdapter{gen: h.cfg.ChatTitleGenerator},
	}
}

func (h *Handler) writeChatServiceError(w http.ResponseWriter, r *http.Request, err error, agentID *string) bool {
	switch {
	case errors.Is(err, chatapp.ErrInputRequired):
		httputil.WriteJSONError(w, http.StatusBadRequest, "input required")
		return true
	case errors.Is(err, chatapp.ErrAgentsNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "agents not configured")
		return true
	case errors.Is(err, chatapp.ErrChatsNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "tasks not configured")
		return true
	case errors.Is(err, chatapp.ErrTaskRunsNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "task runs not configured")
		return true
	case errors.Is(err, chatapp.ErrAgentNotFound):
		if agentID != nil && *agentID != "" {
			httputil.WriteJSONError(w, http.StatusBadRequest, "agent not found")
		} else {
			httputil.WriteJSONError(w, http.StatusNotFound, "task not found")
		}
		return true
	}
	var quotaErr *chatapp.QuotaExceededError
	if errors.As(err, &quotaErr) {
		httputil.WriteQuotaExceeded(w, quotaErr.Reason)
		return true
	}
	return false
}
