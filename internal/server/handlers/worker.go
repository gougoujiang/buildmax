package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

func (h *Handler) getTaskRun(w http.ResponseWriter, r *http.Request) {
	taskRunID := r.PathValue("task_run_id")
	if taskRunID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_run_id required")
		return
	}
	if h.cfg.TaskRunStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "task runs not configured")
		return
	}
	run, task, err := h.cfg.TaskRunStore.GetTaskRunWithTask(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "get_worker_task_run", "task_run_id", taskRunID)
		return
	}
	if run == nil || task == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	// The agent's instructions are appended to the run's system prompt. Resolving them here
	// rather than at task creation means an edited definition takes effect on the next run,
	// which is what someone editing the field expects. A deleted agent still answers, because
	// a run that already names it has to finish under the identity it was started with.
	agentInstructions := ""
	if task.AgentID != nil && *task.AgentID != "" && h.cfg.AgentStore != nil {
		if a, aerr := h.cfg.AgentStore.GetAgentIncludingDeleted(r.Context(), *task.AgentID); aerr != nil {
			// A run missing its instructions is worse than a run that never had any, but
			// refusing to dispatch it is worse still. Say so and continue.
			workerAPILog().Warn("worker handler: agent instructions unavailable", "task_run_id", taskRunID, "agent_id", *task.AgentID, "err", aerr)
		} else if a != nil && a.TeamID == task.TeamID {
			agentInstructions = a.Instructions
		}
	}

	httputil.WriteJSON(w, http.StatusOK, workerclient.GetTaskRunResponse{
		Run: workerclient.TaskRunRun{
			TaskRunID: run.TaskRunID,
			TaskID:    run.TaskID,
			Input:     run.Input,
			Status:    run.Status,
			// The worker polls this route while it executes, so this field is
			// how a cancel reaches a run that is already under way.
			CancelRequested: run.CancelRequestedAt != nil,
			CreatedAt:       run.CreatedAt,
		},
		Task: workerclient.TaskRunTask{
			TaskID:            task.TaskID,
			ConversationID:    task.ConversationID,
			TeamID:            task.TeamID,
			UserID:            task.CreatedBy,
			SessionID:         task.SessionID,
			LastRunID:         task.LastRunID,
			AgentInstructions: agentInstructions,
		},
		// The server decides how the run reaches a model. A worker executes
		// model-chosen code, so it is told the transport and alias rather than
		// choosing them — and it is told nothing else about the model, because
		// endpoint, upstream identifier, and credential stay on this side.
		LLM: h.cfg.WorkerLLM,
	})
}

func (h *Handler) postStream(w http.ResponseWriter, r *http.Request) {
	taskRunID := r.PathValue("task_run_id")
	if taskRunID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_run_id required")
		return
	}
	if h.cfg.TaskRunStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "task runs not configured")
		return
	}
	run, _, err := h.cfg.TaskRunStore.GetTaskRunWithTask(r.Context(), taskRunID)
	if err != nil || run == nil {
		if err != nil {
			httputil.WriteInternalError(w, err, "worker handler error", "handler", "post_worker_stream", "task_run_id", taskRunID)
		} else {
			httputil.WriteJSONError(w, http.StatusNotFound, "run not found")
		}
		return
	}
	var req workerclient.StreamDeltaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	h.hub.Append(run.TaskID, req.Delta)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handlePatchRunning(w http.ResponseWriter, r *http.Request, taskRunID string, req *workerclient.PatchTaskRunRequest) bool {
	if req.Status != string(model.RunStatusRunning) {
		return false
	}
	updated, err := h.cfg.TaskRunStore.ClaimTaskRun(r.Context(), model.ClaimTaskRunInput{
		TaskRunID:      taskRunID,
		ExpectedStatus: model.RunStatusScheduled,
		NewStatus:      model.RunStatusRunning,
		StartedAt:      req.StartedAt,
		SessionID:      req.SessionID,
	})
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "patch_worker_task_run", "task_run_id", taskRunID)
		return true
	}
	if !updated {
		httputil.WriteJSONError(w, http.StatusConflict, "run not scheduled or already running")
		return true
	}
	return true
}

