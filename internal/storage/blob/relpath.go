package blob

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
