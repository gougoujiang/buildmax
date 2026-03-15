package blob

import "path"

// ObjectKeyScope groups prefix and run identifiers for blob key construction.
type ObjectKeyScope struct {
	Prefix         string
	UserID         string
	ConversationID string
	TaskID         string
	TaskRunID      string
}

// PersistObjectKey returns the S3 object key for a persistent user file.
// relPath must be already validated with CleanRelPath.
func PersistObjectKey(prefix, userID, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(prefix, userID, "home", clean), nil
}

// ArtifactResultKey returns the S3 object key for a run's result.md (one artifact per task run).
func ArtifactResultKey(prefix, userID, conversationID, taskID, taskRunID string) string {
	return path.Join(prefix, userID, "artifacts", conversationID, taskID, taskRunID, "result.md")
}

// ArtifactResultKeyScope returns the S3 object key for a run's result.md using scope.
func ArtifactResultKeyScope(scope ObjectKeyScope) string {
	return path.Join(scope.Prefix, scope.UserID, "artifacts", scope.ConversationID, scope.TaskID, scope.TaskRunID, "result.md")
}

// ArtifactFileKey returns the S3 object key for one file under a run's output. relPath is validated with CleanRelPath.
func ArtifactFileKey(prefix, userID, conversationID, taskID, taskRunID, relPath string) (string, error) {
	return ArtifactFileKeyScope(ObjectKeyScope{Prefix: prefix, UserID: userID, ConversationID: conversationID, TaskID: taskID, TaskRunID: taskRunID}, relPath)
}

// ArtifactFileKeyScope returns the S3 object key for one file under a run's output using scope.
func ArtifactFileKeyScope(scope ObjectKeyScope, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(scope.Prefix, scope.UserID, "artifacts", scope.ConversationID, scope.TaskID, scope.TaskRunID, clean), nil
}

// PersistPrefix returns the key prefix under which all persist files for a user live (for ListObjectsV2).
func PersistPrefix(prefix, userID string) string {
	return path.Join(prefix, userID, "home") + "/"
}

// TaskBuildmaxObjectKey returns the S3 object key for a task run buildmax file (logs, sessions, settings).
// Deprecated: use TaskRunGlobalObjectKey or TaskRunGlobalObjectKeyScope. Kept for backward compatibility.
func TaskBuildmaxObjectKey(prefix, userID, conversationID, taskID, taskRunID, relPath string) (string, error) {
	return TaskRunGlobalObjectKeyScope(ObjectKeyScope{Prefix: prefix, UserID: userID, ConversationID: conversationID, TaskID: taskID, TaskRunID: taskRunID}, relPath)
}

// TaskRunGlobalObjectKey returns the S3 object key for a task run global dir file (BUILDMAX_HOME: logs, sessions, settings).
// relPath is validated with CleanRelPath (no .., no absolute).
func TaskRunGlobalObjectKey(prefix, userID, conversationID, taskID, taskRunID, relPath string) (string, error) {
	return TaskRunGlobalObjectKeyScope(ObjectKeyScope{Prefix: prefix, UserID: userID, ConversationID: conversationID, TaskID: taskID, TaskRunID: taskRunID}, relPath)
}

// TaskRunGlobalObjectKeyScope returns the S3 object key for a task run global dir file using scope.
func TaskRunGlobalObjectKeyScope(scope ObjectKeyScope, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(scope.Prefix, scope.UserID, "conversations", scope.ConversationID, "tasks", scope.TaskID, scope.TaskRunID, "global", clean), nil
}

// TaskRunArtifactsObjectKey returns the S3 object key for a task run artifacts dir file (run output files).
// relPath is validated with CleanRelPath (no .., no absolute).
func TaskRunArtifactsObjectKey(prefix, userID, conversationID, taskID, taskRunID, relPath string) (string, error) {
	return TaskRunArtifactsObjectKeyScope(ObjectKeyScope{Prefix: prefix, UserID: userID, ConversationID: conversationID, TaskID: taskID, TaskRunID: taskRunID}, relPath)
}

// TaskRunArtifactsObjectKeyScope returns the S3 object key for a task run artifacts dir file using scope.
func TaskRunArtifactsObjectKeyScope(scope ObjectKeyScope, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(scope.Prefix, scope.UserID, "conversations", scope.ConversationID, "tasks", scope.TaskID, scope.TaskRunID, "artifacts", clean), nil
}
