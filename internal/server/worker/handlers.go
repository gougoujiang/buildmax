package worker

import (
	"encoding/json"
	"net/http"

	"buildmax/internal/server/httputil"
	"buildmax/internal/storage/entity"
	"buildmax/internal/workerapi"
)

// Handler serves the worker API (GET/PATCH /api/worker/task-runs/{id}, POST .../stream).
type Handler struct {
	cfg Config
}

// NewHandler returns a handler that serves worker routes using the given config.
func NewHandler(cfg Config) *Handler {
	return &Handler{cfg: cfg}
}

// Register adds worker routes to mux. All routes require worker auth (Bearer or X-Worker-Token).
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/worker/task-runs/{task_run_id}", h.authMiddleware(http.HandlerFunc(h.getTaskRun)))
	mux.Handle("PATCH /api/worker/task-runs/{task_run_id}", h.authMiddleware(http.HandlerFunc(h.patchTaskRun)))
	mux.Handle("POST /api/worker/task-runs/{task_run_id}/stream", h.authMiddleware(http.HandlerFunc(h.postStream)))
}

func (h *Handler) getTaskRun(w http.ResponseWriter, r *http.Request) {
	chatRunID := r.PathValue("task_run_id")
	if chatRunID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_run_id required")
		return
	}
	if h.cfg.TaskRunStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "task runs not configured")
		return
	}
	run, chat, err := h.cfg.TaskRunStore.GetTaskRunWithChat(r.Context(), chatRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "get_worker_task_run", "task_run_id", chatRunID)
		return
	}
	if run == nil || chat == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, workerapi.GetTaskRunResponse{
		Run: workerapi.TaskRunRun{
			TaskRunID: run.TaskRunID,
			ChatID:    run.ChatID,
			Input:     run.Input,
			Status:    run.Status,
			CreatedAt: run.CreatedAt,
		},
		Task: workerapi.TaskRunTask{
			ChatID:         chat.ChatID,
			ConversationID: chat.ConversationID,
			UserID:         chat.CreatedBy,
			SessionID:      chat.SessionID,
			LastRunID:      chat.LastRunID,
		},
	})
}

func (h *Handler) postStream(w http.ResponseWriter, r *http.Request) {
	chatRunID := r.PathValue("task_run_id")
	if chatRunID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_run_id required")
		return
	}
	if h.cfg.TaskRunStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "task runs not configured")
		return
	}
	if h.cfg.Hub == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "stream hub not configured")
		return
	}
	run, _, err := h.cfg.TaskRunStore.GetTaskRunWithChat(r.Context(), chatRunID)
	if err != nil || run == nil {
		if err != nil {
			httputil.WriteInternalError(w, err, "worker handler error", "handler", "post_worker_stream", "task_run_id", chatRunID)
		} else {
			httputil.WriteJSONError(w, http.StatusNotFound, "run not found")
		}
		return
	}
	var req workerapi.StreamDeltaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	h.cfg.Hub.Append(run.ChatID, req.Delta)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handlePatchRunning handles status RUNNING; returns true if it wrote a response (caller should return).
func (h *Handler) handlePatchRunning(w http.ResponseWriter, r *http.Request, chatRunID string, req *workerapi.PatchTaskRunRequest) bool {
	if req.Status != string(entity.RunStatusRunning) {
		return false
	}
	updated, err := h.cfg.TaskRunStore.ClaimTaskRun(r.Context(), entity.ClaimTaskRunInput{
		TaskRunID:      chatRunID,
		ExpectedStatus: entity.RunStatusScheduled,
		NewStatus:      entity.RunStatusRunning,
		StartedAt:      req.StartedAt,
		SessionID:      req.SessionID,
	})
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "patch_worker_task_run", "task_run_id", chatRunID)
		return true
	}
	if !updated {
		httputil.WriteJSONError(w, http.StatusConflict, "run not scheduled or already running")
		return true
	}
	return true
}

// handlePatchTerminalStatus handles status other than RUNNING: update status, OnRunComplete/SyncTaskFromRun, Hub.Done. Writes errors and does not write 200.
func (h *Handler) handlePatchTerminalStatus(w http.ResponseWriter, r *http.Request, chatRunID string, req *workerapi.PatchTaskRunRequest) bool {
	if err := h.cfg.TaskRunStore.UpdateRun(r.Context(), entity.UpdateTaskRunInput{
		TaskRunID:        chatRunID,
		Status:           entity.RunStatus(req.Status),
		StartedAt:        req.StartedAt,
		EndedAt:          req.EndedAt,
		Output:           req.Output,
		ErrorMessage:     req.ErrorMessage,
		SessionID:        req.SessionID,
		PromptTokens:     req.PromptTokens,
		CompletionTokens: req.CompletionTokens,
	}); err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "patch_worker_task_run", "task_run_id", chatRunID)
		return false
	}
	if req.Status == string(entity.RunStatusSucceeded) && req.Artifact != nil {
		relativePaths := req.Artifact.RelativePaths
		if len(relativePaths) == 0 && req.Artifact.RelativePath != "" {
			relativePaths = []string{req.Artifact.RelativePath}
		}
		if len(relativePaths) == 0 {
			relativePaths = []string{"result.md"}
		}
		if err := h.cfg.TaskRunStore.OnRunComplete(r.Context(), chatRunID, relativePaths); err != nil {
			httputil.WriteInternalError(w, err, "worker handler error", "handler", "patch_worker_task_run_on_complete", "task_run_id", chatRunID)
			return false
		}
	} else if req.Status == string(entity.RunStatusFailed) {
		if err := h.cfg.TaskRunStore.SyncTaskFromRun(r.Context(), chatRunID); err != nil {
			httputil.WriteInternalError(w, err, "worker handler error", "handler", "patch_worker_task_run_sync", "task_run_id", chatRunID)
			return false
		}
	}
	if h.cfg.Hub != nil {
		run, _, _ := h.cfg.TaskRunStore.GetTaskRunWithChat(r.Context(), chatRunID)
		if run != nil {
			h.cfg.Hub.Done(run.ChatID)
		}
	}
	return true
}

func (h *Handler) patchTaskRun(w http.ResponseWriter, r *http.Request) {
	chatRunID := r.PathValue("task_run_id")
	if chatRunID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_run_id required")
		return
	}
	if h.cfg.TaskRunStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "task runs not configured")
		return
	}
	var req workerapi.PatchTaskRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Status == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "status required")
		return
	}
	if h.handlePatchRunning(w, r, chatRunID, &req) {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if h.handlePatchTerminalStatus(w, r, chatRunID, &req) {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
