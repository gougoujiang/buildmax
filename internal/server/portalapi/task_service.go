package portalapi

import (
	"context"
	"errors"
	"net/http"

	taskapp "buildmax/internal/core/task"
	"buildmax/internal/server/httputil"
)

type taskTitleGeneratorAdapter struct {
	gen TitleGenerator
}

func (a taskTitleGeneratorAdapter) GenerateTitle(ctx context.Context, input string) (string, int, int, error) {
	if a.gen == nil {
		return "", 0, 0, nil
	}
	title, usage, err := a.gen.GenerateTitle(ctx, input)
	return title, usage.PromptTokens, usage.CompletionTokens, err
}

func (h *Handler) taskService() *taskapp.Service {
	var quotaChecker taskapp.QuotaChecker
	if h.cfg.QuotaChecker != nil {
		quotaChecker = h.cfg.QuotaChecker
	}
	return &taskapp.Service{
		Agents:         h.cfg.AgentStore,
		Tasks:          h.cfg.TaskStore,
		TaskRuns:       h.cfg.TaskRunStore,
		QuotaChecker:   quotaChecker,
		TitleGenerator: taskTitleGeneratorAdapter{gen: h.cfg.TitleGenerator},
	}
}

func (h *Handler) writeTaskServiceError(w http.ResponseWriter, r *http.Request, err error, agentID *string) bool {
	switch {
	case errors.Is(err, taskapp.ErrInputRequired):
		httputil.WriteJSONError(w, http.StatusBadRequest, "input required")
		return true
	case errors.Is(err, taskapp.ErrAgentsNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "agents not configured")
		return true
	case errors.Is(err, taskapp.ErrTasksNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "tasks not configured")
		return true
	case errors.Is(err, taskapp.ErrTaskRunsNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "task runs not configured")
		return true
	case errors.Is(err, taskapp.ErrAgentNotFound):
		if agentID != nil && *agentID != "" {
			httputil.WriteJSONError(w, http.StatusBadRequest, "agent not found")
		} else {
			httputil.WriteJSONError(w, http.StatusNotFound, "task not found")
		}
		return true
	}
	var quotaErr *taskapp.QuotaExceededError
	if errors.As(err, &quotaErr) {
		httputil.WriteQuotaExceeded(w, quotaErr.Reason)
		return true
	}
	return false
}
