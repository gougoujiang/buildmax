package executor

import "path/filepath"

// WorkspacePaths provides filesystem layout for workspaces, chats, runs, and artifacts.
// It is injected for testability and to avoid hard dependency on internal/config.
type WorkspacePaths interface {
	PersistentWorkspaceDir(workspaceID string) string
	RuntimeWorkspaceDir(workspaceID, chatID string) string
	RuntimeChatBuildmaxDir(workspaceID, chatID string) string
	RuntimeChatRunBuildmaxDir(workspaceID, chatID, chatRunID string) string
	RuntimeChatWSDir(workspaceID, chatID string) string
	ArtifactDir(workspaceID, chatID, chatRunID, artifactID string) string
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

func (p *workspacePathsRoot) RuntimeWorkspaceDir(workspaceID, chatID string) string {
	return filepath.Join(p.root, workspaceID, "chats", chatID)
}

func (p *workspacePathsRoot) RuntimeChatBuildmaxDir(workspaceID, chatID string) string {
	return filepath.Join(p.root, workspaceID, "chats", chatID, "buildmax")
}

func (p *workspacePathsRoot) RuntimeChatRunBuildmaxDir(workspaceID, chatID, chatRunID string) string {
	return filepath.Join(p.root, workspaceID, "chats", chatID, chatRunID, "buildmax")
}

func (p *workspacePathsRoot) RuntimeChatWSDir(workspaceID, chatID string) string {
	return filepath.Join(p.root, workspaceID, "chats", chatID, "ws")
}

func (p *workspacePathsRoot) ArtifactDir(workspaceID, chatID, chatRunID, artifactID string) string {
	return filepath.Join(p.root, workspaceID, "artifacts", chatID, chatRunID, artifactID)
}
