package server

import (
	"net/http"
	"os"
	"path/filepath"

	"buildmax/internal/config"
)

// WorkspaceResponse is one workspace in the GET /api/workspaces response (snake_case).
type WorkspaceResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	OwnerUserID string `json:"owner_user_id"`
	CreatedAt   int64  `json:"created_at"`
}

// ensureWorkspaceDirs creates directory root/id for each id. Ignores errors (best-effort).
func ensureWorkspaceDirs(root string, workspaceIDs []string) {
	for _, id := range workspaceIDs {
		_ = os.MkdirAll(filepath.Join(root, id), 0755)
	}
}

func (s *Server) workspacesHandler(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorkspaceStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "workspaces not configured")
		return
	}
	userID, ok := requireAuth(w, r, s.cfg.JWTSecret)
	if !ok {
		return
	}
	ctx := r.Context()
	if err := s.cfg.WorkspaceStore.EnsureDefaultWorkspaceForUser(ctx, userID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	list, err := s.cfg.WorkspaceStore.ListWorkspacesByOwner(ctx, userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	ids := make([]string, len(list))
	for i := range list {
		ids[i] = list[i].WorkspaceID
	}
	ensureWorkspaceDirs(config.WorkspacesDir(), ids)
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
