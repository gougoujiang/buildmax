package work

import (
	"context"
	"errors"
	agentsvc "github.com/gougoujiang/buildmax/internal/service/agent"
	"net/http"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/access"
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

// decorateIssueResponses fills the derived counts for a page of issues. The
// rules for loading them, including that a failure degrades to zero, live in
// the service; this only places them on the response.
func (h *Handler) decorateIssueResponses(ctx context.Context, out []IssueResponse) {
	if len(out) == 0 {
		return
	}
	ids := make([]string, len(out))
	for i := range out {
		ids[i] = out[i].ID
	}
	counts := h.issueService().CountsFor(ctx, ids)
	for i := range out {
		c := counts[out[i].ID]
		out[i].ChildCount = c.Children
		out[i].DoneChildCount = c.DoneChildren
		out[i].CommentCount = c.Comments
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

func (h *Handler) issueService() *issue.Service {
	return &issue.Service{
		Issues:    h.cfg.Issues,
		Comments:  h.cfg.IssueComments,
		Agents:    h.cfg.Agents,
		Teams:     h.cfg.Teams,
		Workflows: h.cfg.Workflows,
	}
}

func (h *Handler) writeIssueServiceError(w http.ResponseWriter, err error) bool {
	return httputil.WriteServiceError(w, err)
}

func (h *Handler) listIssuesHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Issues, "issues not configured")
	if !ok {
		return
	}
	limit, offset := httputil.LimitOffset(r.URL.Query(), "limit", "offset", httputil.ListPageDefault, httputil.ListPageMax)
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
	list, total, err := h.issueService().ListIssues(r.Context(), teamID, filter, limit, offset)
	if err != nil {
		if h.writeIssueServiceError(w, err) {
			return
		}
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
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Issues, "issues not configured")
	if !ok {
		return
	}
	var req createIssueRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
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
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Issues, "issues not configured")
	if !ok {
		return
	}
	issueID, ok := httputil.PathValue(w, r, "issue_id")
	if !ok {
		return
	}
	issue, err := h.issueService().GetIssue(r.Context(), teamID, issueID)
	if err != nil {
		if h.writeIssueServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_issue", "issue_id", issueID)
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

func (h *Handler) getIssueFlowHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Issues, "issues not configured")
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.Workflows, "workflows not configured") {
		return
	}
	issueID, ok := httputil.PathValue(w, r, "issue_id")
	if !ok {
		return
	}
	limit, offset := httputil.LimitOffset(r.URL.Query(), "limit", "offset", httputil.BrowsePageDefault, httputil.BrowsePageMax)
	flow, err := h.loadIssueFlow(r.Context(), teamID, issueID, limit, offset)
	if err != nil {
		if h.writeIssueServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_issue_flow", "issue_id", issueID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, h.issueFlowToResponse(r.Context(), flow))
}

// issueFlowToResponse turns the gathered view into the shape the Portal reads.
func (h *Handler) issueFlowToResponse(ctx context.Context, flow *issueFlow) issueFlowResponse {
	self := []IssueResponse{issueToResponse(flow.Issue)}
	h.decorateIssueResponses(ctx, self)

	var parentOut *IssueResponse
	if flow.Parent != nil {
		out := issueToResponse(*flow.Parent)
		parentOut = &out
	}
	childrenOut := make([]IssueResponse, len(flow.Children))
	for i := range flow.Children {
		childrenOut[i] = issueToResponse(flow.Children[i])
	}
	h.decorateIssueResponses(ctx, childrenOut)

	var workflowOut *workflowResponse
	if flow.Workflow != nil {
		out := workflowToResponse(*flow.Workflow)
		workflowOut = &out
	}

	runOut := make([]issueFlowRunResponse, len(flow.Runs))
	for i := range flow.Runs {
		steps := make([]workflowStepRunResponse, len(flow.Runs[i].Steps))
		for j := range flow.Runs[i].Steps {
			steps[j] = workflowStepRunToResponse(flow.Runs[i].Steps[j])
		}
		runOut[i] = issueFlowRunResponse{Run: workflowRunToResponse(flow.Runs[i].Run), Steps: steps}
	}

	agentTasks := make([]TaskResponse, len(flow.AgentTasks))
	for i := range flow.AgentTasks {
		agentTasks[i] = taskToResponse(flow.AgentTasks[i])
	}

	outputs, latest := h.aggregateIssueOutputs(ctx, flow.AgentTasks, flow.StepsByTaskID)
	return issueFlowResponse{
		Issue:        self[0],
		Parent:       parentOut,
		Children:     childrenOut,
		Workflow:     workflowOut,
		Runs:         runOut,
		AgentTasks:   agentTasks,
		LatestResult: latest,
		Outputs:      outputs,
		Total:        flow.TotalRuns,
	}
}

func (h *Handler) createIssueAgentRunHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Issues, "issues not configured")
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.Agents, "agents not configured") {
		return
	}
	if !httputil.RequireStore(w, h.cfg.Tasks, "tasks not configured") {
		return
	}
	if !httputil.RequireStore(w, h.cfg.Conversations, "conversations not configured") {
		return
	}
	issueID, ok := httputil.PathValue(w, r, "issue_id")
	if !ok {
		return
	}
	var req createIssueAgentRunRequest
	if r.ContentLength != 0 {
		if !httputil.DecodeJSONBody(w, r, &req) {
			return
		}
	}
	issue, err := h.issueService().GetIssue(r.Context(), teamID, issueID)
	if err != nil {
		if h.writeIssueServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_issue_for_agent_run", "issue_id", issueID)
		return
	}
	if issue.AssigneeKind == nil || issue.AssigneeID == nil || *issue.AssigneeKind != model.IssueAssigneeAgent {
		httputil.WriteJSONError(w, http.StatusBadRequest, "issue not assigned to agent")
		return
	}
	// The agent has to belong to this team as well, and asking the agent
	// service is what keeps that one rule rather than a third copy of it.
	if _, err := h.agentService().GetAgent(r.Context(), teamID, *issue.AssigneeID); err != nil {
		if errors.Is(err, agentsvc.ErrAgentNotFound) {
			httputil.WriteJSONError(w, http.StatusBadRequest, "agent not found")
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_agent_for_issue_run", "issue_id", issueID, "agent_id", *issue.AssigneeID)
		return
	}
	conv, err := h.cfg.Conversations.CreateConversationInTeam(r.Context(), teamID, userID, "issue_agent", userID)
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
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.Issues, "issues not configured")
	if !ok {
		return
	}
	issueID, ok := httputil.PathValue(w, r, "issue_id")
	if !ok {
		return
	}
	var req patchIssueRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	if req.AssigneeKind != nil && *req.AssigneeKind == model.IssueAssigneeWorkflow {
		if _, ok := h.guard().TeamAction(w, r, userID, teamID, access.ActionAssignIssueWorkflow); !ok {
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

// agentService answers whether an agent belongs to this team. The work package
// builds its own rather than taking one, because that is the only question it
// asks of agents.
func (h *Handler) agentService() *agentsvc.Service {
	return &agentsvc.Service{Agents: h.cfg.Agents}
}
