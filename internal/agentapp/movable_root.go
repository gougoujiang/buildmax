package agentapp

import (
	"path/filepath"
	"sync"
)

// MovableRoot is the workspace root the runtime's tools resolve against, held
// as session state rather than captured when the runtime was assembled.
//
// It implements tool.Workspace. Every tool consults it per call, so moving the
// root moves the whole runtime at once instead of leaving each tool to
// remember where it started. The surface that moves it — entering and leaving
// a worktree — is phase 2 of docs/design/workspace-root-and-worktrees.md; this
// type is what phase 2 sets.
//
// Reads are concurrent: read-only tools run in parallel, per
// docs/design/parallel-tool-execution.md.
type MovableRoot struct {
	mu   sync.RWMutex
	root string
}

// NewMovableRoot returns a root starting at dir, which must already be
// absolute and cleaned — resolveWorkspaceRoot is the one place that decides
// what a workspace directory means.
func NewMovableRoot(dir string) *MovableRoot {
	return &MovableRoot{root: dir}
}

// Root implements tool.Workspace.
func (r *MovableRoot) Root() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.root
}

// Set moves the root. dir is cleaned but not otherwise checked: whether a
// directory may be entered at all is decided before this is called, by the
// containment rule in docs/design/workspace-root-and-worktrees.md D3.
func (r *MovableRoot) Set(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.root = filepath.Clean(dir)
}
