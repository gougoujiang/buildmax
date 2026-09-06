package taskrun

import "path/filepath"

// RuntimePaths provides the space-owned filesystem layout for tasks and runs.
// It is injected for testability and to avoid hard dependency on internal/config.
type RuntimePaths interface {
	RuntimeTaskRunDir(spaceID, taskID, taskRunID string) string
	RuntimeTaskRunHomeDir(spaceID, taskID, taskRunID string) string
	RuntimeTaskRunArtifactsDir(spaceID, taskID, taskRunID string) string
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

func (p *runtimePathsRoot) RuntimeTaskRunHomeDir(spaceID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(spaceID, taskID, taskRunID), "home")
}

func (p *runtimePathsRoot) RuntimeTaskRunArtifactsDir(spaceID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(spaceID, taskID, taskRunID), "artifacts")
}

func (p *runtimePathsRoot) RuntimeTaskRunGlobalDir(spaceID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(spaceID, taskID, taskRunID), "global")
}
