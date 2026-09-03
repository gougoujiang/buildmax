package taskrun

import "path/filepath"

// RuntimePaths provides the team-owned filesystem layout for tasks and runs.
// It is injected for testability and to avoid hard dependency on internal/config.
type RuntimePaths interface {
	RuntimeTaskRunDir(teamID, taskID, taskRunID string) string
	RuntimeTaskRunHomeDir(teamID, taskID, taskRunID string) string
	RuntimeTaskRunArtifactsDir(teamID, taskID, taskRunID string) string
	RuntimeTaskRunGlobalDir(teamID, taskID, taskRunID string) string
}

// runtimePathsRoot implements RuntimePaths with a single root directory.
type runtimePathsRoot struct {
	root string
}

// NewRuntimePathsFromRoot returns a RuntimePaths that uses root as the parent of all team dirs.
func NewRuntimePathsFromRoot(root string) RuntimePaths {
	return &runtimePathsRoot{root: root}
}

func (p *runtimePathsRoot) RuntimeTaskRunDir(teamID, taskID, taskRunID string) string {
	return filepath.Join(p.root, teamID, "tasks", taskID, taskRunID)
}

func (p *runtimePathsRoot) RuntimeTaskRunHomeDir(teamID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(teamID, taskID, taskRunID), "home")
}

func (p *runtimePathsRoot) RuntimeTaskRunArtifactsDir(teamID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(teamID, taskID, taskRunID), "artifacts")
}

func (p *runtimePathsRoot) RuntimeTaskRunGlobalDir(teamID, taskID, taskRunID string) string {
	return filepath.Join(p.RuntimeTaskRunDir(teamID, taskID, taskRunID), "global")
}