func (h *Handler) handlePatchTerminalStatus(w http.ResponseWriter, r *http.Request, taskRunID string, req *workerclient.PatchTaskRunRequest) bool {
	if err := h.cfg.TaskRunStore.UpdateRun(r.Context(), model.UpdateTaskRunInput{
		TaskRunID:        taskRunID,
		Status:           model.RunStatus(req.Status),
		StartedAt:        req.StartedAt,
		EndedAt:          req.EndedAt,
		Output:           req.Output,
		ErrorMessage:     req.ErrorMessage,
		SessionID:        req.SessionID,
		PromptTokens:     req.PromptTokens,
		CompletionTokens: req.CompletionTokens,
		TracePath:        req.TracePath,
	}); err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "patch_worker_task_run", "task_run_id", taskRunID)
		return false
	}
	// A canceled run is registered exactly like a finished one. It stopped
	// early, but whatever it produced before stopping is real work, and
	// discarding it would make cancelling more expensive than waiting.
	registersArtifacts := req.Status == string(model.RunStatusSucceeded) || req.Status == string(model.RunStatusCanceled)
	switch {
	case registersArtifacts && req.Artifact != nil:
		relativePaths := req.Artifact.RelativePaths
		if len(relativePaths) == 0 && req.Artifact.RelativePath != "" {
			relativePaths = []string{req.Artifact.RelativePath}
		}
		if len(relativePaths) == 0 {
			relativePaths = []string{"result.md"}
		}
		if err := h.cfg.TaskRunStore.OnRunComplete(r.Context(), taskRunID, relativePaths); err != nil {
			httputil.WriteInternalError(w, err, "worker handler error", "handler", "patch_worker_task_run_on_complete", "task_run_id", taskRunID)
			return false
		}
	case req.Status == string(model.RunStatusFailed) || req.Status == string(model.RunStatusCanceled):
		if err := h.cfg.TaskRunStore.SyncTaskFromRun(r.Context(), taskRunID); err != nil {
			httputil.WriteInternalError(w, err, "worker handler error", "handler", "patch_worker_task_run_sync", "task_run_id", taskRunID)
			return false
		}
	}
	h.announceTaskRunTerminal(r.Context(), taskRunID, req.Status, req.Output, req.ErrorMessage)
	return true
}

// announceTaskRunTerminal closes the run's output stream and tells Tier 1 that
// the run is over.
//
// Every terminal outcome goes through here — reported by a worker or written by
// the server on a cancel — because a conversation that started a task waits for
// exactly one of these. Losing it leaves the conversation waiting for a run that
// has already stopped.
func (h *Handler) announceTaskRunTerminal(ctx context.Context, taskRunID, status string, output, errorMessage *string) {
	run, task, _ := h.cfg.TaskRunStore.GetTaskRunWithTask(ctx, taskRunID)
	if run == nil {
		return
	}
	h.hub.Done(run.TaskID)
	if task == nil {
		return
	}
	info := TaskRunTerminalInfo{
		TaskRunID:      run.TaskRunID,
		TaskID:         run.TaskID,
		ConversationID: task.ConversationID,
		UserID:         task.CreatedBy,
		Status:         status,
		Output:         output,
		ErrorMessage:   errorMessage,
	}
	go func() {
		workerAPILog().Info("firing task run terminal callbacks", "task_run_id", info.TaskRunID, "status", info.Status)
		h.connRegistry.OnTaskRunTerminal(context.Background(), info)
		if h.cfg.OnTaskRunTerminal != nil {
			h.cfg.OnTaskRunTerminal(context.Background(), info)
		}
	}()
}

func (h *Handler) patchTaskRun(w http.ResponseWriter, r *http.Request) {
	taskRunID := r.PathValue("task_run_id")
	if taskRunID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_run_id required")
		return
	}
	if h.cfg.TaskRunStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "task runs not configured")
		return
	}
	var req workerclient.PatchTaskRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Status == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "status required")
		return
	}
	if h.handlePatchRunning(w, r, taskRunID, &req) {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if h.handlePatchTerminalStatus(w, r, taskRunID, &req) {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// Identity belongs in an attr, not in every message string.
func workerAPILog() *slog.Logger { return slog.With("component", "worker_api") }
