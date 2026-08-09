package util

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ResolveWorkspaceRoot resolves and absolutizes a workspace root directory.
// If dir is empty, the current working directory is used.
func ResolveWorkspaceRoot(dir string) (string, error) {
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// ResolvePath resolves a user-supplied path relative to root, ensuring the result
// stays under root. Returns the absolute, cleaned path.
// Includes a Windows-safe prefix check (filepath.Rel can return an absolute
// path when roots differ on different drives).
// Does NOT stat the path — callers handle existence and type checks.
func ResolvePath(root, userPath string) (string, error) {
	var resolved string
	if filepath.IsAbs(userPath) {
		r, err := filepath.Abs(filepath.Clean(userPath))
		if err != nil {
			return "", err
		}
		resolved = r
	} else {
		joined := filepath.Join(root, userPath)
		r, err := filepath.Abs(filepath.Clean(joined))
		if err != nil {
			return "", err
		}
		resolved = r
	}

	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", errors.New("path outside allowed root")
	}
	if rel == ".." || strings.HasPrefix(rel, "..") {
		return "", errors.New("path outside allowed root")
	}

	cleanRoot := filepath.Clean(root)
	resolvedClean := filepath.Clean(resolved)
	if resolvedClean != cleanRoot && !strings.HasPrefix(resolvedClean, cleanRoot+string(filepath.Separator)) {
		return "", errors.New("path outside allowed root")
	}

	return resolved, nil
}
