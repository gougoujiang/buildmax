package server

import (
	"encoding/json"
	"net/http"

	"buildmax/internal/model"
)

// TaskResponse is one task in the list/create response (snake_case).
type TaskResponse struct {
	ID           string  `json:"id"`
	WorkspaceID  string  `json:"workspace_id"`
	ProjectID    *string `json:"project_id,omitempty"`
	SessionID    *string `json:"session_id,omitempty"`
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

func taskToResponse(t model.Task) TaskResponse {
	return TaskResponse{
		ID:           t.TaskID,
		WorkspaceID:  t.WorkspaceID,
		ProjectID:    t.ProjectID,
		SessionID:    t.SessionID,
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

// resolveProjectForWorkspace resolves projectID (optional). If empty, returns (nil, true).
// If non-empty, validates project exists and belongs to workspace; on success returns (project, true),
// on failure writes the appropriate error and returns (nil, false).
func (s *Server) resolveProjectForWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, projectID string) (*model.Project, bool) {
	if projectID == "" {
		return nil, true
	}
	if !s.requireProjectStore(w) {
		return nil, false
	}
	project, err := s.cfg.ProjectStore.GetProject(r.Context(), projectID)
	if err != nil {
		writeInternalError(w, err, "handler", "resolve_project", "project_id", projectID)
		return nil, false
	}
	if project == nil {
		writeJSONError(w, http.StatusNotFound, "project not found")
		return nil, false
	}
	if project.WorkspaceID != workspaceID {
		writeJSONError(w, http.StatusBadRequest, "project does not belong to workspace")
		return nil, false
	}
	return project, true
}

// listWorkspaceTasksHandler handles GET /api/workspaces/{workspace_id}/tasks.
// Optional query param project_id filters by project (validates project belongs to workspace).
func (s *Server) listWorkspaceTasksHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireTaskStore(w) {
		return
	}
	var projectIDPtr *string
	if pid := r.URL.Query().Get("project_id"); pid != "" {
		project, ok := s.resolveProjectForWorkspace(w, r, workspaceID, pid)
		if !ok {
			return
		}
		if project != nil {
			projectIDPtr = &project.ProjectID
		}
	}
	list, err := s.cfg.TaskStore.ListTasksByWorkspace(r.Context(), workspaceID, projectIDPtr)
	if err != nil {
		writeInternalError(w, err, "handler", "list_tasks", "workspace_id", workspaceID)
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
	userID, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireTaskStore(w) {
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
		project, ok := s.resolveProjectForWorkspace(w, r, workspaceID, req.ProjectID)
		if !ok {
			return
		}
		if project != nil {
			projectIDPtr = &project.ProjectID
		}
	}
	task, err := s.cfg.TaskStore.CreateTask(r.Context(), workspaceID, projectIDPtr, req.Input, userID)
	if err != nil {
		writeInternalError(w, err, "handler", "create_task", "workspace_id", workspaceID)
		return
	}
	writeJSON(w, http.StatusCreated, taskToResponse(*task))
}
