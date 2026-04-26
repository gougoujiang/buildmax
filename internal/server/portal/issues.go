package portal

import (
	"net/http"

	issueapp "buildmax/internal/app/issue"
	"buildmax/internal/server/httputil"
	"buildmax/internal/storage/entity"
)

type IssueResponse struct {
	ID           string  `json:"id"`
	UserID       string  `json:"user_id"`
	TeamID       string  `json:"team_id,omitempty"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	Status       string  `json:"status"`
	AssigneeKind *string `json:"assignee_kind,omitempty"`
	AssigneeID   *string `json:"assignee_id,omitempty"`
	CreatedBy    string  `json:"created_by"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
}

type issueListResponse struct {
	Issues []IssueResponse `json:"issues"`
	Total  int             `json:"total"`
}

type issueFlowRunResponse struct {
	Run   workflowRunResponse       `json:"run"`
	Steps []workflowStepRunResponse `json:"steps"`
}

type issueFlowResponse struct {
	Issue    IssueResponse          `json:"issue"`
	Workflow *workflowResponse      `json:"workflow,omitempty"`
	Runs     []issueFlowRunResponse `json:"runs"`
	Total    int                    `json:"total"`
}

type createIssueRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type patchIssueRequest struct {
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	Status       *string `json:"status"`
	AssigneeKind *string `json:"assignee_kind"`
	AssigneeID   *string `json:"assignee_id"`
}

func issueToResponse(issue entity.Issue) IssueResponse {
	return IssueResponse{
		ID:           issue.IssueID,
		UserID:       issue.UserID,
		TeamID:       issue.TeamID,
		Title:        issue.Title,
		Description:  issue.Description,
		Status:       issue.Status,
		AssigneeKind: issue.AssigneeKind,
		AssigneeID:   issue.AssigneeID,
		CreatedBy:    issue.CreatedBy,
		CreatedAt:    issue.CreatedAt,
		UpdatedAt:    issue.UpdatedAt,
	}
}

func (h *Handler) listIssuesHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.IssueStore, "issues not configured")
	if !ok {
		return
	}
	limit, offset := parseLimitOffset(r.URL.Query(), "limit", "offset", 50, 100)
	list, total, err := h.cfg.IssueStore.ListIssuesByTeam(r.Context(), teamID, limit, offset)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_issues", "user_id", userID, "team_id", teamID)
		return
	}
	out := make([]IssueResponse, len(list))
	for i := range list {
		out[i] = issueToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, issueListResponse{Issues: out, Total: total})
}

func (h *Handler) createIssueHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.IssueStore, "issues not configured")
	if !ok {
		return
	}
	var req createIssueRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	issue, err := h.issueService().CreateIssue(r.Context(), issueapp.CreateIssueCmd{
		UserID:      userID,
		TeamID:      teamID,
		Title:       req.Title,
		Description: req.Description,
	})
	if err != nil {
		if h.writeIssueServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_issue", "user_id", userID, "team_id", teamID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, issueToResponse(*issue))
}

func (h *Handler) getIssueHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.IssueStore, "issues not configured")
	if !ok {
		return
	}
	issueID, ok := pathValueRequired(w, r, "issue_id")
	if !ok {
		return
	}
	issue, err := h.cfg.IssueStore.GetIssue(r.Context(), issueID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_issue", "issue_id", issueID)
		return
	}
	if issue == nil || issue.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "issue not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, issueToResponse(*issue))
}

func (h *Handler) getIssueFlowHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.IssueStore, "issues not configured")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.WorkflowStore, "workflows not configured") {
		return
	}
	issueID, ok := pathValueRequired(w, r, "issue_id")
	if !ok {
		return
	}
	issue, err := h.cfg.IssueStore.GetIssue(r.Context(), issueID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_issue_flow_issue", "issue_id", issueID)
		return
	}
	if issue == nil || issue.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "issue not found")
		return
	}

	var workflowOut *workflowResponse
	if issue.AssigneeKind != nil && issue.AssigneeID != nil && *issue.AssigneeKind == entity.IssueAssigneeWorkflow {
		workflow, err := h.cfg.WorkflowStore.GetWorkflow(r.Context(), *issue.AssigneeID)
		if err != nil {
			httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_issue_flow_workflow", "issue_id", issueID)
			return
		}
		if workflow != nil && workflow.TeamID == teamID {
			out := workflowToResponse(*workflow)
			workflowOut = &out
		}
	}

	limit, offset := parseLimitOffset(r.URL.Query(), "limit", "offset", 20, 100)
	runs, total, err := h.cfg.WorkflowStore.ListWorkflowRunsByIssue(r.Context(), issueID, limit, offset)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_issue_flow_runs", "issue_id", issueID)
		return
	}
	runOut := make([]issueFlowRunResponse, len(runs))
	for i := range runs {
		steps, err := h.cfg.WorkflowStore.ListWorkflowStepRuns(r.Context(), runs[i].WorkflowRunID)
		if err != nil {
			httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_issue_flow_steps", "workflow_run_id", runs[i].WorkflowRunID)
			return
		}
		stepOut := make([]workflowStepRunResponse, len(steps))
		for j := range steps {
			stepOut[j] = workflowStepRunToResponse(steps[j])
		}
		runOut[i] = issueFlowRunResponse{
			Run:   workflowRunToResponse(runs[i]),
			Steps: stepOut,
		}
	}
	httputil.WriteJSON(w, http.StatusOK, issueFlowResponse{
		Issue:    issueToResponse(*issue),
		Workflow: workflowOut,
		Runs:     runOut,
		Total:    total,
	})
}

func (h *Handler) patchIssueHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.IssueStore, "issues not configured")
	if !ok {
		return
	}
	issueID, ok := pathValueRequired(w, r, "issue_id")
	if !ok {
		return
	}
	var req patchIssueRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	issue, err := h.issueService().UpdateIssue(r.Context(), issueapp.UpdateIssueCmd{
		UserID:       userID,
		TeamID:       teamID,
		IssueID:      issueID,
		Title:        req.Title,
		Description:  req.Description,
		Status:       req.Status,
		AssigneeKind: req.AssigneeKind,
		AssigneeID:   req.AssigneeID,
	})
	if err != nil {
		if h.writeIssueServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "patch_issue", "issue_id", issueID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, issueToResponse(*issue))
}
