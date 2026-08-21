package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/service/conversation"
	"github.com/gougoujiang/buildmax/internal/service/task"
)

type TaskResponse struct {
	ID             string  `json:"id"`
	ConversationID string  `json:"conversation_id"`
	SessionID      *string `json:"session_id,omitempty"`
	Status         string  `json:"status"`
	Input          string  `json:"input"`
	Title          string  `json:"title,omitempty"`
	Output         *string `json:"output,omitempty"`
	CreatedBy      string  `json:"created_by"`
	CreatedAt      int64   `json:"created_at"`
	StartedAt      *int64  `json:"started_at,omitempty"`
	EndedAt        *int64  `json:"ended_at,omitempty"`
	ErrorMessage   *string `json:"error_message,omitempty"`
	AgentID        *string `json:"agent_id,omitempty"`
	IssueID        *string `json:"issue_id,omitempty"`
}

type createTaskRequest struct {
	Input   string  `json:"input"`
	AgentID *string `json:"agent_id,omitempty"`
}

func taskToResponse(task model.Task) TaskResponse {
	return TaskResponse{
		ID:             task.TaskID,
		ConversationID: task.ConversationID,
		SessionID:      task.SessionID,
		Status:         task.Status,
		Input:          task.Input,
		Title:          task.Title,
		Output:         task.Output,
		CreatedBy:      task.CreatedBy,
		CreatedAt:      task.CreatedAt,
		StartedAt:      task.StartedAt,
		EndedAt:        task.EndedAt,
		ErrorMessage:   task.ErrorMessage,
		AgentID:        task.AgentID,
		IssueID:        task.IssueID,
	}
}

func (h *Handler) taskService() *task.TaskService {
	var quotaChecker task.QuotaChecker
	if h.cfg.QuotaService != nil {
		quotaChecker = h.cfg.QuotaService
	}
	var workflowSteps task.WorkflowStepLookup
	if h.cfg.WorkflowStore != nil {
		workflowSteps = h.cfg.WorkflowStore
	}
	return &task.TaskService{
		Agents:         h.cfg.AgentStore,
		Tasks:          h.cfg.TaskStore,
		TaskRuns:       h.cfg.TaskRunStore,
		QuotaChecker:   quotaChecker,
		TitleGenerator: h.cfg.TitleGenerator,
		WorkflowSteps:  workflowSteps,
	}
}

// writeTaskServiceError answers a task-service refusal.
//
// agentID is still a parameter because one case is genuinely ambiguous: with no
// agent named in the request, "agent not found" means the task was not found.
func (h *Handler) writeTaskServiceError(w http.ResponseWriter, r *http.Request, err error, agentID *string) bool {
	if errors.Is(err, task.ErrAgentNotFound) && (agentID == nil || *agentID == "") {
		httputil.WriteJSONError(w, http.StatusNotFound, "task not found")
		return true
	}
	var quotaErr *task.QuotaExceededError
	if errors.As(err, &quotaErr) {
		httputil.WriteQuotaExceeded(w, quotaErr.Reason)
		return true
	}
	return httputil.WriteServiceError(w, err)
}

func (h *Handler) getTaskForTeam(w http.ResponseWriter, r *http.Request, teamID, taskID string) (*model.Task, *model.Conversation, bool) {
	task, err := h.cfg.TaskStore.GetTask(r.Context(), taskID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_task", "task_id", taskID)
		return nil, nil, false
	}
	if task == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "task not found")
		return nil, nil, false
	}
	conv, ok := h.getConversationForTeam(w, r, teamID, task.ConversationID)
	if !ok {
		return nil, nil, false
	}
	if task.TeamID != "" && task.TeamID != teamID {
		httputil.WriteJSONError(w, http.StatusNotFound, "task not found")
		return nil, nil, false
	}
	return task, conv, true
}

type tasksListResponse struct {
	Tasks []TaskResponse `json:"tasks"`
	Total int            `json:"total"`
}

