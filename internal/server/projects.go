package server

import (
	"encoding/json"
	"net/http"

	"buildmax/internal/store"
)

// ProjectResponse is one project in the list/create response (snake_case).
type ProjectResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
}

// createProjectRequest is the JSON body for POST /api/workspaces/{workspace_id}/projects.
type createProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// userOwnsWorkspace returns true if the user owns the workspace.
func (s *Server) userOwnsWorkspace(r *http.Request, userID, workspaceID string) (bool, error) {
	if s.cfg.WorkspaceStore == nil {
		return false, nil
	}
	return s.cfg.WorkspaceStore.WorkspaceBelongsToUser(r.Context(), workspaceID, userID)
}

func (s *Server) listProjectsHandler(w http.ResponseWriter, r *http.Request) {
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
	if s.cfg.ProjectStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "projects not configured")
		return
	}
	list, err := s.cfg.ProjectStore.ListProjectsByWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]ProjectResponse, len(list))
	for i := range list {
		out[i] = projectToResponse(list[i])
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createProjectHandler(w http.ResponseWriter, r *http.Request) {
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
	if s.cfg.ProjectStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "projects not configured")
		return
	}
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name required")
		return
	}
	project, err := s.cfg.ProjectStore.CreateProject(r.Context(), workspaceID, req.Name, req.Description)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, projectToResponse(*project))
}

func projectToResponse(p store.Project) ProjectResponse {
	return ProjectResponse{
		ID:          p.ProjectID,
		WorkspaceID: p.WorkspaceID,
		Name:        p.Name,
		Description: p.Description,
		CreatedAt:   p.CreatedAt,
	}
}
