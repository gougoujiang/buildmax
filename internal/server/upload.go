package server

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"buildmax/internal/config"
	"buildmax/internal/util"
)

const maxUploadFiles = 10
const maxUploadDirFiles = 200

// uploadResponse is the JSON body returned on successful upload.
type uploadResponse struct {
	Uploaded []string `json:"uploaded"`
}

func (s *Server) uploadHandler(w http.ResponseWriter, r *http.Request) {
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

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	fileHeaders := r.MultipartForm.File["files"]
	if len(fileHeaders) == 0 {
		writeJSONError(w, http.StatusBadRequest, "no files provided")
		return
	}

	paths := r.MultipartForm.Value["paths"]
	dirMode := len(paths) > 0

	if dirMode {
		if len(paths) != len(fileHeaders) {
			writeJSONError(w, http.StatusBadRequest, "paths count must match files count")
			return
		}
		if len(fileHeaders) > maxUploadDirFiles {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("too many files (max %d)", maxUploadDirFiles))
			return
		}
	} else {
		if len(fileHeaders) > maxUploadFiles {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("too many files (max %d)", maxUploadFiles))
			return
		}
	}

	destDir := filepath.Join(config.WorkspacesDir(), workspaceID)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		slog.Error("upload: mkdir", "err", err, "dir", destDir)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	ws := &util.Workspace{Root: destDir}
	uploaded := make([]string, 0, len(fileHeaders))

	for i, fh := range fileHeaders {
		var relPath string
		if dirMode {
			relPath = paths[i]
			if relPath == "" {
				writeJSONError(w, http.StatusBadRequest, "empty path in paths field")
				return
			}
			absPath, err := ws.ResolvePath(relPath)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid path: "+relPath)
				return
			}
			if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
				slog.Error("upload: mkdir subdir", "err", err, "path", absPath)
				writeJSONError(w, http.StatusInternalServerError, "internal error")
				return
			}
		} else {
			relPath = filepath.Base(fh.Filename)
			if relPath == "." || relPath == "" {
				continue
			}
		}

		src, err := fh.Open()
		if err != nil {
			slog.Error("upload: open multipart file", "err", err, "name", relPath)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}

		var dstPath string
		if dirMode {
			dstPath, _ = ws.ResolvePath(relPath)
		} else {
			dstPath = filepath.Join(destDir, relPath)
		}

		dst, err := os.Create(dstPath)
		if err != nil {
			src.Close()
			slog.Error("upload: create file", "err", err, "path", dstPath)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}

		if _, err := io.Copy(dst, src); err != nil {
			dst.Close()
			src.Close()
			slog.Error("upload: copy file", "err", err, "path", dstPath)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
		dst.Close()
		src.Close()

		uploaded = append(uploaded, relPath)
	}

	writeJSON(w, http.StatusOK, uploadResponse{Uploaded: uploaded})
}
