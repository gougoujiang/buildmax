package portal

import (
	"net/http"

	workflowapp "buildmax/internal/app/workflow"
	"buildmax/internal/server/httputil"
	"buildmax/internal/storage/entity"
)

type workflowResponse struct {
	ID          string `json:"id"`
	TeamID      string `json:"team_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Definition  string `json:"definition"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type workflowRunResponse struct {
	ID             string  `json:"id"`
	WorkflowID     string  `json:"workflow_id"`
	IssueID        *string `json:"issue_id,omitempty"`
	ConversationID string  `json:"conversation_id"`
	Status         string  `json:"status"`
	CreatedBy      string  `json:"created_by"`
	CreatedAt      int64   `json:"created_at"`
	StartedAt      *int64  `json:"started_at,omitempty"`
	EndedAt        *int64  `json:"ended_at,omitempty"`
	ErrorMessage   *string `json:"error_message,omitempty"`
}

type workflowStepRunResponse struct {
	ID            string  `json:"id"`
	WorkflowRunID string  `json:"workflow_run_id"`
	StepID        string  `json:"step_id"`
	StepIndex     int     `json:"step_index"`
	StepType      string  `json:"step_type"`
	TargetAgentID *string `json:"target_agent_id,omitempty"`
	Prompt        string  `json:"prompt"`
	Status        string  `json:"status"`
	TaskID        *string `json:"task_id,omitempty"`
	TaskRunID     *string `json:"task_run_id,omitempty"`
	OutputSummary *string `json:"output_summary,omitempty"`
	ErrorMessage  *string `json:"error_message,omitempty"`
	CreatedAt     int64   `json:"created_at"`
	StartedAt     *int64  `json:"started_at,omitempty"`
	EndedAt       *int64  `json:"ended_at,omitempty"`
}

type workflowListResponse struct {
	Workflows []workflowResponse `json:"workflows"`
}

type workflowRunListResponse struct {
	Runs  []workflowRunResponse `json:"runs"`
	Total int                   `json:"total"`
}

type workflowRunDetailResponse struct {
	Run   workflowRunResponse       `json:"run"`
	Steps []workflowStepRunResponse `json:"steps"`
}

type createWorkflowRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Definition  string `json:"definition"`
}

type patchWorkflowRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Definition  *string `json:"definition"`
}

type createWorkflowRunRequest struct {
	IssueID *string `json:"issue_id,omitempty"`
}

func workflowToResponse(workflow entity.Workflow) workflowResponse {
	return workflowResponse{
		ID:          workflow.WorkflowID,
		TeamID:      workflow.TeamID,
		Name:        workflow.Name,
		Description: workflow.Description,
		Definition:  workflow.Definition,
		CreatedBy:   workflow.CreatedBy,
		CreatedAt:   workflow.CreatedAt,
		UpdatedAt:   workflow.UpdatedAt,
	}
}

func workflowRunToResponse(run entity.WorkflowRun) workflowRunResponse {
	return workflowRunResponse{
		ID:             run.WorkflowRunID,
		WorkflowID:     run.WorkflowID,
		IssueID:        run.IssueID,
		ConversationID: run.ConversationID,
		Status:         run.Status,
		CreatedBy:      run.CreatedBy,
		CreatedAt:      run.CreatedAt,
		StartedAt:      run.StartedAt,
		EndedAt:        run.EndedAt,
		ErrorMessage:   run.ErrorMessage,
	}
}

func workflowStepRunToResponse(step entity.WorkflowStepRun) workflowStepRunResponse {
	return workflowStepRunResponse{
		ID:            step.StepRunID,
		WorkflowRunID: step.WorkflowRunID,
		StepID:        step.StepID,
		StepIndex:     step.StepIndex,
		StepType:      step.StepType,
		TargetAgentID: step.TargetAgentID,
		Prompt:        step.Prompt,
		Status:        step.Status,
		TaskID:        step.TaskID,
		TaskRunID:     step.TaskRunID,
		OutputSummary: step.OutputSummary,
		ErrorMessage:  step.ErrorMessage,
		CreatedAt:     step.CreatedAt,
		StartedAt:     step.StartedAt,
		EndedAt:       step.EndedAt,
	}
}

func (h *Handler) listWorkflowsHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.WorkflowStore, "workflows not configured")
	if !ok {
		return
	}
	list, err := h.workflowService().ListWorkflows(r.Context(), teamID)
	if err != nil {
		if h.writeWorkflowServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_workflows", "team_id", teamID)
		return
	}
	out := make([]workflowResponse, len(list))
	for i := range list {
		out[i] = workflowToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, workflowListResponse{Workflows: out})
}

func (h *Handler) createWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.WorkflowStore, "workflows not configured")
	if !ok {
		return
	}
	var req createWorkflowRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	workflow, err := h.workflowService().CreateWorkflow(r.Context(), workflowapp.CreateWorkflowCmd{
		TeamID:      teamID,
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		Definition:  req.Definition,
	})
	if err != nil {
		if h.writeWorkflowServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_workflow", "team_id", teamID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, workflowToResponse(*workflow))
}

