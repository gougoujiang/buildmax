package portalapi

import (
	"path/filepath"

	"buildmax/internal/config"
)

func (h *Handler) workspacesDir() string {
	if h.cfg.WorkspacesDir != "" {
		return h.cfg.WorkspacesDir
	}
	return config.WorkspacesDir()
}

func (h *Handler) persistentWorkspaceDir(workspaceID string) string {
	return filepath.Join(h.workspacesDir(), workspaceID, "home")
}
