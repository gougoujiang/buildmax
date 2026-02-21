package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// WorkspaceResponse is one workspace in the GET /api/workspaces response (snake_case).
type WorkspaceResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	OwnerUserID string `json:"owner_user_id"`
	CreatedAt   int64  `json:"created_at"`
}

// createWorkspaceRequest is the JSON body for POST /api/workspaces.
type createWorkspaceRequest struct {
	Name string `json:"name"`
}

// ensureWorkspaceDirs creates workspace root and home subdir for each workspace. Ignores errors (best-effort).
// Layout: root/<workspaceID>/home (so that PersistentWorkspaceDir(workspaceID) exists).
func ensureWorkspaceDirs(root string, workspaceIDs []string) {
	for _, id := range workspaceIDs {
		_ = os.MkdirAll(filepath.Join(root, id, "home"), 0755)
	}
}

func (s *Server) workspacesHandler(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.cfg.WorkspaceStore, "workspaces not configured") {
		return
	}
	userID, ok := requireAuth(w, r, s.cfg.JWTSecret)
	if !ok {
		return
	}
	ctx := r.Context()
	if err := s.cfg.WorkspaceStore.EnsureDefaultWorkspaceForUser(ctx, userID); err != nil {
		writeInternalError(w, err, "handler", "workspaces", "op", "ensure_default")
		return
	}
	list, err := s.cfg.WorkspaceStore.ListWorkspacesByOwner(ctx, userID)
	if err != nil {
		writeInternalError(w, err, "handler", "workspaces", "op", "list")
		return
	}
	ids := make([]string, len(list))
	for i := range list {
		ids[i] = list[i].WorkspaceID
	}
	ensureWorkspaceDirs(s.workspacesDir(), ids)
	out := make([]WorkspaceResponse, len(list))
	for i := range list {
		out[i] = WorkspaceResponse{
			ID:          list[i].WorkspaceID,
			Name:        list[i].Name,
			OwnerUserID: list[i].OwnerUserID,
			CreatedAt:   list[i].CreatedAt,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createWorkspaceHandler(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.cfg.WorkspaceStore, "workspaces not configured") {
		return
	}
	userID, ok := requireAuth(w, r, s.cfg.JWTSecret)
	if !ok {
		return
	}
	var req createWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "name required")
		return
	}
	ctx := r.Context()
	ws, err := s.cfg.WorkspaceStore.CreateWorkspace(ctx, userID, req.Name)
	if err != nil {
		writeInternalError(w, err, "handler", "create_workspace")
		return
	}
	destDir := s.persistentWorkspaceDir(ws.WorkspaceID)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		writeInternalError(w, err, "handler", "create_workspace", "mkdir", destDir)
		return
	}
	out := WorkspaceResponse{
		ID:          ws.WorkspaceID,
		Name:        ws.Name,
		OwnerUserID: ws.OwnerUserID,
		CreatedAt:   ws.CreatedAt,
	}
	writeJSON(w, http.StatusCreated, out)
}
