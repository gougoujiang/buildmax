package portal

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"buildmax/internal/server/httputil"
)

// WorkspaceResponse is one workspace in the GET /api/workspaces response (snake_case).
type WorkspaceResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	OwnerUserID string `json:"owner_user_id"`
	CreatedAt   int64  `json:"created_at"`
}

type createWorkspaceRequest struct {
	Name string `json:"name"`
}

func ensureWorkspaceDirs(root string, workspaceIDs []string) {
	for _, id := range workspaceIDs {
		_ = os.MkdirAll(filepath.Join(root, id, "home"), 0755)
	}
}

func (h *Handler) workspacesHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w, h.cfg.WorkspaceStore, "workspaces not configured") {
		return
	}
	userID, ok := requireAuth(w, r, h.cfg.JWTSecret)
	if !ok {
		return
	}
	ctx := r.Context()
	if err := h.cfg.WorkspaceStore.EnsureDefaultWorkspaceForUser(ctx, userID); err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "workspaces", "op", "ensure_default")
		return
	}
	list, err := h.cfg.WorkspaceStore.ListWorkspacesByOwner(ctx, userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "workspaces", "op", "list")
		return
	}
	ids := make([]string, len(list))
	for i := range list {
		ids[i] = list[i].WorkspaceID
	}
	ensureWorkspaceDirs(h.workspacesDir(), ids)
	out := make([]WorkspaceResponse, len(list))
	for i := range list {
		out[i] = WorkspaceResponse{
			ID:          list[i].WorkspaceID,
			Name:        list[i].Name,
			OwnerUserID: list[i].OwnerUserID,
			CreatedAt:   list[i].CreatedAt,
		}
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createWorkspaceHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w, h.cfg.WorkspaceStore, "workspaces not configured") {
		return
	}
	userID, ok := requireAuth(w, r, h.cfg.JWTSecret)
	if !ok {
		return
	}
	var req createWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "name required")
		return
	}
	ctx := r.Context()
	ws, err := h.cfg.WorkspaceStore.CreateWorkspace(ctx, userID, req.Name)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_workspace")
		return
	}
	destDir := h.persistentWorkspaceDir(ws.WorkspaceID)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_workspace", "mkdir", destDir)
		return
	}
	out := WorkspaceResponse{
		ID:          ws.WorkspaceID,
		Name:        ws.Name,
		OwnerUserID: ws.OwnerUserID,
		CreatedAt:   ws.CreatedAt,
	}
	httputil.WriteJSON(w, http.StatusCreated, out)
}
