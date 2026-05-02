package portalapi

import (
	"path/filepath"
)

func (h *Handler) workspacesDir() string {
	return h.cfg.WorkspacesDir
}

func (h *Handler) persistentWorkspaceDir(workspaceID string) string {
	return filepath.Join(h.workspacesDir(), workspaceID, "home")
}
