package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// workerAuthMiddleware requires Authorization: Bearer <token> or X-Worker-Token.
func (h *Handler) workerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.cfg.WorkerToken == "" {
			httputil.WriteJSONError(w, http.StatusUnauthorized, "worker auth not configured")
			return
		}
		token := ""
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		} else if t := r.Header.Get("X-Worker-Token"); t != "" {
			token = t
		}
		if token != h.cfg.WorkerToken {
			httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid or missing worker token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

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
	httputil.WriteJSON(w, http.StatusOK, workerclient.GetTaskRunResponse{
		Run: workerclient.TaskRunRun{
			TaskRunID: run.TaskRunID,
			TaskID:    run.TaskID,
			Input:     run.Input,
			Status:    run.Status,
			CreatedAt: run.CreatedAt,
		},
		Task: workerclient.TaskRunTask{
			TaskID:         task.TaskID,
			ConversationID: task.ConversationID,
			TeamID:         task.TeamID,
			UserID:         task.CreatedBy,
			SessionID:      task.SessionID,
			LastRunID:      task.LastRunID,
		},
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
	}); err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "patch_worker_task_run", "task_run_id", taskRunID)
		return false
	}
	if req.Status == string(model.RunStatusSucceeded) && req.Artifact != nil {
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
	} else if req.Status == string(model.RunStatusFailed) {
		if err := h.cfg.TaskRunStore.SyncTaskFromRun(r.Context(), taskRunID); err != nil {
			httputil.WriteInternalError(w, err, "worker handler error", "handler", "patch_worker_task_run_sync", "task_run_id", taskRunID)
			return false
		}
	}
	run, task, _ := h.cfg.TaskRunStore.GetTaskRunWithTask(r.Context(), taskRunID)
	if run != nil {
		h.hub.Done(run.TaskID)
	}
	if run != nil && task != nil {
		info := TaskRunTerminalInfo{
			TaskRunID:      run.TaskRunID,
			TaskID:         run.TaskID,
			ConversationID: task.ConversationID,
			UserID:         task.CreatedBy,
			Status:         req.Status,
			Output:         req.Output,
			ErrorMessage:   req.ErrorMessage,
		}
		go func() {
			slog.Info("worker: firing task run terminal callbacks", "task_run_id", info.TaskRunID, "status", info.Status)
			h.connRegistry.OnTaskRunTerminal(context.Background(), info)
			if h.cfg.OnTaskRunTerminal != nil {
				h.cfg.OnTaskRunTerminal(context.Background(), info)
			}
		}()
	}
	return true
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
