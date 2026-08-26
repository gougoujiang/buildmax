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

// Workspace reports the directory a caller resolves paths against.
//
// It is consulted per call rather than captured at construction: a session's
// root moves when it enters a worktree, and a tool or sandbox profile still
// holding the launch directory would keep working on the tree the user left,
// with nothing to signal it. See docs/design/workspace-root-and-worktrees.md.
type Workspace interface {
	Root() string
}

// FixedRoot is a Workspace that never moves: a surface with no way to switch
// roots, and any caller that only needs one directory.
type FixedRoot string

// Root implements Workspace.
func (f FixedRoot) Root() string { return string(f) }

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
