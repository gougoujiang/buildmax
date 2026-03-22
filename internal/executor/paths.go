package executor

import "path/filepath"

// RuntimePaths provides filesystem layout for user files, tasks, runs, and artifacts.
// It is injected for testability and to avoid hard dependency on internal/config.
type RuntimePaths interface {
	PersistentUserDir(userID string) string
	RuntimeTaskRunDir(userID, conversationID, taskID, taskRunID string) string
	RuntimeTaskRunHomeDir(userID, conversationID, taskID, taskRunID string) string
	RuntimeTaskRunArtifactsDir(userID, conversationID, taskID, taskRunID string) string
	RuntimeTaskRunGlobalDir(userID, conversationID, taskID, taskRunID string) string
	RunOutputDir(userID, conversationID, taskID, taskRunID string) string
}

// runtimePathsRoot implements RuntimePaths with a single root directory.
type runtimePathsRoot struct {
	root string
}

// NewRuntimePathsFromRoot returns a RuntimePaths that uses root as the parent of all user dirs.
func NewRuntimePathsFromRoot(root string) RuntimePaths {
	return &runtimePathsRoot{root: root}
}

func (p *runtimePathsRoot) PersistentUserDir(userID string) string {
	return filepath.Join(p.root, userID, "home")
}

func (p *runtimePathsRoot) RuntimeTaskRunDir(userID, conversationID, taskID, taskRunID string) string {
	return filepath.Join(p.root, userID, "conversations", conversationID, "tasks", taskID, taskRunID)
}

func (p *runtimePathsRoot) RuntimeTaskRunHomeDir(userID, conversationID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(userID, conversationID, taskID, taskRunID), "home")
}

func (p *runtimePathsRoot) RuntimeTaskRunArtifactsDir(userID, conversationID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(userID, conversationID, taskID, taskRunID), "artifacts")
}

func (p *runtimePathsRoot) RuntimeTaskRunGlobalDir(userID, conversationID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(userID, conversationID, taskID, taskRunID), "global")
}

func (p *runtimePathsRoot) RunOutputDir(userID, conversationID, taskID, taskRunID string) string {
	return filepath.Join(p.root, userID, "artifacts", conversationID, taskID, taskRunID)
}