func (h *Handler) listConversationTasksHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.TaskStore, "tasks not configured")
	if !ok {
		return
	}
	conversationID, ok := pathValueRequired(w, r, "conversation_id")
	if !ok {
		return
	}
	if _, ok = h.getConversationForTeam(w, r, teamID, conversationID); !ok {
		return
	}
	q := r.URL.Query()
	usePaginated := q.Has("limit") || q.Has("offset") || q.Get("executed_only") == "true"
	if usePaginated {
		limit, offset := parseLimitOffset(q, "limit", "offset", bulkPageDefault, bulkPageMax)
		executedOnly := q.Get("executed_only") == "true"
		list, total, err := h.cfg.TaskStore.ListTasksByConversationPaginated(r.Context(), conversationID, executedOnly, limit, offset)
		if err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "list_tasks", "conversation_id", conversationID)
			return
		}
		out := make([]TaskResponse, len(list))
		for i := range list {
			out[i] = taskToResponse(list[i])
		}
		httputil.WriteJSON(w, http.StatusOK, tasksListResponse{Tasks: out, Total: total})
		return
	}
	order := q.Get("order")
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	list, err := h.cfg.TaskStore.ListTasksByConversation(r.Context(), conversationID, order)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_tasks", "conversation_id", conversationID)
		return
	}
	out := make([]TaskResponse, len(list))
	for i := range list {
		out[i] = taskToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createConversationTaskHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.TaskStore, "tasks not configured")
	if !ok {
		return
	}
	conversationID, ok := pathValueRequired(w, r, "conversation_id")
	if !ok {
		return
	}
	if _, ok = h.getConversationForTeam(w, r, teamID, conversationID); !ok {
		return
	}
	var req createTaskRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	createdTask, err := h.taskService().CreateTask(r.Context(), task.CreateTaskCmd{
		ConversationID: conversationID,
		UserID:         userID,
		TeamID:         teamID,
		Input:          req.Input,
		AgentID:        req.AgentID,
		CreatedByType:  model.RunCreatedByTypeUser,
		TriggerSource:  model.RunTriggerSourcePortalTaskCreate,
	})
	if err != nil {
		if h.writeTaskServiceError(w, r, err, req.AgentID) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "create_task", "conversation_id", conversationID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, taskToResponse(*createdTask))
}

func (h *Handler) getTaskHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.TaskStore, "tasks not configured")
	if !ok {
		return
	}
	taskID, ok := pathValueRequired(w, r, "task_id")
	if !ok {
		return
	}
	task, _, ok := h.getTaskForTeam(w, r, teamID, taskID)
	if !ok {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, taskToResponse(*task))
}

type createTaskRunRequest struct {
	Input string `json:"input"`
}

func (h *Handler) createTaskRunViaConversation(w http.ResponseWriter, r *http.Request, userID, taskID, input string) bool {
	result, err := h.conversationService().RerunTask(r.Context(), conversation.RerunTaskCmd{
		UserID:  userID,
		Channel: "portal",
		Message: input,
		TaskID:  taskID,
	})
	if err != nil {
		if h.writeConversationServiceError(w, r, err, nil) {
			return true
		}
		if errors.Is(err, model.ErrRunInProgress) {
			httputil.WriteJSONError(w, http.StatusConflict, "a run is already in progress for this task")
			return true
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "create_run", "task_id", taskID)
		return true
	}
	if len(result.TaskRunIDs) == 0 {
		httputil.WriteJSONError(w, http.StatusInternalServerError, "no run created")
		return true
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"task_run_id": result.TaskRunIDs[0], "task_id": taskID})
	return true
}

func (h *Handler) createTaskRunHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.TaskRunStore, "task runs not configured")
	if !ok {
		return
	}
	taskID, ok := pathValueRequired(w, r, "task_id")
	if !ok {
		return
	}
	task, _, ok := h.getTaskForTeam(w, r, teamID, taskID)
	if !ok {
		return
	}
	var req createTaskRunRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Input == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "input required")
		return
	}
	h.createTaskRunViaConversation(w, r, userID, task.TaskID, req.Input)
}

