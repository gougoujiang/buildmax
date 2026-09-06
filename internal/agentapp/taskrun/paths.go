package taskrun

import "path/filepath"

// RuntimePaths provides the space-owned filesystem layout for tasks and runs.
// It is injected for testability and to avoid hard dependency on internal/config.
type RuntimePaths interface {
	RuntimeTaskRunDir(spaceID, taskID, taskRunID string) string
	RuntimeTaskRunWorkspaceDir(spaceID, taskID, taskRunID string) string
	RuntimeTaskRunGlobalDir(spaceID, taskID, taskRunID string) string
}

// runtimePathsRoot implements RuntimePaths with a single root directory.
type runtimePathsRoot struct {
	root string
}

// NewRuntimePathsFromRoot returns a RuntimePaths that uses root as the parent of all space dirs.
func NewRuntimePathsFromRoot(root string) RuntimePaths {
	return &runtimePathsRoot{root: root}
}

func (p *runtimePathsRoot) RuntimeTaskRunDir(spaceID, taskID, taskRunID string) string {
	return filepath.Join(p.root, spaceID, "tasks", taskID, taskRunID)
}

// RuntimeTaskRunWorkspaceDir is the run's workspace/: the Agent's cwd and the
// single writable tool root, materialized from the Space's files and captured
// as the workspace checkpoint. It replaces the former home/ (read copy) plus
// artifacts/ (output) split; see docs/design/task-workspace-checkpoints.md §4.1.
func (p *runtimePathsRoot) RuntimeTaskRunWorkspaceDir(spaceID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(spaceID, taskID, taskRunID), "workspace")
}

func (p *runtimePathsRoot) RuntimeTaskRunGlobalDir(spaceID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(spaceID, taskID, taskRunID), "global")
}
