package blob

import (
	"errors"
	"path"
	"strings"
)

// ErrInvalidPath is returned when a path is empty, absolute, or contains traversal.
var ErrInvalidPath = errors.New("invalid path: empty, absolute, or contains traversal")

// CleanRelPath normalizes and validates a relative path. Rejects "", absolute paths, ".." (anywhere), and backslashes.
// Returns a slash-separated clean path suitable for storage keys or file paths under a root.
func CleanRelPath(p string) (string, error) {
	if p == "" {
		return "", ErrInvalidPath
	}
	if strings.Contains(p, "..") {
		return "", ErrInvalidPath
	}
	// Normalize to forward slashes and clean
	p = path.Clean(strings.ReplaceAll(p, "\\", "/"))
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
