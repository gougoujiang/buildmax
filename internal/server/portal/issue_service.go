package portal

import (
	"errors"
	"net/http"

	issueapp "buildmax/internal/app/issue"
	"buildmax/internal/server/httputil"
)

func (h *Handler) issueService() *issueapp.Service {
	return &issueapp.Service{
		Issues: h.cfg.IssueStore,
		Agents: h.cfg.AgentStore,
		Teams:  h.cfg.TeamStore,
	}
}

func (h *Handler) writeIssueServiceError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, issueapp.ErrIssuesNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "issues not configured")
		return true
	case errors.Is(err, issueapp.ErrTitleRequired):
		httputil.WriteJSONError(w, http.StatusBadRequest, "title required")
		return true
	case errors.Is(err, issueapp.ErrTeamsNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "teams not configured")
		return true
	case errors.Is(err, issueapp.ErrInvalidStatus):
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid status")
		return true
	case errors.Is(err, issueapp.ErrInvalidAssigneeKind):
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid assignee_kind")
		return true
	case errors.Is(err, issueapp.ErrInvalidAssigneeID):
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid assignee_id")
		return true
	case errors.Is(err, issueapp.ErrAgentsNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "agents not configured")
		return true
	case errors.Is(err, issueapp.ErrAgentNotFound):
		httputil.WriteJSONError(w, http.StatusBadRequest, "agent not found")
		return true
	case errors.Is(err, issueapp.ErrIssueNotFound):
		httputil.WriteJSONError(w, http.StatusNotFound, "issue not found")
		return true
	}
	return false
}
