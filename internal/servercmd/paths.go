package servercmd

import (
	"path/filepath"

	"buildmax/internal/executor"
)

// serverWorkspacePaths implements executor.WorkspacePaths using a resolved absolute root (set at server startup).
type serverWorkspacePaths struct {
	root string
}

// Ensure serverWorkspacePaths implements executor.WorkspacePaths.
var _ executor.WorkspacePaths = (*serverWorkspacePaths)(nil)

func (p *serverWorkspacePaths) PersistentWorkspaceDir(workspaceID string) string {
	return filepath.Join(p.root, workspaceID, "persist")
}

func (p *serverWorkspacePaths) RuntimeWorkspaceDir(workspaceID, taskID string) string {
	return filepath.Join(p.root, workspaceID, "tasks", taskID)
}

func (p *serverWorkspacePaths) RuntimeTaskBuildmaxDir(workspaceID, taskID string) string {
	return filepath.Join(p.root, workspaceID, "tasks", taskID, "buildmax")
}

func (p *serverWorkspacePaths) RuntimeTaskWSDir(workspaceID, taskID string) string {
	return filepath.Join(p.root, workspaceID, "tasks", taskID, "ws")
}

func (p *serverWorkspacePaths) ArtifactDir(workspaceID, taskID, artifactID string) string {
	return filepath.Join(p.root, workspaceID, "artifacts", taskID, artifactID)
}
