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

// userOwnsWorkspace returns true if the user owns the workspace (workspace id is in the user's list).
func (s *Server) userOwnsWorkspace(r *http.Request, userID, workspaceID string) bool {
	if s.cfg.WorkspaceStore == nil {
		return false
	}
	list, err := s.cfg.WorkspaceStore.ListWorkspacesByOwner(r.Context(), userID)
	if err != nil {
		return false
	}
	for _, w := range list {
		if w.ID == workspaceID {
			return true
		}
	}
	return false
}

func (s *Server) listProjectsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromRequest(r, s.cfg.JWTSecret)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		return
	}
	workspaceID := r.PathValue("workspace_id")
	if workspaceID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"workspace_id required"}`))
		return
	}
	if !s.userOwnsWorkspace(r, userID, workspaceID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
		return
	}
	if s.cfg.ProjectStore == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"projects not configured"}`))
		return
	}
	list, err := s.cfg.ProjectStore.ListProjectsByWorkspace(r.Context(), workspaceID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	out := make([]ProjectResponse, len(list))
	for i := range list {
		out[i] = projectToResponse(list[i])
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) createProjectHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromRequest(r, s.cfg.JWTSecret)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		return
	}
	workspaceID := r.PathValue("workspace_id")
	if workspaceID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"workspace_id required"}`))
		return
	}
	if !s.userOwnsWorkspace(r, userID, workspaceID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
		return
	}
	if s.cfg.ProjectStore == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"projects not configured"}`))
		return
	}
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid request body"}`))
		return
	}
	if req.Name == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"name required"}`))
		return
	}
	project, err := s.cfg.ProjectStore.CreateProject(r.Context(), workspaceID, req.Name, req.Description)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(projectToResponse(*project))
}

func projectToResponse(p store.Project) ProjectResponse {
	return ProjectResponse{
		ID:          p.ID,
		WorkspaceID: p.WorkspaceID,
		Name:        p.Name,
		Description: p.Description,
		CreatedAt:   p.CreatedAt,
	}
}
