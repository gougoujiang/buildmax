package portal

import (
	"errors"
	"net/http"

	workflowapp "buildmax/internal/app/workflow"
	"buildmax/internal/server/httputil"
)

func (h *Handler) workflowService() *workflowapp.Service {
	return &workflowapp.Service{
		Workflows:     h.cfg.WorkflowStore,
		Agents:        h.cfg.AgentStore,
		Issues:        h.cfg.IssueStore,
		Conversations: h.cfg.ConversationStore,
		Task:          h.taskService(),
	}
}

func (h *Handler) writeWorkflowServiceError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, workflowapp.ErrWorkflowsNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "workflows not configured")
		return true
	case errors.Is(err, workflowapp.ErrConversationsNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "conversations not configured")
		return true
	case errors.Is(err, workflowapp.ErrIssuesNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "issues not configured")
		return true
	case errors.Is(err, workflowapp.ErrTasksNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "tasks not configured")
		return true
	case errors.Is(err, workflowapp.ErrWorkflowNameRequired):
		httputil.WriteJSONError(w, http.StatusBadRequest, "workflow name required")
		return true
	case errors.Is(err, workflowapp.ErrWorkflowDefinitionRequired):
		httputil.WriteJSONError(w, http.StatusBadRequest, "workflow definition required")
		return true
	case errors.Is(err, workflowapp.ErrWorkflowNotFound):
		httputil.WriteJSONError(w, http.StatusNotFound, "workflow not found")
		return true
	case errors.Is(err, workflowapp.ErrWorkflowRunNotFound):
		httputil.WriteJSONError(w, http.StatusNotFound, "workflow run not found")
		return true
	case errors.Is(err, workflowapp.ErrIssueNotFound):
		httputil.WriteJSONError(w, http.StatusNotFound, "issue not found")
		return true
	case errors.Is(err, workflowapp.ErrIssueWorkflowMismatch):
		httputil.WriteJSONError(w, http.StatusBadRequest, "issue not assigned to workflow")
		return true
	case errors.Is(err, workflowapp.ErrInvalidDefinition), errors.Is(err, workflowapp.ErrInvalidStepType), errors.Is(err, workflowapp.ErrInvalidStepID), errors.Is(err, workflowapp.ErrInvalidTargetAgent):
		httputil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return true
	}
	return false
}
