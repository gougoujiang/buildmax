package server

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"buildmax/internal/config"
)

const maxUploadFiles = 10

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

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	fileHeaders := r.MultipartForm.File["files"]
	if len(fileHeaders) == 0 {
		writeJSONError(w, http.StatusBadRequest, "no files provided")
		return
	}
	if len(fileHeaders) > maxUploadFiles {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("too many files (max %d)", maxUploadFiles))
		return
	}

	destDir := filepath.Join(config.WorkspacesDir(), workspaceID)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		slog.Error("upload: mkdir", "err", err, "dir", destDir)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	uploaded := make([]string, 0, len(fileHeaders))
	for _, fh := range fileHeaders {
		name := filepath.Base(fh.Filename)
		if name == "." || name == "" {
			continue
		}

		src, err := fh.Open()
		if err != nil {
			slog.Error("upload: open multipart file", "err", err, "name", name)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}

		dstPath := filepath.Join(destDir, name)
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

		uploaded = append(uploaded, name)
	}

	writeJSON(w, http.StatusOK, uploadResponse{Uploaded: uploaded})
}