func (h *Handler) getWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.WorkflowStore, "workflows not configured")
	if !ok {
		return
	}
	workflowID, ok := pathValueRequired(w, r, "workflow_id")
	if !ok {
		return
	}
	workflow, err := h.workflowService().GetWorkflow(r.Context(), teamID, workflowID)
	if err != nil {
		if h.writeWorkflowServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_workflow", "team_id", teamID, "workflow_id", workflowID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, workflowToResponse(*workflow))
}

func (h *Handler) patchWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.WorkflowStore, "workflows not configured")
	if !ok {
		return
	}
	workflowID, ok := pathValueRequired(w, r, "workflow_id")
	if !ok {
		return
	}
	var req patchWorkflowRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	workflow, err := h.workflowService().UpdateWorkflow(r.Context(), workflowapp.UpdateWorkflowCmd{
		TeamID:      teamID,
		WorkflowID:  workflowID,
		Name:        req.Name,
		Description: req.Description,
		Definition:  req.Definition,
	})
	if err != nil {
		if h.writeWorkflowServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "patch_workflow", "team_id", teamID, "workflow_id", workflowID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, workflowToResponse(*workflow))
}

func (h *Handler) listWorkflowRunsHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.WorkflowStore, "workflows not configured")
	if !ok {
		return
	}
	workflowID, ok := pathValueRequired(w, r, "workflow_id")
	if !ok {
		return
	}
	limit, offset := parseLimitOffset(r.URL.Query(), "limit", "offset", 20, 100)
	runs, total, err := h.workflowService().ListWorkflowRuns(r.Context(), teamID, workflowID, limit, offset)
	if err != nil {
		if h.writeWorkflowServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_workflow_runs", "team_id", teamID, "workflow_id", workflowID)
		return
	}
	out := make([]workflowRunResponse, len(runs))
	for i := range runs {
		out[i] = workflowRunToResponse(runs[i])
	}
	httputil.WriteJSON(w, http.StatusOK, workflowRunListResponse{Runs: out, Total: total})
}

func (h *Handler) getWorkflowRunHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.WorkflowStore, "workflows not configured")
	if !ok {
		return
	}
	runID, ok := pathValueRequired(w, r, "workflow_run_id")
	if !ok {
		return
	}
	run, steps, err := h.workflowService().GetWorkflowRunDetail(r.Context(), teamID, runID)
	if err != nil {
		if h.writeWorkflowServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_workflow_run", "team_id", teamID, "workflow_run_id", runID)
		return
	}
	stepOut := make([]workflowStepRunResponse, len(steps))
	for i := range steps {
		stepOut[i] = workflowStepRunToResponse(steps[i])
	}
	httputil.WriteJSON(w, http.StatusOK, workflowRunDetailResponse{Run: workflowRunToResponse(*run), Steps: stepOut})
}

func (h *Handler) createWorkflowRunHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.WorkflowStore, "workflows not configured")
	if !ok {
		return
	}
	workflowID, ok := pathValueRequired(w, r, "workflow_id")
	if !ok {
		return
	}
	var req createWorkflowRunRequest
	if r.ContentLength != 0 {
		if !decodeJSONBody(w, r, &req) {
			return
		}
	}
	run, steps, err := h.workflowService().StartWorkflowRun(r.Context(), workflowapp.StartWorkflowRunCmd{
		TeamID:     teamID,
		UserID:     userID,
		WorkflowID: workflowID,
		IssueID:    req.IssueID,
	})
	if err != nil {
		if h.writeWorkflowServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_workflow_run", "team_id", teamID, "workflow_id", workflowID)
		return
	}
	stepOut := make([]workflowStepRunResponse, len(steps))
	for i := range steps {
		stepOut[i] = workflowStepRunToResponse(steps[i])
	}
	httputil.WriteJSON(w, http.StatusCreated, workflowRunDetailResponse{Run: workflowRunToResponse(*run), Steps: stepOut})
}

func (h *Handler) createIssueWorkflowRunHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.WorkflowStore, "workflows not configured")
	if !ok {
		return
	}
	issueID, ok := pathValueRequired(w, r, "issue_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.IssueStore, "issues not configured") {
		return
	}
	issue, err := h.cfg.IssueStore.GetIssue(r.Context(), issueID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "get_issue_for_workflow_run", "team_id", teamID, "issue_id", issueID)
		return
	}
	if issue == nil || issue.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "issue not found")
		return
	}
	if issue.AssigneeKind == nil || issue.AssigneeID == nil || *issue.AssigneeKind != entity.IssueAssigneeWorkflow {
		httputil.WriteJSONError(w, http.StatusBadRequest, "issue not assigned to workflow")
		return
	}
	run, steps, err := h.workflowService().StartWorkflowRun(r.Context(), workflowapp.StartWorkflowRunCmd{
		TeamID:     teamID,
		UserID:     userID,
		WorkflowID: *issue.AssigneeID,
		IssueID:    &issueID,
	})
	if err != nil {
		if h.writeWorkflowServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_issue_workflow_run", "team_id", teamID, "issue_id", issueID)
		return
	}
	stepOut := make([]workflowStepRunResponse, len(steps))
	for i := range steps {
		stepOut[i] = workflowStepRunToResponse(steps[i])
	}
	httputil.WriteJSON(w, http.StatusCreated, workflowRunDetailResponse{Run: workflowRunToResponse(*run), Steps: stepOut})
}
