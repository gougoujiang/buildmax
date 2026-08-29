package objectstore

import (
	"errors"
	"path"
	"strings"
)

// ErrInvalidPath is returned when a path is empty, absolute, or contains traversal.
var ErrInvalidPath = errors.New("invalid path: empty, absolute, or contains traversal")

// normalizeRelSegment normalizes a path segment to forward slashes and cleans it (no new package dependency).
func normalizeRelSegment(p string) string {
	return path.Clean(strings.ReplaceAll(p, "\\", "/"))
}

// CleanRelPath normalizes and validates a relative path. Rejects "", absolute paths, ".." (anywhere), and backslashes.
// Returns a slash-separated clean path suitable for storage keys or file paths under a root.
func CleanRelPath(p string) (string, error) {
	if p == "" {
		return "", ErrInvalidPath
	}
	if strings.Contains(p, "..") {
		return "", ErrInvalidPath
	}
	p = normalizeRelSegment(p)
	if p == "." {
		return "", ErrInvalidPath
	}
	if strings.HasPrefix(p, "/") {
		return "", ErrInvalidPath
	}
	if strings.HasPrefix(p, "..") {
		return "", ErrInvalidPath
	}
	return p, nil
}

// runKeyScope groups prefix and run identifiers for blob key construction.
type runKeyScope struct {
	Prefix         string
	CreatedBy      string
	ConversationID string
	TaskID         string
	TaskRunID      string
}

// PersistObjectKey returns the S3 object key for a persistent team file.
// relPath must be already validated with CleanRelPath.
func PersistObjectKey(prefix, teamID, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(prefix, teamID, "home", clean), nil
}

// RunOutputResultKey returns the S3 object key for a run's result.md (one artifact per task run).
func RunOutputResultKey(prefix, createdBy, conversationID, taskID, taskRunID string) string {
	return runOutputResultKey(runKeyScope{
		Prefix:         prefix,
		CreatedBy:      createdBy,
		ConversationID: conversationID,
		TaskID:         taskID,
		TaskRunID:      taskRunID,
	})
}

// RunOutputFileKey returns the S3 object key for one file under a run's output. relPath is validated with CleanRelPath.
func RunOutputFileKey(prefix, createdBy, conversationID, taskID, taskRunID, relPath string) (string, error) {
	return runOutputFileKey(runKeyScope{
		Prefix:         prefix,
		CreatedBy:      createdBy,
		ConversationID: conversationID,
		TaskID:         taskID,
		TaskRunID:      taskRunID,
	}, relPath)
}

func runOutputResultKey(scope runKeyScope) string {
	return path.Join(scope.Prefix, scope.CreatedBy, "artifacts", scope.ConversationID, scope.TaskID, scope.TaskRunID, "result.md")
}

func runOutputFileKey(scope runKeyScope, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(scope.Prefix, scope.CreatedBy, "artifacts", scope.ConversationID, scope.TaskID, scope.TaskRunID, clean), nil
}

// PersistPrefix returns the key prefix under which all persist files for a team live (for ListObjectsV2).
func PersistPrefix(prefix, teamID string) string {
	return path.Join(prefix, teamID, "home") + "/"
}

// RunGlobalObjectKey returns the S3 object key for a task run global dir file (BUILDMAX_HOME: logs, sessions, settings).
// relPath is validated with CleanRelPath (no .., no absolute).
func RunGlobalObjectKey(prefix, createdBy, conversationID, taskID, taskRunID, relPath string) (string, error) {
	return taskRunGlobalKey(runKeyScope{
		Prefix:         prefix,
		CreatedBy:      createdBy,
		ConversationID: conversationID,
		TaskID:         taskID,
		TaskRunID:      taskRunID,
	}, relPath)
}

func taskRunGlobalKey(scope runKeyScope, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(scope.Prefix, scope.CreatedBy, "conversations", scope.ConversationID, "tasks", scope.TaskID, scope.TaskRunID, "global", clean), nil
}

// RunArtifactsObjectKey returns the S3 object key for a task run artifacts dir file (run output files).
// relPath is validated with CleanRelPath (no .., no absolute).
func RunArtifactsObjectKey(prefix, createdBy, conversationID, taskID, taskRunID, relPath string) (string, error) {
	return taskRunArtifactsKey(runKeyScope{
		Prefix:         prefix,
		CreatedBy:      createdBy,
		ConversationID: conversationID,
		TaskID:         taskID,
		TaskRunID:      taskRunID,
	}, relPath)
}

func taskRunArtifactsKey(scope runKeyScope, relPath string) (string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return "", err
	}
	return path.Join(scope.Prefix, scope.CreatedBy, "conversations", scope.ConversationID, "tasks", scope.TaskID, scope.TaskRunID, "artifacts", clean), nil
}

// PluginPackagesPrefix is where every published package lives.
const PluginPackagesPrefix = "plugins"

// ErrInvalidDigest is returned when a digest is not the labelled SHA-256 this
// format uses.
var ErrInvalidDigest = errors.New("invalid digest: want sha256:<64 hex characters>")

// PluginPackageKey returns the object key for one release's bytes.
//
// The key is derived from the content rather than from the version. A
// version-derived key would let a second publish of an existing version
// overwrite the first release's bytes and only afterwards fail on the
// uniqueness constraint — the bytes somebody reviewed replaced by bytes nobody
// did. Content addressing also makes republishing identical bytes free, and
// leaves a failed publish with a harmless orphan rather than a corrupted
// release.
//
// The plugin name stays in the path so an operator can see what is stored, and
// so removing one plugin's bytes can never reach another's.
func PluginPackageKey(prefix, pluginName, digest string) (string, error) {
	name, err := CleanRelPath(pluginName)
	if err != nil {
		return "", err
	}
	if strings.Contains(name, "/") {
		return "", ErrInvalidPath
	}
	hex, ok := strings.CutPrefix(digest, "sha256:")
	if !ok || len(hex) != 64 || !isLowerHex(hex) {
		return "", ErrInvalidDigest
	}
	// The colon of the labelled form is not a filename anywhere useful.
	return path.Join(prefix, PluginPackagesPrefix, name, "sha256-"+hex+".tar.gz"), nil
}

func isLowerHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
