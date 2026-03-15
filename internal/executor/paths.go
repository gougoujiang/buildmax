package executor

import "path/filepath"

// WorkspacePaths provides filesystem layout for workspaces, tasks, runs, and artifacts.
// It is injected for testability and to avoid hard dependency on internal/config.
type WorkspacePaths interface {
	PersistentWorkspaceDir(workspaceID string) string
	RuntimeTaskRunDir(workspaceID, chatID, chatRunID string) string
	RuntimeTaskRunHomeDir(workspaceID, chatID, chatRunID string) string
	RuntimeTaskRunArtifactsDir(workspaceID, chatID, chatRunID string) string
	RuntimeTaskRunGlobalDir(workspaceID, chatID, chatRunID string) string
	RunOutputDir(workspaceID, chatID, chatRunID string) string
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
	return filepath.Join(p.root, workspaceID, "home")
}

func (p *workspacePathsRoot) RuntimeTaskRunDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(p.root, workspaceID, "tasks", chatID, chatRunID)
}

func (p *workspacePathsRoot) RuntimeTaskRunHomeDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(workspaceID, chatID, chatRunID), "home")
}

func (p *workspacePathsRoot) RuntimeTaskRunArtifactsDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(workspaceID, chatID, chatRunID), "artifacts")
}

func (p *workspacePathsRoot) RuntimeTaskRunGlobalDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(workspaceID, chatID, chatRunID), "global")
}

func (p *workspacePathsRoot) RunOutputDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(p.root, workspaceID, "artifacts", chatID, chatRunID)
}
