package executor

import "path/filepath"

// WorkspacePaths provides filesystem layout for workspaces, chats, runs, and artifacts.
// It is injected for testability and to avoid hard dependency on internal/config.
type WorkspacePaths interface {
	PersistentWorkspaceDir(workspaceID string) string
	RuntimeChatRunDir(workspaceID, chatID, chatRunID string) string
	RuntimeChatRunHomeDir(workspaceID, chatID, chatRunID string) string
	RuntimeChatRunArtifactsDir(workspaceID, chatID, chatRunID string) string
	RuntimeChatRunGlobalDir(workspaceID, chatID, chatRunID string) string
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

func (p *workspacePathsRoot) RuntimeChatRunDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(p.root, workspaceID, "chats", chatID, chatRunID)
}

func (p *workspacePathsRoot) RuntimeChatRunHomeDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(p.RuntimeChatRunDir(workspaceID, chatID, chatRunID), "home")
}

func (p *workspacePathsRoot) RuntimeChatRunArtifactsDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(p.RuntimeChatRunDir(workspaceID, chatID, chatRunID), "artifacts")
}

func (p *workspacePathsRoot) RuntimeChatRunGlobalDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(p.RuntimeChatRunDir(workspaceID, chatID, chatRunID), "global")
}

func (p *workspacePathsRoot) RunOutputDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(p.root, workspaceID, "artifacts", chatID, chatRunID)
}
