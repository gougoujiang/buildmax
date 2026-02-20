package server

import (
	"encoding/json"
	"net/http"

	"buildmax/internal/workerapi"
)

// getWorkerTaskRunHandler handles GET /api/worker/task-runs/{run_id}. Returns run and task for the worker.
func (s *Server) getWorkerTaskRunHandler(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	if runID == "" {
		writeJSONError(w, http.StatusBadRequest, "run_id required")
		return
	}
	if !s.requireStore(w, s.cfg.TaskRunStore, "task runs not configured") {
		return
	}
	run, task, err := s.cfg.TaskRunStore.GetTaskRunWithTask(r.Context(), runID)
	if err != nil {
		writeInternalError(w, err, "handler", "get_worker_task_run", "run_id", runID)
		return
	}
	if run == nil || task == nil {
		writeJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, workerapi.GetTaskRunResponse{
		Run: workerapi.TaskRunRun{
			RunID:     run.RunID,
			TaskID:    run.TaskID,
			Input:     run.Input,
			Status:    run.Status,
			CreatedAt: run.CreatedAt,
		},
		Task: workerapi.TaskRunTask{
			TaskID:      task.TaskID,
			WorkspaceID: task.WorkspaceID,
			ProjectID:   task.ProjectID,
			SessionID:   task.SessionID,
			LastRunID:   task.LastRunID,
		},
	})
}

// patchWorkerTaskRunHandler handles PATCH /api/worker/task-runs/{run_id}. Updates run status; on SUCCEEDED with artifact calls OnRunComplete, on FAILED calls SyncTaskFromRun.
func (s *Server) patchWorkerTaskRunHandler(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	if runID == "" {
		writeJSONError(w, http.StatusBadRequest, "run_id required")
		return
	}
	if !s.requireStore(w, s.cfg.TaskRunStore, "task runs not configured") {
		return
	}
	var req workerapi.PatchTaskRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Status == "" {
		writeJSONError(w, http.StatusBadRequest, "status required")
		return
	}

	if req.Status == workerapi.StatusRunning {
		updated, err := s.cfg.TaskRunStore.UpdateTaskRunStatusIf(r.Context(), runID, workerapi.StatusScheduled, workerapi.StatusRunning, req.StartedAt, nil, nil, nil, req.SessionID)
		if err != nil {
			writeInternalError(w, err, "handler", "patch_worker_task_run", "run_id", runID)
			return
		}
		if !updated {
			writeJSONError(w, http.StatusConflict, "run not scheduled or already running")
			return
		}
	} else {
		if err := s.cfg.TaskRunStore.UpdateTaskRunStatus(r.Context(), runID, req.Status, req.StartedAt, req.EndedAt, req.Output, req.ErrorMessage, req.SessionID); err != nil {
			writeInternalError(w, err, "handler", "patch_worker_task_run", "run_id", runID)
			return
		}
		if req.Status == workerapi.StatusSucceeded && req.Artifact != nil && req.Artifact.ArtifactID != "" && req.Artifact.RelativePath != "" {
			if err := s.cfg.TaskRunStore.OnRunComplete(r.Context(), runID, req.Artifact.ArtifactID, req.Artifact.RelativePath); err != nil {
				writeInternalError(w, err, "handler", "patch_worker_task_run_on_complete", "run_id", runID)
				return
			}
		} else if req.Status == workerapi.StatusFailed {
			if err := s.cfg.TaskRunStore.SyncTaskFromRun(r.Context(), runID); err != nil {
				writeInternalError(w, err, "handler", "patch_worker_task_run_sync", "run_id", runID)
				return
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
