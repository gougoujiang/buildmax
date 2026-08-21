package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/service/issue"
	"github.com/gougoujiang/buildmax/internal/service/task"
)

type IssueResponse struct {
	ID            string  `json:"id"`
	UserID        string  `json:"user_id"`
	TeamID        string  `json:"team_id,omitempty"`
	ParentIssueID *string `json:"parent_issue_id,omitempty"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	Status        string  `json:"status"`
	AssigneeKind  *string `json:"assignee_kind,omitempty"`
	AssigneeID    *string `json:"assignee_id,omitempty"`
	CreatedBy     string  `json:"created_by"`
	CreatedAt     int64   `json:"created_at"`
	UpdatedAt     int64   `json:"updated_at"`
	// ChildCount, DoneChildCount, and CommentCount are derived per response,
	// never stored. They are zero on responses that do not compute them.
	ChildCount     int `json:"child_count"`
	DoneChildCount int `json:"done_child_count"`
	CommentCount   int `json:"comment_count"`
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
	Issue IssueResponse `json:"issue"`
	// Parent is set on a sub-issue; Children on a parent. Runs, agent tasks,
	// and outputs stay scoped to Issue — a parent's Results panel must keep
	// meaning "what this issue produced".
	Parent       *IssueResponse         `json:"parent,omitempty"`
	Children     []IssueResponse        `json:"children"`
	Workflow     *workflowResponse      `json:"workflow,omitempty"`
	Runs         []issueFlowRunResponse `json:"runs"`
	AgentTasks   []TaskResponse         `json:"agent_tasks"`
	LatestResult *issueOutputResponse   `json:"latest_result,omitempty"`
	Outputs      []issueOutputResponse  `json:"outputs"`
	Total        int                    `json:"total"`
}

type createIssueAgentRunRequest struct {
	Input string `json:"input"`
}

type createIssueRequest struct {
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	ParentIssueID *string `json:"parent_issue_id"`
}

type patchIssueRequest struct {
	Title         *string `json:"title"`
	Description   *string `json:"description"`
	Status        *string `json:"status"`
	AssigneeKind  *string `json:"assignee_kind"`
	AssigneeID    *string `json:"assignee_id"`
	ParentIssueID *string `json:"parent_issue_id"`
}

func issueToResponse(issue model.Issue) IssueResponse {
	return IssueResponse{
		ID:            issue.IssueID,
		UserID:        issue.UserID,
		TeamID:        issue.TeamID,
		ParentIssueID: issue.ParentIssueID,
		Title:         issue.Title,
		Description:   issue.Description,
		Status:        issue.Status,
		AssigneeKind:  issue.AssigneeKind,
		AssigneeID:    issue.AssigneeID,
		CreatedBy:     issue.CreatedBy,
		CreatedAt:     issue.CreatedAt,
		UpdatedAt:     issue.UpdatedAt,
	}
}

// decorateIssueResponses fills the derived counts for a page of issues with one
// grouped query each, rather than a count per row.
//
// Failures degrade to zero counts instead of failing the listing: a missing
// progress badge is a worse-looking page, an error is no page at all.
func (h *Handler) decorateIssueResponses(ctx context.Context, out []IssueResponse) {
	if len(out) == 0 {
		return
	}
	ids := make([]string, len(out))
	for i := range out {
		ids[i] = out[i].ID
	}
	if h.cfg.IssueStore != nil {
		if stats, err := h.cfg.IssueStore.ChildStatsForIssues(ctx, ids); err == nil {
			for i := range out {
				if s, ok := stats[out[i].ID]; ok {
					out[i].ChildCount = s.Total
					out[i].DoneChildCount = s.Done
				}
			}
		} else {
			slog.Warn("issue child stats not loaded", "err", err)
		}
	}
	if h.cfg.IssueCommentStore != nil {
		if counts, err := h.cfg.IssueCommentStore.CountIssueComments(ctx, ids); err == nil {
			for i := range out {
				out[i].CommentCount = counts[out[i].ID]
			}
		} else {
			slog.Warn("issue comment counts not loaded", "err", err)
		}
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
		Comments:  h.cfg.IssueCommentStore,
		Agents:    h.cfg.AgentStore,
		Teams:     h.cfg.TeamStore,
		Workflows: h.cfg.WorkflowStore,
	}
}

func (h *Handler) writeIssueServiceError(w http.ResponseWriter, err error) bool {
	return httputil.WriteServiceError(w, err)
}

func (h *Handler) listIssuesHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.IssueStore, "issues not configured")
	if !ok {
		return
	}
	limit, offset := parseLimitOffset(r.URL.Query(), "limit", "offset", listPageDefault, listPageMax)
	// No parent_id lists every issue in the team, sub-issues included. That is
	// what callers predating the hierarchy expect, so the board opts into the
	// filtered view rather than the endpoint changing under anyone.
	var filter model.ListIssuesFilter
	switch parentID := r.URL.Query().Get("parent_id"); parentID {
	case "":
	case "none":
		filter.TopLevelOnly = true
	default:
		filter.ParentIssueID = parentID
	}
	list, total, err := h.cfg.IssueStore.ListIssuesByTeam(r.Context(), teamID, filter, limit, offset)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_issues", "user_id", userID, "team_id", teamID)
		return
	}
	out := make([]IssueResponse, len(list))
	for i := range list {
		out[i] = issueToResponse(list[i])
	}
	h.decorateIssueResponses(r.Context(), out)
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
		UserID:        userID,
		TeamID:        teamID,
		Title:         req.Title,
		Description:   req.Description,
		ParentIssueID: req.ParentIssueID,
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
	out := []IssueResponse{issueToResponse(*issue)}
	h.decorateIssueResponses(r.Context(), out)
	httputil.WriteJSON(w, http.StatusOK, out[0])
}

// issueRelatives resolves the issue's place in the hierarchy: its parent if it
// is a sub-issue, its children if it is a parent. The hierarchy is two levels
// deep, so an issue is never both.
//
// Lookup failures degrade to no relatives rather than failing the flow
// response, which is the same rule aggregateIssueOutputs follows.
func (h *Handler) issueRelatives(ctx context.Context, issue model.Issue, teamID string) (*IssueResponse, []IssueResponse) {
	children := []IssueResponse{}
	if issue.ParentIssueID != nil && *issue.ParentIssueID != "" {
		parent, err := h.cfg.IssueStore.GetIssue(ctx, *issue.ParentIssueID)
		if err != nil {
			slog.Warn("issue parent not loaded", "err", err, "issue_id", issue.IssueID)
			return nil, children
		}
		if parent == nil || parent.TeamID != teamID {
			return nil, children
		}
		out := issueToResponse(*parent)
		return &out, children
	}
	list, err := h.cfg.IssueStore.ListIssueChildren(ctx, issue.IssueID)
	if err != nil {
		slog.Warn("issue children not loaded", "err", err, "issue_id", issue.IssueID)
		return nil, children
	}
	children = make([]IssueResponse, len(list))
	for i := range list {
		children[i] = issueToResponse(list[i])
	}
	h.decorateIssueResponses(ctx, children)
	return nil, children
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
	limit, offset := parseLimitOffset(r.URL.Query(), "limit", "offset", browsePageDefault, browsePageMax)
	runs, total, err := h.cfg.WorkflowStore.ListWorkflowRunsByIssue(r.Context(), issueID, limit, offset)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_issue_flow_runs", "issue_id", issueID)
		return
	}
	agentTasks := []TaskResponse{}
	var agentTaskModels []model.Task
	if h.cfg.TaskStore != nil {
		tasks, _, err := h.cfg.TaskStore.ListTasksByIssue(r.Context(), issueID, limit, offset)
		if err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "get_issue_flow_agent_tasks", "issue_id", issueID)
			return
		}
		agentTaskModels = tasks
		agentTasks = make([]TaskResponse, len(tasks))
		for i := range tasks {
			agentTasks[i] = taskToResponse(tasks[i])
		}
	}
	stepsByTaskID := map[string]model.WorkflowStepRun{}
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
			if steps[j].TaskID != nil && *steps[j].TaskID != "" {
				stepsByTaskID[*steps[j].TaskID] = steps[j]
			}
		}
		runOut[i] = issueFlowRunResponse{
			Run:   workflowRunToResponse(runs[i]),
			Steps: stepOut,
		}
	}
	outputs, latest := h.aggregateIssueOutputs(r.Context(), agentTaskModels, stepsByTaskID)
	self := []IssueResponse{issueToResponse(*issue)}
	h.decorateIssueResponses(r.Context(), self)
	parentOut, childrenOut := h.issueRelatives(r.Context(), *issue, teamID)
	httputil.WriteJSON(w, http.StatusOK, issueFlowResponse{
		Issue:        self[0],
		Parent:       parentOut,
		Children:     childrenOut,
		Workflow:     workflowOut,
		Runs:         runOut,
		AgentTasks:   agentTasks,
		LatestResult: latest,
		Outputs:      outputs,
		Total:        total,
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
		UserID:        userID,
		TeamID:        teamID,
		IssueID:       issueID,
		Title:         req.Title,
		Description:   req.Description,
		Status:        req.Status,
		AssigneeKind:  req.AssigneeKind,
		AssigneeID:    req.AssigneeID,
		ParentIssueID: req.ParentIssueID,
	})
	if err != nil {
		if h.writeIssueServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "patch_issue", "issue_id", issueID)
		return
	}
	out := []IssueResponse{issueToResponse(*updatedIssue)}
	h.decorateIssueResponses(r.Context(), out)
	httputil.WriteJSON(w, http.StatusOK, out[0])
}
