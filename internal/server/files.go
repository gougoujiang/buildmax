package server

import (
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"

	"buildmax/internal/storage/blob"
)

// fileNode is the JSON shape for a directory tree node.
type fileNode struct {
	ID       string      `json:"id"`                 // relative path from workspace root; "." for root
	Name     string      `json:"name"`               // base name (or "home" for root; uploaded files live under home/)
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
	if !s.requireStore(w, s.cfg.Storage.PersistStorage, "persist storage not configured") {
		return
	}
	ctx := r.Context()
	relPaths, err := s.cfg.Storage.PersistStorage.ListFiles(ctx, workspaceID)
	if err != nil {
		writeInternalError(w, err, "handler", "files_tree", "workspace_id", workspaceID)
		return
	}
	tree := buildTreeFromFileList(relPaths)
	tree.ID = "."
	tree.Name = "home"
	writeJSON(w, http.StatusOK, tree)
}

// buildTreeFromFileList builds a fileNode tree from a flat list of relative file paths.
// Folders appear only when they contain at least one file.
func buildTreeFromFileList(relPaths []string) *fileNode {
	root := &fileNode{ID: ".", Name: "home", Type: "folder", Children: []*fileNode{}}
	seenDirs := make(map[string]*fileNode)
	seenDirs[""] = root
	sort.Strings(relPaths)
	for _, rel := range relPaths {
		if rel == "" {
			continue
		}
		rel = filepath.ToSlash(rel)
		dir := path.Dir(rel)
		name := path.Base(rel)
		var parent *fileNode
		if dir == "." {
			parent = root
		} else {
			var ok bool
			parent, ok = seenDirs[dir]
			if !ok {
				parent = ensurePath(seenDirs, root, dir)
			}
		}
		parent.Children = append(parent.Children, &fileNode{ID: rel, Name: name, Type: "file"})
	}
	sortFileNodes(root)
	return root
}

func ensurePath(seenDirs map[string]*fileNode, root *fileNode, dir string) *fileNode {
	if dir == "." || dir == "" {
		return root
	}
	if n, ok := seenDirs[dir]; ok {
		return n
	}
	parentDir := path.Dir(dir)
	parent := ensurePath(seenDirs, root, parentDir)
	name := path.Base(dir)
	node := &fileNode{ID: dir, Name: name, Type: "folder", Children: []*fileNode{}}
	parent.Children = append(parent.Children, node)
	seenDirs[dir] = node
	return node
}

func sortFileNodes(n *fileNode) {
	sort.Slice(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.Type != b.Type {
			return a.Type == "folder"
		}
		return a.Name < b.Name
	})
	for _, c := range n.Children {
		if c.Type == "folder" {
			sortFileNodes(c)
		}
	}
}

// fileContentHandler handles GET /api/workspaces/{workspace_id}/files/{path...}.
// Returns the raw file content as text/plain.
func (s *Server) fileContentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.Storage.PersistStorage, "persist storage not configured") {
		return
	}
	filePath := r.PathValue("path")
	if filePath == "" {
		writeJSONError(w, http.StatusBadRequest, "file path required")
		return
	}
	cleanPath, err := blob.CleanRelPath(filePath)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}
	data, err := s.cfg.Storage.PersistStorage.Get(r.Context(), workspaceID, cleanPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, blob.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "file not found")
			return
		}
		writeInternalError(w, err, "handler", "file_content", "path", cleanPath)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
