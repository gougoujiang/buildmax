package handlers

import (
	"errors"
	"net/http"
	"strings"

	"buildmax/internal/core/issue"
	"buildmax/internal/core/model"
	"buildmax/internal/core/task"
	"buildmax/internal/server/httputil"
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
	Issue      IssueResponse          `json:"issue"`
	Workflow   *workflowResponse      `json:"workflow,omitempty"`
	Runs       []issueFlowRunResponse `json:"runs"`
	AgentTasks []TaskResponse         `json:"agent_tasks"`
	Total      int                    `json:"total"`
}

type createIssueAgentRunRequest struct {
	Input string `json:"input"`
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

func issueToResponse(issue model.Issue) IssueResponse {
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

func buildIssueAgentRunInput(issue model.Issue) string {
	var b strings.Builder
	b.WriteString("Work on this issue.\n\n")
	b.WriteString("Title: ")
	b.WriteString(issue.Title)
	if strings.TrimSpace(issue.Description) != "" {
		b.WriteString("\n\nDescription:\n")
		b.WriteString(issue.Description)
	}
	return b.String()
}

func (h *Handler) issueService() *issue.IssueService {
	return &issue.IssueService{
		Issues:    h.cfg.IssueStore,
		Agents:    h.cfg.AgentStore,
		Teams:     h.cfg.TeamStore,
		Workflows: h.cfg.WorkflowStore,
	}
}

func (h *Handler) writeIssueServiceError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, issue.ErrIssuesNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "issues not configured")
	case errors.Is(err, issue.ErrTitleRequired):
		httputil.WriteJSONError(w, http.StatusBadRequest, "title required")
	case errors.Is(err, issue.ErrTeamsNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "teams not configured")
	case errors.Is(err, issue.ErrInvalidStatus):
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid status")
	case errors.Is(err, issue.ErrInvalidAssigneeKind):
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid assignee_kind")
	case errors.Is(err, issue.ErrInvalidAssigneeID):
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid assignee_id")
	case errors.Is(err, issue.ErrAgentsNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "agents not configured")
	case errors.Is(err, issue.ErrAgentNotFound):
		httputil.WriteJSONError(w, http.StatusBadRequest, "agent not found")
	case errors.Is(err, issue.ErrWorkflowsNotConfigured):
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "workflows not configured")
	case errors.Is(err, issue.ErrWorkflowNotFound):
		httputil.WriteJSONError(w, http.StatusBadRequest, "workflow not found")
	case errors.Is(err, issue.ErrWorkflowNotPublished):
		httputil.WriteJSONError(w, http.StatusBadRequest, "workflow not published")
	case errors.Is(err, issue.ErrIssueNotFound):
		httputil.WriteJSONError(w, http.StatusNotFound, "issue not found")
	default:
		return false
	}
	return true
}

func (h *Handler) listIssuesHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.IssueStore, "issues not configured")
	if !ok {
		return
	}
	limit, offset := parseLimitOffset(r.URL.Query(), "limit", "offset", 50, 100)
	list, total, err := h.cfg.IssueStore.ListIssuesByTeam(r.Context(), teamID, limit, offset)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_issues", "user_id", userID, "team_id", teamID)
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
	createdIssue, err := h.issueService().CreateIssue(r.Context(), issue.CreateIssueCmd{
		UserID:      userID,
		TeamID:      teamID,
		Title:       req.Title,
		Description: req.Description,
	})
	if err != nil {
		if h.writeIssueServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "create_issue", "user_id", userID, "team_id", teamID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, issueToResponse(*createdIssue))
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
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_issue", "issue_id", issueID)
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
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_issue_flow_issue", "issue_id", issueID)
		return
	}
	if issue == nil || issue.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "issue not found")
		return
	}
	var workflowOut *workflowResponse
	if issue.AssigneeKind != nil && issue.AssigneeID != nil && *issue.AssigneeKind == model.IssueAssigneeWorkflow {
		workflow, err := h.cfg.WorkflowStore.GetWorkflow(r.Context(), *issue.AssigneeID)
		if err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "get_issue_flow_workflow", "issue_id", issueID)
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
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_issue_flow_runs", "issue_id", issueID)
		return
	}
	agentTasks := []TaskResponse{}
	if h.cfg.TaskStore != nil {
		tasks, _, err := h.cfg.TaskStore.ListTasksByIssue(r.Context(), issueID, limit, offset)
		if err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "get_issue_flow_agent_tasks", "issue_id", issueID)
			return
		}
		agentTasks = make([]TaskResponse, len(tasks))
		for i := range tasks {
			agentTasks[i] = taskToResponse(tasks[i])
		}
	}
	runOut := make([]issueFlowRunResponse, len(runs))
	for i := range runs {
		steps, err := h.cfg.WorkflowStore.ListWorkflowStepRuns(r.Context(), runs[i].WorkflowRunID)
		if err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "get_issue_flow_steps", "workflow_run_id", runs[i].WorkflowRunID)
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
		Issue:      issueToResponse(*issue),
		Workflow:   workflowOut,
		Runs:       runOut,
		AgentTasks: agentTasks,
		Total:      total,
	})
}

