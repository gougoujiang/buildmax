package server

import (
	"encoding/json"
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

func (s *Server) workspacesHandler(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WorkspaceStore == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"workspaces not configured"}`))
		return
	}
	userID, ok := userIDFromRequest(r, s.cfg.JWTSecret)
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		return
	}
	ctx := r.Context()
	if err := s.cfg.WorkspaceStore.EnsureDefaultWorkspaceForUser(ctx, userID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	list, err := s.cfg.WorkspaceStore.ListWorkspacesByOwner(ctx, userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	root := config.WorkspacesDir()
	for _, w := range list {
		dir := filepath.Join(root, w.ID)
		_ = os.MkdirAll(dir, 0755)
	}
	out := make([]WorkspaceResponse, len(list))
	for i := range list {
		out[i] = WorkspaceResponse{
			ID:          list[i].ID,
			Name:        list[i].Name,
			OwnerUserID: list[i].OwnerUserID,
			CreatedAt:   list[i].CreatedAt,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}
