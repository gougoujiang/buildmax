package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"

	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

type fileNode struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Children []*fileNode `json:"children,omitempty"`
}

func (h *Handler) filesTreeHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.PersistStorage, "persist storage not configured")
	if !ok {
		return
	}
	relPaths, err := h.cfg.PersistStorage.ListFiles(r.Context(), teamID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "files_tree", "team_id", teamID)
		return
	}
	tree := buildTreeFromFileList(relPaths)
	tree.ID = "."
	tree.Name = "home"
	httputil.WriteJSON(w, http.StatusOK, tree)
}

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

func (h *Handler) fileContentHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.PersistStorage, "persist storage not configured")
	if !ok {
		return
	}
	filePath := r.PathValue("path")
	if filePath == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "file path required")
		return
	}
	cleanPath, err := blob.CleanRelPath(filePath)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}
	data, err := h.cfg.PersistStorage.Get(r.Context(), teamID, cleanPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, blob.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "file not found")
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "file_content", "path", cleanPath)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

const maxUploadFiles = 10
const maxUploadDirFiles = 200

type uploadResponse struct {
	Uploaded []string `json:"uploaded"`
}

func (h *Handler) uploadHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.PersistStorage, "persist storage not configured")
	if !ok {
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	fileHeaders := r.MultipartForm.File["files"]
	if len(fileHeaders) == 0 {
		httputil.WriteJSONError(w, http.StatusBadRequest, "no files provided")
		return
	}
	paths := r.MultipartForm.Value["paths"]
	dirMode := len(paths) > 0
	if dirMode {
		if len(paths) != len(fileHeaders) {
			httputil.WriteJSONError(w, http.StatusBadRequest, "paths count must match files count")
			return
		}
		if len(fileHeaders) > maxUploadDirFiles {
			httputil.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("too many files (max %d)", maxUploadDirFiles))
			return
		}
	} else {
		if len(fileHeaders) > maxUploadFiles {
			httputil.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("too many files (max %d)", maxUploadFiles))
			return
		}
	}
	ctx := r.Context()
	uploaded := make([]string, 0, len(fileHeaders))
	for i, fh := range fileHeaders {
		var relPath string
		if dirMode {
			relPath = paths[i]
			if relPath == "" {
				httputil.WriteJSONError(w, http.StatusBadRequest, "empty path in paths field")
				return
			}
		} else {
			relPath = filepath.Base(fh.Filename)
			if relPath == "." || relPath == "" {
				continue
			}
		}
		cleanPath, err := blob.CleanRelPath(relPath)
		if err != nil {
			httputil.WriteJSONError(w, http.StatusBadRequest, "invalid path: "+relPath)
			return
		}
		src, err := fh.Open()
		if err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "upload", "name", relPath)
			return
		}
		if err := h.cfg.PersistStorage.Put(ctx, teamID, cleanPath, src); err != nil {
			src.Close()
			httputil.WriteInternalError(w, err, "handler error", "handler", "upload", "path", cleanPath)
			return
		}
		src.Close()
		uploaded = append(uploaded, cleanPath)
	}
	httputil.WriteJSON(w, http.StatusOK, uploadResponse{Uploaded: uploaded})
}
