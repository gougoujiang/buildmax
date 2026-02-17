package server

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"

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
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	wsDir := s.persistentWorkspaceDir(workspaceID)
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		writeInternalError(w, err, "handler", "files_tree", "dir", wsDir)
		return
	}
	tree, err := buildTree(wsDir, wsDir, "")
	if err != nil {
		writeInternalError(w, err, "handler", "files_tree", "dir", wsDir)
		return
	}
	tree.ID = "."
	tree.Name = "Workspace"

	writeJSON(w, http.StatusOK, tree)
}

// fileContentHandler handles GET /api/workspaces/{workspace_id}/files/{path...}.
// Returns the raw file content as text/plain.
func (s *Server) fileContentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	filePath := r.PathValue("path")
	if filePath == "" {
		writeJSONError(w, http.StatusBadRequest, "file path required")
		return
	}
	ws := &util.Workspace{Root: s.persistentWorkspaceDir(workspaceID)}
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
		writeInternalError(w, err, "handler", "file_content", "path", absPath)
		return
	}
	if info.IsDir() {
		writeJSONError(w, http.StatusNotFound, "path is a directory")
		return
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		writeInternalError(w, err, "handler", "file_content", "path", absPath)
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