func (h *Handler) createIssueAgentRunHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.IssueStore, "issues not configured")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.AgentStore, "agents not configured") {
		return
	}
	if !h.requireStore(w, h.cfg.TaskStore, "tasks not configured") {
		return
	}
	if !h.requireStore(w, h.cfg.ConversationStore, "conversations not configured") {
		return
	}
	issueID, ok := pathValueRequired(w, r, "issue_id")
	if !ok {
		return
	}
	var req createIssueAgentRunRequest
	if r.ContentLength != 0 {
		if !decodeJSONBody(w, r, &req) {
			return
		}
	}
	issue, err := h.cfg.IssueStore.GetIssue(r.Context(), issueID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_issue_for_agent_run", "issue_id", issueID)
		return
	}
	if issue == nil || issue.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "issue not found")
		return
	}
	if issue.AssigneeKind == nil || issue.AssigneeID == nil || *issue.AssigneeKind != model.IssueAssigneeAgent {
		httputil.WriteJSONError(w, http.StatusBadRequest, "issue not assigned to agent")
		return
	}
	agent, err := h.cfg.AgentStore.GetAgent(r.Context(), *issue.AssigneeID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_agent_for_issue_run", "issue_id", issueID, "agent_id", *issue.AssigneeID)
		return
	}
	if agent == nil || agent.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusBadRequest, "agent not found")
		return
	}
	conv, err := h.cfg.ConversationStore.CreateConversationInTeam(r.Context(), teamID, userID, "issue_agent", userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "create_issue_agent_conversation", "issue_id", issueID)
		return
	}
	input := req.Input
	if input == "" {
		input = buildIssueAgentRunInput(*issue)
	}
	createdTask, err := h.taskService().CreateTask(r.Context(), task.CreateTaskCmd{
		ConversationID: conv.ConversationID,
		UserID:         userID,
		TeamID:         teamID,
		Input:          input,
		AgentID:        issue.AssigneeID,
		IssueID:        &issueID,
		CreatedByType:  model.RunCreatedByTypeUser,
		TriggerSource:  model.RunTriggerSourceIssueAgentRun,
	})
	if err != nil {
		if h.writeTaskServiceError(w, r, err, issue.AssigneeID) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "create_issue_agent_task", "issue_id", issueID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, taskToResponse(*createdTask))
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
	if req.AssigneeKind != nil && *req.AssigneeKind == model.IssueAssigneeWorkflow {
		if _, ok := h.authorizeTeamAction(w, r, userID, teamID, actionAssignIssueWorkflow); !ok {
			return
		}
	}
	updatedIssue, err := h.issueService().UpdateIssue(r.Context(), issue.UpdateIssueCmd{
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
		httputil.WriteInternalError(w, err, "handler error", "handler", "patch_issue", "issue_id", issueID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, issueToResponse(*updatedIssue))
}
