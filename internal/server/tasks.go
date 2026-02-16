package server

import (
	"encoding/json"
	"net/http"

	"buildmax/internal/store"
)

// TaskResponse is one task in the list/create response (snake_case).
type TaskResponse struct {
	ID           string  `json:"id"`
	WorkspaceID  string  `json:"workspace_id"`
	ProjectID    *string `json:"project_id,omitempty"`
	Status       string  `json:"status"`
	Input        string  `json:"input"`
	Output       *string `json:"output,omitempty"`
	CreatedBy    string  `json:"created_by"`
	CreatedAt    int64   `json:"created_at"`
	StartedAt    *int64  `json:"started_at,omitempty"`
	EndedAt      *int64  `json:"ended_at,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
}

// createTaskRequest is the JSON body for POST /api/workspaces/{workspace_id}/tasks.
type createTaskRequest struct {
	Input     string `json:"input"`
	ProjectID string `json:"project_id"`
}

func taskToResponse(t store.Task) TaskResponse {
	return TaskResponse{
		ID:           t.TaskID,
		WorkspaceID:  t.WorkspaceID,
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

// listWorkspaceTasksHandler handles GET /api/workspaces/{workspace_id}/tasks.
// Optional query param project_id filters by project (validates project belongs to workspace).
func (s *Server) listWorkspaceTasksHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r, s.cfg.JWTSecret)
	if !ok {
		return
	}
	workspaceID := r.PathValue("workspace_id")
	if workspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace_id required")
		return
	}
	owned, err := s.userOwnsWorkspace(r, userID, workspaceID)
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

	var projectIDPtr *string
	if pid := r.URL.Query().Get("project_id"); pid != "" {
		if s.cfg.ProjectStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "projects not configured")
			return
		}
		project, err := s.cfg.ProjectStore.GetProject(r.Context(), pid)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if project == nil {
			writeJSONError(w, http.StatusNotFound, "project not found")
			return
		}
		if project.WorkspaceID != workspaceID {
			writeJSONError(w, http.StatusBadRequest, "project does not belong to workspace")
			return
		}
		projectIDPtr = &pid
	}

	list, err := s.cfg.TaskStore.ListTasksByWorkspace(r.Context(), workspaceID, projectIDPtr)
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

// createWorkspaceTaskHandler handles POST /api/workspaces/{workspace_id}/tasks.
// Body: { "input": "…", "project_id": "optional" }.
func (s *Server) createWorkspaceTaskHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r, s.cfg.JWTSecret)
	if !ok {
		return
	}
	workspaceID := r.PathValue("workspace_id")
	if workspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace_id required")
		return
	}
	owned, err := s.userOwnsWorkspace(r, userID, workspaceID)
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

	var projectIDPtr *string
	if req.ProjectID != "" {
		if s.cfg.ProjectStore == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "projects not configured")
			return
		}
		project, err := s.cfg.ProjectStore.GetProject(r.Context(), req.ProjectID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if project == nil {
			writeJSONError(w, http.StatusNotFound, "project not found")
			return
		}
		if project.WorkspaceID != workspaceID {
			writeJSONError(w, http.StatusBadRequest, "project does not belong to workspace")
			return
		}
		projectIDPtr = &req.ProjectID
	}

	task, err := s.cfg.TaskStore.CreateTask(r.Context(), workspaceID, projectIDPtr, req.Input, userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, taskToResponse(*task))
}
