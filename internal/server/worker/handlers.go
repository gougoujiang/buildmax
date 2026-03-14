package worker

import (
	"encoding/json"
	"net/http"

	"buildmax/internal/storage/entity"
	"buildmax/internal/workerapi"
)

// Handler serves the worker API (GET/PATCH /api/worker/chat-runs/{id}, POST .../stream).
type Handler struct {
	cfg Config
}

// NewHandler returns a handler that serves worker routes using the given config.
func NewHandler(cfg Config) *Handler {
	return &Handler{cfg: cfg}
}

// Register adds worker routes to mux. All routes require worker auth (Bearer or X-Worker-Token).
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/worker/chat-runs/{chat_run_id}", h.authMiddleware(http.HandlerFunc(h.getChatRun)))
	mux.Handle("PATCH /api/worker/chat-runs/{chat_run_id}", h.authMiddleware(http.HandlerFunc(h.patchChatRun)))
	mux.Handle("POST /api/worker/chat-runs/{chat_run_id}/stream", h.authMiddleware(http.HandlerFunc(h.postStream)))
}

func (h *Handler) getChatRun(w http.ResponseWriter, r *http.Request) {
	chatRunID := r.PathValue("chat_run_id")
	if chatRunID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_run_id required")
		return
	}
	if h.cfg.ChatRunStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "chat runs not configured")
		return
	}
	run, chat, err := h.cfg.ChatRunStore.GetChatRunWithChat(r.Context(), chatRunID)
	if err != nil {
		writeInternalError(w, err, "handler", "get_worker_chat_run", "chat_run_id", chatRunID)
		return
	}
	if run == nil || chat == nil {
		writeJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, workerapi.GetChatRunResponse{
		Run: workerapi.ChatRunRun{
			ChatRunID: run.ChatRunID,
			ChatID:    run.ChatID,
			Input:     run.Input,
			Status:    run.Status,
			CreatedAt: run.CreatedAt,
		},
		Chat: workerapi.ChatRunChat{
			ChatID:      chat.ChatID,
			WorkspaceID: chat.WorkspaceID,
			SessionID:   chat.SessionID,
			LastRunID:   chat.LastRunID,
		},
	})
}

func (h *Handler) postStream(w http.ResponseWriter, r *http.Request) {
	chatRunID := r.PathValue("chat_run_id")
	if chatRunID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_run_id required")
		return
	}
	if h.cfg.ChatRunStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "chat runs not configured")
		return
	}
	if h.cfg.Hub == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "stream hub not configured")
		return
	}
	run, _, err := h.cfg.ChatRunStore.GetChatRunWithChat(r.Context(), chatRunID)
	if err != nil || run == nil {
		if err != nil {
			writeInternalError(w, err, "handler", "post_worker_stream", "chat_run_id", chatRunID)
		} else {
			writeJSONError(w, http.StatusNotFound, "run not found")
		}
		return
	}
	var req workerapi.StreamDeltaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	h.cfg.Hub.Append(run.ChatID, req.Delta)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handlePatchRunning handles status RUNNING; returns true if it wrote a response (caller should return).
func (h *Handler) handlePatchRunning(w http.ResponseWriter, r *http.Request, chatRunID string, req *workerapi.PatchChatRunRequest) bool {
	if req.Status != workerapi.StatusRunning {
		return false
	}
	updated, err := h.cfg.ChatRunStore.ClaimChatRun(r.Context(), entity.ClaimChatRunInput{
		ChatRunID:      chatRunID,
		ExpectedStatus: entity.RunStatusScheduled,
		NewStatus:      entity.RunStatusRunning,
		StartedAt:      req.StartedAt,
		SessionID:      req.SessionID,
	})
	if err != nil {
		writeInternalError(w, err, "handler", "patch_worker_chat_run", "chat_run_id", chatRunID)
		return true
	}
	if !updated {
		writeJSONError(w, http.StatusConflict, "run not scheduled or already running")
		return true
	}
	return true
}

// handlePatchTerminalStatus handles status other than RUNNING: update status, OnRunComplete/SyncChatFromRun, Hub.Done. Writes errors and does not write 200.
func (h *Handler) handlePatchTerminalStatus(w http.ResponseWriter, r *http.Request, chatRunID string, req *workerapi.PatchChatRunRequest) bool {
	if err := h.cfg.ChatRunStore.UpdateRun(r.Context(), entity.UpdateChatRunInput{
		ChatRunID:        chatRunID,
		Status:           entity.RunStatus(req.Status),
		StartedAt:        req.StartedAt,
		EndedAt:          req.EndedAt,
		Output:           req.Output,
		ErrorMessage:     req.ErrorMessage,
		SessionID:        req.SessionID,
		PromptTokens:     req.PromptTokens,
		CompletionTokens: req.CompletionTokens,
	}); err != nil {
		writeInternalError(w, err, "handler", "patch_worker_chat_run", "chat_run_id", chatRunID)
		return false
	}
	if req.Status == workerapi.StatusSucceeded && req.Artifact != nil {
		relativePaths := req.Artifact.RelativePaths
		if len(relativePaths) == 0 && req.Artifact.RelativePath != "" {
			relativePaths = []string{req.Artifact.RelativePath}
		}
		if len(relativePaths) == 0 {
			relativePaths = []string{"result.md"}
		}
		if err := h.cfg.ChatRunStore.OnRunComplete(r.Context(), chatRunID, relativePaths); err != nil {
			writeInternalError(w, err, "handler", "patch_worker_chat_run_on_complete", "chat_run_id", chatRunID)
			return false
		}
	} else if req.Status == workerapi.StatusFailed {
		if err := h.cfg.ChatRunStore.SyncChatFromRun(r.Context(), chatRunID); err != nil {
			writeInternalError(w, err, "handler", "patch_worker_chat_run_sync", "chat_run_id", chatRunID)
			return false
		}
	}
	if h.cfg.Hub != nil {
		run, _, _ := h.cfg.ChatRunStore.GetChatRunWithChat(r.Context(), chatRunID)
		if run != nil {
			h.cfg.Hub.Done(run.ChatID)
		}
	}
	return true
}

func (h *Handler) patchChatRun(w http.ResponseWriter, r *http.Request) {
	chatRunID := r.PathValue("chat_run_id")
	if chatRunID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_run_id required")
		return
	}
	if h.cfg.ChatRunStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "chat runs not configured")
		return
	}
	var req workerapi.PatchChatRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Status == "" {
		writeJSONError(w, http.StatusBadRequest, "status required")
		return
	}
	if h.handlePatchRunning(w, r, chatRunID, &req) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if h.handlePatchTerminalStatus(w, r, chatRunID, &req) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