// retryTaskResponse names the new run and the one it repeats, so a caller that
// reloads a task can tell which of its runs is the retry.
type retryTaskResponse struct {
	TaskID           string `json:"task_id"`
	TaskRunID        string `json:"task_run_id"`
	RetryOfTaskRunID string `json:"retry_of_task_run_id"`
	Status           string `json:"status"`
}

// retryTaskHandler runs the task's most recent run again, with the same input.
//
// Retry exists because the common reason a run has to be repeated — a worker
// that died, an expired credential, a model that timed out — has nothing to do
// with what the run was asked to do, and making someone retype the instructions
// to recover from it invites them to retype them differently.
func (h *Handler) retryTaskHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.TaskRunStore, "task runs not configured")
	if !ok {
		return
	}
	taskID, ok := pathValueRequired(w, r, "task_id")
	if !ok {
		return
	}
	target, _, ok := h.getTaskForTeam(w, r, teamID, taskID)
	if !ok {
		return
	}
	result, err := h.taskService().RetryRun(r.Context(), task.RetryRunCmd{UserID: userID, TaskID: target.TaskID})
	if err != nil {
		if h.writeTaskServiceError(w, r, err, nil) {
			return
		}
		// model.ErrRunInProgress comes from the store, below the service, so it
		// carries no Kind of its own.
		if errors.Is(err, model.ErrRunInProgress) {
			httputil.WriteJSONError(w, http.StatusConflict, "a run is already in progress for this task")
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "retry_task", "task_id", taskID)
		return
	}
	slog.Info("task run retried", "task_id", target.TaskID, "task_run_id", result.Run.TaskRunID, "retry_of_task_run_id", result.RetriedRun.TaskRunID, "user_id", userID)
	httputil.WriteJSON(w, http.StatusCreated, retryTaskResponse{
		TaskID:           target.TaskID,
		TaskRunID:        result.Run.TaskRunID,
		RetryOfTaskRunID: result.RetriedRun.TaskRunID,
		Status:           result.Run.Status,
	})
}

// cancelTaskResponse reports what the cancel did.
//
// The two outcomes are genuinely different and the caller has to tell them
// apart: a run that had not started is over when this returns, while a started
// one is only asked to stop and keeps running until its worker confirms.
type cancelTaskResponse struct {
	TaskID    string `json:"task_id"`
	TaskRunID string `json:"task_run_id"`
	// Status is the run's status now, not the one it is heading for.
	Status string `json:"status"`
	// CancelRequested is true while the run is still executing and the stop is
	// pending its worker.
	CancelRequested bool `json:"cancel_requested"`
}

// cancelTaskHandler stops the task's in-flight run.
//
// A run that has not been dispatched is canceled outright: no worker holds it,
// so nothing has to agree. A run already with a worker is asked instead, and the
// worker ends it — the server has no way to reach into another process's agent
// loop, and pretending otherwise would leave the run's own record lying about
// what it was doing. `StaleRunReaper` finishes the ones no worker answers for.
func (h *Handler) cancelTaskHandler(w http.ResponseWriter, r *http.Request) {
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.TaskRunStore, "task runs not configured")
	if !ok {
		return
	}
	taskID, ok := pathValueRequired(w, r, "task_id")
	if !ok {
		return
	}
	target, _, ok := h.getTaskForTeam(w, r, teamID, taskID)
	if !ok {
		return
	}
	run, err := h.cfg.TaskRunStore.GetActiveTaskRunByTask(r.Context(), target.TaskID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "cancel_task", "task_id", taskID)
		return
	}
	if run == nil {
		httputil.WriteJSONError(w, http.StatusConflict, "this task has no run in progress")
		return
	}

	// Record the request first. It names who asked and starts the clock the
	// backstop measures, and it stays true whichever of the two paths below the
	// run turns out to be on.
	now := time.Now().Unix()
	requested, err := h.cfg.TaskRunStore.RequestTaskRunCancel(r.Context(), run.TaskRunID, userID, now)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "cancel_task", "task_run_id", run.TaskRunID)
		return
	}
	if h.finishUndispatchedRun(r, run.TaskRunID, now) {
		httputil.WriteJSON(w, http.StatusOK, cancelTaskResponse{
			TaskID:    target.TaskID,
			TaskRunID: run.TaskRunID,
			Status:    string(model.RunStatusCanceled),
		})
		return
	}

	// Not PENDING any more: either a worker has it, or it finished while this
	// request was in flight. Re-reading is what tells those apart.
	current, err := h.cfg.TaskRunStore.GetTaskRun(r.Context(), run.TaskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "cancel_task", "task_run_id", run.TaskRunID)
		return
	}
	if current == nil || model.RunStatusTerminal(current.Status) {
		httputil.WriteJSONError(w, http.StatusConflict, "this run has already finished")
		return
	}
	if !requested && current.CancelRequestedAt == nil {
		httputil.WriteJSONError(w, http.StatusConflict, "this run could not be canceled")
		return
	}
	slog.Info("cancel requested for a running task", "task_id", target.TaskID, "task_run_id", current.TaskRunID, "status", current.Status, "user_id", userID)
	httputil.WriteJSON(w, http.StatusAccepted, cancelTaskResponse{
		TaskID:          target.TaskID,
		TaskRunID:       current.TaskRunID,
		Status:          current.Status,
		CancelRequested: true,
	})
}

