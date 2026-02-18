package executor

// WorkspacePaths provides filesystem layout for workspaces, tasks, and artifacts.
// It is injected for testability and to avoid hard dependency on internal/config.
type WorkspacePaths interface {
	PersistentWorkspaceDir(workspaceID string) string
	RuntimeWorkspaceDir(workspaceID, taskID string) string
	RuntimeTaskBuildmaxDir(workspaceID, taskID string) string
	RuntimeTaskWSDir(workspaceID, taskID string) string
	ArtifactDir(workspaceID, taskID, artifactID string) string
}

