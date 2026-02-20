package blob

import "path"

// PersistObjectKey returns the S3 object key for a persistent workspace file.
// relPath must be already validated with CleanRelPath.
func PersistObjectKey(prefix, workspaceID, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(prefix, workspaceID, "persist", clean), nil
}

// ArtifactResultKey returns the S3 object key for an artifact result.md.
func ArtifactResultKey(prefix, workspaceID, taskID, runID, artifactID string) string {
	return path.Join(prefix, workspaceID, "artifacts", taskID, runID, artifactID, "result.md")
}

// PersistPrefix returns the key prefix under which all persist files for a workspace live (for ListObjectsV2).
func PersistPrefix(prefix, workspaceID string) string {
	return path.Join(prefix, workspaceID, "persist") + "/"
}

// TaskBuildmaxObjectKey returns the S3 object key for a task run buildmax file (logs, sessions, settings).
// relPath is validated with CleanRelPath (no .., no absolute).
func TaskBuildmaxObjectKey(prefix, workspaceID, taskID, runID, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(prefix, workspaceID, "tasks", taskID, runID, "buildmax", clean), nil
}
