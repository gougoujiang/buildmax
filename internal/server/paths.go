package server

import (
	"path/filepath"

	"buildmax/internal/config"
)

func (s *Server) workspacesDir() string {
	if s.cfg.Storage.WorkspacesDir != "" {
		return s.cfg.Storage.WorkspacesDir
	}
	return config.WorkspacesDir()
}

func (s *Server) persistentWorkspaceDir(workspaceID string) string {
	return filepath.Join(s.workspacesDir(), workspaceID, "home")
}

