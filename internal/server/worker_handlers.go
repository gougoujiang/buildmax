package server

import (
	"encoding/json"
	"net/http"

	"buildmax/internal/workerapi"
)

// getWorkerChatRunHandler handles GET /api/worker/chat-runs/{chat_run_id}. Returns run and chat for the worker.
func (s *Server) getWorkerChatRunHandler(w http.ResponseWriter, r *http.Request) {
	chatRunID := r.PathValue("chat_run_id")
	if chatRunID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_run_id required")
		return
	}
	if !s.requireStore(w, s.cfg.ChatRunStore, "chat runs not configured") {
		return
	}
	run, chat, err := s.cfg.ChatRunStore.GetChatRunWithChat(r.Context(), chatRunID)
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

// postWorkerStreamHandler handles POST /api/worker/chat-runs/{chat_run_id}/stream. Appends delta to the chat's stream buffer (hub keyed by chat_id).
func (s *Server) postWorkerStreamHandler(w http.ResponseWriter, r *http.Request) {
	chatRunID := r.PathValue("chat_run_id")
	if chatRunID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_run_id required")
		return
	}
	if !s.requireStore(w, s.cfg.ChatRunStore, "chat runs not configured") {
		return
	}
	if s.hub == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "stream hub not configured")
		return
	}
	run, _, err := s.cfg.ChatRunStore.GetChatRunWithChat(r.Context(), chatRunID)
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
	s.hub.Append(run.ChatID, req.Delta)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// patchWorkerChatRunHandler handles PATCH /api/worker/chat-runs/{chat_run_id}. Updates run status; on SUCCEEDED with artifact calls OnRunComplete, on FAILED calls SyncChatFromRun.
func (s *Server) patchWorkerChatRunHandler(w http.ResponseWriter, r *http.Request) {
	chatRunID := r.PathValue("chat_run_id")
	if chatRunID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_run_id required")
		return
	}
	if !s.requireStore(w, s.cfg.ChatRunStore, "chat runs not configured") {
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

	if req.Status == workerapi.StatusRunning {
		updated, err := s.cfg.ChatRunStore.UpdateChatRunStatusIf(r.Context(), chatRunID, workerapi.StatusScheduled, workerapi.StatusRunning, req.StartedAt, nil, nil, nil, req.SessionID)
		if err != nil {
			writeInternalError(w, err, "handler", "patch_worker_chat_run", "chat_run_id", chatRunID)
			return
		}
		if !updated {
			writeJSONError(w, http.StatusConflict, "run not scheduled or already running")
			return
		}
	} else {
		if err := s.cfg.ChatRunStore.UpdateChatRunStatus(r.Context(), chatRunID, req.Status, req.StartedAt, req.EndedAt, req.Output, req.ErrorMessage, req.SessionID); err != nil {
			writeInternalError(w, err, "handler", "patch_worker_chat_run", "chat_run_id", chatRunID)
			return
		}
		if req.Status == workerapi.StatusSucceeded && req.Artifact != nil {
			relativePaths := req.Artifact.RelativePaths
			if len(relativePaths) == 0 && req.Artifact.RelativePath != "" {
				relativePaths = []string{req.Artifact.RelativePath}
			}
			if len(relativePaths) == 0 {
				relativePaths = []string{"result.md"}
			}
			if err := s.cfg.ChatRunStore.OnRunComplete(r.Context(), chatRunID, relativePaths); err != nil {
				writeInternalError(w, err, "handler", "patch_worker_chat_run_on_complete", "chat_run_id", chatRunID)
				return
			}
		} else if req.Status == workerapi.StatusFailed {
			if err := s.cfg.ChatRunStore.SyncChatFromRun(r.Context(), chatRunID); err != nil {
				writeInternalError(w, err, "handler", "patch_worker_chat_run_sync", "chat_run_id", chatRunID)
				return
			}
		}
		// Release stream buffer for this chat (hub keyed by chat_id).
		if s.hub != nil {
			run, _, _ := s.cfg.ChatRunStore.GetChatRunWithChat(r.Context(), chatRunID)
			if run != nil {
				s.hub.Done(run.ChatID)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
