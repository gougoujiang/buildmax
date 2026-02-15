package server

import (
	"encoding/json"
	"net/http"

	"buildmax/internal/store"
)

// TaskResponse is one task in the list/create response (snake_case). Id is the task_id (UUID).
type TaskResponse struct {
	ID            string  `json:"id"`
	ProjectID     string  `json:"project_id"`
	Status        string  `json:"status"`
	Input         string  `json:"input"`
	Output        *string `json:"output,omitempty"`
	CreatedBy     string  `json:"created_by"`
	CreatedAt     int64   `json:"created_at"`
	StartedAt     *int64  `json:"started_at,omitempty"`
	EndedAt       *int64  `json:"ended_at,omitempty"`
	ErrorMessage  *string `json:"error_message,omitempty"`
}

// createTaskRequest is the JSON body for POST /api/projects/{project_id}/tasks.
type createTaskRequest struct {
	Input string `json:"input"`
}

func taskToResponse(t store.Task) TaskResponse {
	return TaskResponse{
		ID:           t.TaskID,
		ProjectID:    t.ProjectID,
		Status:       t.Status,
		Input:        t.Input,
		Output:       t.Output,
		CreatedBy:    t.CreatedBy,
		CreatedAt:    t.CreatedAt,
		StartedAt:    t.StartedAt,
		EndedAt:      t.EndedAt,
		ErrorMessage: t.ErrorMessage,
	}
}

func (s *Server) listTasksHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r, s.cfg.JWTSecret)
	if !ok {
		return
	}
	projectID := r.PathValue("project_id")
	if projectID == "" {
		writeJSONError(w, http.StatusBadRequest, "project_id required")
		return
	}
	if s.cfg.ProjectStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "projects not configured")
		return
	}
	project, err := s.cfg.ProjectStore.GetProject(r.Context(), projectID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if project == nil {
		writeJSONError(w, http.StatusNotFound, "project not found")
		return
	}
	owned, err := s.userOwnsWorkspace(r, userID, project.WorkspaceID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !owned {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}
	if s.cfg.TaskStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "tasks not configured")
		return
	}
	list, err := s.cfg.TaskStore.ListTasksByProject(r.Context(), projectID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]TaskResponse, len(list))
	for i := range list {
		out[i] = taskToResponse(list[i])
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createTaskHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r, s.cfg.JWTSecret)
	if !ok {
		return
	}
	projectID := r.PathValue("project_id")
	if projectID == "" {
		writeJSONError(w, http.StatusBadRequest, "project_id required")
		return
	}
	if s.cfg.ProjectStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "projects not configured")
		return
	}
	project, err := s.cfg.ProjectStore.GetProject(r.Context(), projectID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if project == nil {
		writeJSONError(w, http.StatusNotFound, "project not found")
		return
	}
	owned, err := s.userOwnsWorkspace(r, userID, project.WorkspaceID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !owned {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}
	if s.cfg.TaskStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "tasks not configured")
		return
	}
	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Input == "" {
		writeJSONError(w, http.StatusBadRequest, "input required")
		return
	}
	task, err := s.cfg.TaskStore.CreateTask(r.Context(), projectID, req.Input, userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, taskToResponse(*task))
}