// finishUndispatchedRun cancels a run that is still PENDING and reports whether
// it did. The claim is conditional on PENDING, so a run the scheduler picked up
// in the meantime is left to its worker rather than being marked over while it
// runs.
func (h *Handler) finishUndispatchedRun(r *http.Request, taskRunID string, endedAt int64) bool {
	message := "this run was canceled before it started"
	claimed, err := h.cfg.TaskRunStore.ClaimTaskRun(r.Context(), model.ClaimTaskRunInput{
		TaskRunID:      taskRunID,
		ExpectedStatus: model.RunStatusPending,
		NewStatus:      model.RunStatusCanceled,
		EndedAt:        &endedAt,
		ErrorMessage:   &message,
	})
	if err != nil {
		slog.Warn("could not cancel a pending run", "task_run_id", taskRunID, "err", err)
		return false
	}
	if !claimed {
		return false
	}
	if err := h.cfg.TaskRunStore.SyncTaskFromRun(r.Context(), taskRunID); err != nil {
		slog.Warn("could not sync a task from its canceled run", "task_run_id", taskRunID, "err", err)
	}
	h.announceTaskRunTerminal(r.Context(), taskRunID, string(model.RunStatusCanceled), nil, &message)
	return true
}

type SessionMessage struct {
	Role       string            `json:"role"`
	Content    string            `json:"content,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	ToolCalls  []SessionToolCall `json:"tool_calls,omitempty"`
}

type SessionToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

type ConversationResponse struct {
	ID        string           `json:"id"`
	Title     string           `json:"title,omitempty"`
	CreatedAt string           `json:"created_at"`
	Messages  []SessionMessage `json:"messages,omitempty"`
}

func (h *Handler) getTaskConversationHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.TaskStore, "tasks not configured")
	if !ok {
		return
	}
	taskID := r.PathValue("task_id")
	if taskID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_id required")
		return
	}
	task, _, ok := h.getTaskForTeam(w, r, teamID, taskID)
	if !ok {
		return
	}
	if task.SessionID == nil || *task.SessionID == "" {
		httputil.WriteJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	if task.LastRunID == nil || *task.LastRunID == "" {
		httputil.WriteJSONError(w, http.StatusNotFound, "conversation not found")
		return
	}
	sessionID := *task.SessionID
	lastRunID := *task.LastRunID
	data, err := h.loadTaskConversationData(r.Context(), task, lastRunID, sessionID)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, blob.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "conversation file not found")
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_conversation", "task_id", task.TaskID)
		return
	}
	var out ConversationResponse
	if err := json.Unmarshal(data, &out); err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "get_conversation", "task_id", task.TaskID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) loadTaskConversationData(ctx context.Context, task *model.Task, lastRunID, sessionID string) ([]byte, error) {
	return h.readRunGlobal(ctx, task, lastRunID, "sessions/"+sessionID+".json")
}
