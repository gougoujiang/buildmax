package server

import (
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"

	"buildmax/internal/config"
	"buildmax/internal/util"
)

// fileNode is the JSON shape for a directory tree node.
type fileNode struct {
	ID       string      `json:"id"`                 // relative path from workspace root; "." for root
	Name     string      `json:"name"`               // base name (or "Workspace" for root)
	Type     string      `json:"type"`               // "folder" or "file"
	Children []*fileNode `json:"children,omitempty"` // only for folders
}

// filesTreeHandler handles GET /api/workspaces/{workspace_id}/files.
// Returns the full directory tree as nested JSON.
func (s *Server) filesTreeHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r, s.cfg.JWTSecret)
	if !ok {
		return
	}
	workspaceID := r.PathValue("workspace_id")
	if workspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace_id required")
		return
	}
	owned, err := s.userOwnsWorkspace(r, userID, workspaceID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !owned {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}

	wsDir := filepath.Join(config.WorkspacesDir(), workspaceID)
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		slog.Error("files: mkdir", "err", err, "dir", wsDir)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tree, err := buildTree(wsDir, wsDir, "")
	if err != nil {
		slog.Error("files: build tree", "err", err, "dir", wsDir)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	tree.ID = "."
	tree.Name = "Workspace"

	writeJSON(w, http.StatusOK, tree)
}

// fileContentHandler handles GET /api/workspaces/{workspace_id}/files/{path...}.
// Returns the raw file content as text/plain.
func (s *Server) fileContentHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r, s.cfg.JWTSecret)
	if !ok {
		return
	}
	workspaceID := r.PathValue("workspace_id")
	if workspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace_id required")
		return
	}
	owned, err := s.userOwnsWorkspace(r, userID, workspaceID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !owned {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}

	filePath := r.PathValue("path")
	if filePath == "" {
		writeJSONError(w, http.StatusBadRequest, "file path required")
		return
	}

	ws := &util.Workspace{Root: filepath.Join(config.WorkspacesDir(), workspaceID)}
	absPath, err := ws.ResolvePath(filePath)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, http.StatusNotFound, "file not found")
			return
		}
		slog.Error("files: stat", "err", err, "path", absPath)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if info.IsDir() {
		writeJSONError(w, http.StatusNotFound, "path is a directory")
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		slog.Error("files: read", "err", err, "path", absPath)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// buildTree recursively walks a directory and returns a fileNode tree.
// dir is the current absolute directory being walked.
// relPrefix is the relative path of dir from the workspace root (empty for root itself).
// Sorts: folders first, then files, both alphabetically.
func buildTree(root, dir, relPrefix string) (*fileNode, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var dirs, files []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	children := make([]*fileNode, 0, len(dirs)+len(files))

	for _, d := range dirs {
		childRel := d.Name()
		if relPrefix != "" {
			childRel = path.Join(relPrefix, d.Name())
		}
		child, err := buildTree(root, filepath.Join(dir, d.Name()), childRel)
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}

	for _, f := range files {
		childRel := f.Name()
		if relPrefix != "" {
			childRel = path.Join(relPrefix, f.Name())
		}
		children = append(children, &fileNode{
			ID:   childRel,
			Name: f.Name(),
			Type: "file",
		})
	}

	name := filepath.Base(dir)
	if relPrefix == "" {
		name = "Workspace"
	}

	return &fileNode{
		ID:       relPrefix,
		Name:     name,
		Type:     "folder",
		Children: children,
	}, nil
}
