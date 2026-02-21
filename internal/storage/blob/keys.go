package blob

import "path"

// PersistObjectKey returns the S3 object key for a persistent workspace file.
// relPath must be already validated with CleanRelPath.
func PersistObjectKey(prefix, workspaceID, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(prefix, workspaceID, "home", clean), nil
}

// ArtifactResultKey returns the S3 object key for a run's result.md (one artifact per chat run).
func ArtifactResultKey(prefix, workspaceID, chatID, chatRunID string) string {
	return path.Join(prefix, workspaceID, "artifacts", chatID, chatRunID, "result.md")
}

// ArtifactFileKey returns the S3 object key for one file under a run's output. relPath is validated with CleanRelPath.
func ArtifactFileKey(prefix, workspaceID, chatID, chatRunID, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(prefix, workspaceID, "artifacts", chatID, chatRunID, clean), nil
}

// PersistPrefix returns the key prefix under which all persist files for a workspace live (for ListObjectsV2).
func PersistPrefix(prefix, workspaceID string) string {
	return path.Join(prefix, workspaceID, "home") + "/"
}

// ChatBuildmaxObjectKey returns the S3 object key for a chat run buildmax file (logs, sessions, settings).
// Deprecated: use ChatRunGlobalObjectKey. Kept for backward compatibility.
func ChatBuildmaxObjectKey(prefix, workspaceID, chatID, chatRunID, relPath string) (string, error) {
	return ChatRunGlobalObjectKey(prefix, workspaceID, chatID, chatRunID, relPath)
}

// ChatRunGlobalObjectKey returns the S3 object key for a chat run global dir file (BUILDMAX_HOME: logs, sessions, settings).
// relPath is validated with CleanRelPath (no .., no absolute).
func ChatRunGlobalObjectKey(prefix, workspaceID, chatID, chatRunID, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(prefix, workspaceID, "chats", chatID, chatRunID, "global", clean), nil
}

// ChatRunArtifactsObjectKey returns the S3 object key for a chat run artifacts dir file (run output files).
// relPath is validated with CleanRelPath (no .., no absolute).
func ChatRunArtifactsObjectKey(prefix, workspaceID, chatID, chatRunID, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(prefix, workspaceID, "chats", chatID, chatRunID, "artifacts", clean), nil
}
