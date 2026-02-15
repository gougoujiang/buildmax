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
	userID, ok := userIDFromRequest(r, s.cfg.JWTSecret)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		return
	}
	projectID := r.PathValue("project_id")
	if projectID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"project_id required"}`))
		return
	}
	if s.cfg.ProjectStore == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	project, err := s.cfg.ProjectStore.GetProject(r.Context(), projectID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	if project == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"project not found"}`))
		return
	}
	if !s.userOwnsWorkspace(r, userID, project.WorkspaceID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
		return
	}
	if s.cfg.TaskStore == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"tasks not configured"}`))
		return
	}
	list, err := s.cfg.TaskStore.ListTasksByProject(r.Context(), projectID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	out := make([]TaskResponse, len(list))
	for i := range list {
		out[i] = taskToResponse(list[i])
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) createTaskHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromRequest(r, s.cfg.JWTSecret)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		return
	}
	projectID := r.PathValue("project_id")
	if projectID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"project_id required"}`))
		return
	}
	if s.cfg.ProjectStore == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	project, err := s.cfg.ProjectStore.GetProject(r.Context(), projectID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	if project == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"project not found"}`))
		return
	}
	if !s.userOwnsWorkspace(r, userID, project.WorkspaceID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
		return
	}
	if s.cfg.TaskStore == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"tasks not configured"}`))
		return
	}
	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid request body"}`))
		return
	}
	if req.Input == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"input required"}`))
		return
	}
	task, err := s.cfg.TaskStore.CreateTask(r.Context(), projectID, req.Input, userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(taskToResponse(*task))
}
