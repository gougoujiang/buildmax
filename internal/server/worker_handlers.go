package server

import (
	"encoding/json"
	"net/http"
)

// workerTaskResponse is the JSON response for GET /api/worker/tasks/{task_id} (snake_case).
type workerTaskResponse struct {
	TaskID      string  `json:"task_id"`
	WorkspaceID string  `json:"workspace_id"`
	ProjectID   *string `json:"project_id,omitempty"`
	Status      string  `json:"status"`
	Input       string  `json:"input"`
	CreatedBy   string  `json:"created_by"`
	CreatedAt   int64   `json:"created_at"`
}

// patchWorkerTaskRequest is the JSON body for PATCH /api/worker/tasks/{task_id} (snake_case).
type patchWorkerTaskRequest struct {
	Status       string  `json:"status"`
	SessionID    *string `json:"session_id,omitempty"`
	StartedAt    *int64  `json:"started_at,omitempty"`
	EndedAt      *int64  `json:"ended_at,omitempty"`
	Output       *string `json:"output,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
	Artifact     *struct {
		ArtifactID   string `json:"artifact_id"`
		RelativePath string `json:"relative_path"`
	} `json:"artifact,omitempty"`
}

// getWorkerTaskHandler handles GET /api/worker/tasks/{task_id}. Returns task details by task_id (worker does not need workspace_id).
func (s *Server) getWorkerTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	if taskID == "" {
		writeJSONError(w, http.StatusBadRequest, "task_id required")
		return
	}
	if !s.requireTaskStore(w) {
		return
	}
	task, err := s.cfg.TaskStore.GetTask(r.Context(), taskID)
	if err != nil {
		writeInternalError(w, err, "handler", "get_worker_task", "task_id", taskID)
		return
	}
	if task == nil {
		writeJSONError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, workerTaskResponse{
		TaskID:      task.TaskID,
		WorkspaceID: task.WorkspaceID,
		ProjectID:   task.ProjectID,
		Status:      task.Status,
		Input:       task.Input,
		CreatedBy:   task.CreatedBy,
		CreatedAt:   task.CreatedAt,
	})
}

// patchWorkerTaskHandler handles PATCH /api/worker/tasks/{task_id}. Updates task status and optional artifact.
func (s *Server) patchWorkerTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	if taskID == "" {
		writeJSONError(w, http.StatusBadRequest, "task_id required")
		return
	}
	if !s.requireTaskStore(w) {
		return
	}
	var req patchWorkerTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Status == "" {
		writeJSONError(w, http.StatusBadRequest, "status required")
		return
	}

	if req.Status == "RUNNING" {
		updated, err := s.cfg.TaskStore.UpdateTaskStatusIf(r.Context(), taskID, "SCHEDULED", "RUNNING", req.StartedAt, nil, nil, nil, req.SessionID)
		if err != nil {
			writeInternalError(w, err, "handler", "patch_worker_task", "task_id", taskID)
			return
		}
		if !updated {
			writeJSONError(w, http.StatusConflict, "task not scheduled or already running")
			return
		}
	} else {
		if err := s.cfg.TaskStore.UpdateTaskStatus(r.Context(), taskID, req.Status, req.StartedAt, req.EndedAt, req.Output, req.ErrorMessage, req.SessionID); err != nil {
			writeInternalError(w, err, "handler", "patch_worker_task", "task_id", taskID)
			return
		}
	}

	if req.Artifact != nil && req.Artifact.ArtifactID != "" && req.Artifact.RelativePath != "" {
		if !s.requireArtifactStore(w) {
			return
		}
		newSeq, err := s.cfg.TaskStore.IncrementTaskSeq(r.Context(), taskID)
		if err != nil {
			writeInternalError(w, err, "handler", "patch_worker_task_increment_seq", "task_id", taskID)
			return
		}
		if err := s.cfg.ArtifactStore.CreateArtifactWithItem(r.Context(), taskID, req.Artifact.ArtifactID, newSeq, req.Artifact.RelativePath); err != nil {
			writeInternalError(w, err, "handler", "patch_worker_task_create_artifact", "task_id", taskID)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
