package executor

import "path/filepath"

// WorkspacePaths provides filesystem layout for workspaces, tasks, runs, and artifacts.
// It is injected for testability and to avoid hard dependency on internal/config.
type WorkspacePaths interface {
	PersistentWorkspaceDir(workspaceID string) string
	RuntimeWorkspaceDir(workspaceID, taskID string) string
	RuntimeTaskBuildmaxDir(workspaceID, taskID string) string
	RuntimeTaskRunBuildmaxDir(workspaceID, taskID, runID string) string
	RuntimeTaskWSDir(workspaceID, taskID string) string
	ArtifactDir(workspaceID, taskID, runID, artifactID string) string
}

// workspacePathsRoot implements WorkspacePaths with a single root directory (e.g. WorkspacesDir).
type workspacePathsRoot struct {
	root string
}

// NewWorkspacePathsFromRoot returns a WorkspacePaths that uses root as the parent of all workspace dirs.
func NewWorkspacePathsFromRoot(root string) WorkspacePaths {
	return &workspacePathsRoot{root: root}
}

func (p *workspacePathsRoot) PersistentWorkspaceDir(workspaceID string) string {
	return filepath.Join(p.root, workspaceID, "persist")
}

func (p *workspacePathsRoot) RuntimeWorkspaceDir(workspaceID, taskID string) string {
	return filepath.Join(p.root, workspaceID, "tasks", taskID)
}

func (p *workspacePathsRoot) RuntimeTaskBuildmaxDir(workspaceID, taskID string) string {
	return filepath.Join(p.root, workspaceID, "tasks", taskID, "buildmax")
}

func (p *workspacePathsRoot) RuntimeTaskRunBuildmaxDir(workspaceID, taskID, runID string) string {
	return filepath.Join(p.root, workspaceID, "tasks", taskID, runID, "buildmax")
}

func (p *workspacePathsRoot) RuntimeTaskWSDir(workspaceID, taskID string) string {
	return filepath.Join(p.root, workspaceID, "tasks", taskID, "ws")
}

func (p *workspacePathsRoot) ArtifactDir(workspaceID, taskID, runID, artifactID string) string {
	return filepath.Join(p.root, workspaceID, "artifacts", taskID, runID, artifactID)
}

