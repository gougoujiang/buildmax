package blob

import "path"

// ObjectKeyScope groups prefix and run identifiers for blob key construction.
type ObjectKeyScope struct {
	Prefix      string
	WorkspaceID string
	ChatID      string
	TaskRunID   string
}

// PersistObjectKey returns the S3 object key for a persistent workspace file.
// relPath must be already validated with CleanRelPath.
func PersistObjectKey(prefix, workspaceID, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(prefix, workspaceID, "home", clean), nil
}

// ArtifactResultKey returns the S3 object key for a run's result.md (one artifact per task run).
func ArtifactResultKey(prefix, workspaceID, chatID, chatRunID string) string {
	return path.Join(prefix, workspaceID, "artifacts", chatID, chatRunID, "result.md")
}

// ArtifactResultKeyScope returns the S3 object key for a run's result.md using scope.
func ArtifactResultKeyScope(scope ObjectKeyScope) string {
	return path.Join(scope.Prefix, scope.WorkspaceID, "artifacts", scope.ChatID, scope.TaskRunID, "result.md")
}

// ArtifactFileKey returns the S3 object key for one file under a run's output. relPath is validated with CleanRelPath.
func ArtifactFileKey(prefix, workspaceID, chatID, chatRunID, relPath string) (string, error) {
	return ArtifactFileKeyScope(ObjectKeyScope{Prefix: prefix, WorkspaceID: workspaceID, ChatID: chatID, TaskRunID: chatRunID}, relPath)
}

// ArtifactFileKeyScope returns the S3 object key for one file under a run's output using scope.
func ArtifactFileKeyScope(scope ObjectKeyScope, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(scope.Prefix, scope.WorkspaceID, "artifacts", scope.ChatID, scope.TaskRunID, clean), nil
}

// PersistPrefix returns the key prefix under which all persist files for a workspace live (for ListObjectsV2).
func PersistPrefix(prefix, workspaceID string) string {
	return path.Join(prefix, workspaceID, "home") + "/"
}

// ChatBuildmaxObjectKey returns the S3 object key for a task run buildmax file (logs, sessions, settings).
// Deprecated: use TaskRunGlobalObjectKey or TaskRunGlobalObjectKeyScope. Kept for backward compatibility.
func ChatBuildmaxObjectKey(prefix, workspaceID, chatID, chatRunID, relPath string) (string, error) {
	return TaskRunGlobalObjectKeyScope(ObjectKeyScope{Prefix: prefix, WorkspaceID: workspaceID, ChatID: chatID, TaskRunID: chatRunID}, relPath)
}

// TaskRunGlobalObjectKey returns the S3 object key for a task run global dir file (BUILDMAX_HOME: logs, sessions, settings).
// relPath is validated with CleanRelPath (no .., no absolute).
func TaskRunGlobalObjectKey(prefix, workspaceID, chatID, chatRunID, relPath string) (string, error) {
	return TaskRunGlobalObjectKeyScope(ObjectKeyScope{Prefix: prefix, WorkspaceID: workspaceID, ChatID: chatID, TaskRunID: chatRunID}, relPath)
}

// TaskRunGlobalObjectKeyScope returns the S3 object key for a task run global dir file using scope.
func TaskRunGlobalObjectKeyScope(scope ObjectKeyScope, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(scope.Prefix, scope.WorkspaceID, "tasks", scope.ChatID, scope.TaskRunID, "global", clean), nil
}

// TaskRunArtifactsObjectKey returns the S3 object key for a task run artifacts dir file (run output files).
// relPath is validated with CleanRelPath (no .., no absolute).
func TaskRunArtifactsObjectKey(prefix, workspaceID, chatID, chatRunID, relPath string) (string, error) {
	return TaskRunArtifactsObjectKeyScope(ObjectKeyScope{Prefix: prefix, WorkspaceID: workspaceID, ChatID: chatID, TaskRunID: chatRunID}, relPath)
}

// TaskRunArtifactsObjectKeyScope returns the S3 object key for a task run artifacts dir file using scope.
func TaskRunArtifactsObjectKeyScope(scope ObjectKeyScope, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(scope.Prefix, scope.WorkspaceID, "tasks", scope.ChatID, scope.TaskRunID, "artifacts", clean), nil
}
