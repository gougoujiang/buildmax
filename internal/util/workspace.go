package util

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Workspace represents a resolved, absolute workspace root path.
// Create one at program startup via NewWorkspace and share it across tools.
type Workspace struct {
	Root string // absolute, cleaned path
}

// NewWorkspace resolves and absolutizes root.
// If root is empty, the current working directory is used.
func NewWorkspace(root string) (*Workspace, error) {
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Workspace{Root: filepath.Clean(abs)}, nil
}

// ResolvePath resolves a user-supplied path relative to the workspace root,
// ensuring the result stays under root. Returns the absolute, cleaned path.
// Includes a Windows-safe prefix check (filepath.Rel can return an absolute
// path when roots differ on different drives).
// Does NOT stat the path — callers handle existence and type checks.
func (w *Workspace) ResolvePath(userPath string) (string, error) {
	var resolved string
	if filepath.IsAbs(userPath) {
		// Absolute path: clean and use directly (boundary checked below).
		r, err := filepath.Abs(filepath.Clean(userPath))
		if err != nil {
			return "", err
		}
		resolved = r
	} else {
		// Relative path: join with workspace root.
		joined := filepath.Join(w.Root, userPath)
		r, err := filepath.Abs(filepath.Clean(joined))
		if err != nil {
			return "", err
		}
		resolved = r
	}

	// Ensure resolved path is under root (reject path traversal).
	rel, err := filepath.Rel(w.Root, resolved)
	if err != nil {
		return "", errors.New("path outside allowed root")
	}
	if rel == ".." || strings.HasPrefix(rel, "..") {
		return "", errors.New("path outside allowed root")
	}

	// On Windows, Rel can return an absolute path when roots differ;
	// ensure resolved is under root via prefix check.
	cleanRoot := filepath.Clean(w.Root)
	resolvedClean := filepath.Clean(resolved)
	if resolvedClean != cleanRoot && !strings.HasPrefix(resolvedClean, cleanRoot+string(filepath.Separator)) {
		return "", errors.New("path outside allowed root")
	}

	return resolved, nil
}
