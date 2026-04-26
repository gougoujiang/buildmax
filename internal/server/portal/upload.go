package portal

import (
	"fmt"
	"net/http"
	"path/filepath"

	"buildmax/internal/server/httputil"
	"buildmax/internal/storage/blob"
)

const maxUploadFiles = 10
const maxUploadDirFiles = 200

type uploadResponse struct {
	Uploaded []string `json:"uploaded"`
}

func (h *Handler) uploadHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.PersistStorage, "persist storage not configured")
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
			httputil.WriteInternalError(w, err, "portal handler error", "handler", "upload", "name", relPath)
			return
		}
		if err := h.cfg.PersistStorage.Put(ctx, teamID, cleanPath, src); err != nil {
			src.Close()
			httputil.WriteInternalError(w, err, "portal handler error", "handler", "upload", "path", cleanPath)
			return
		}
		src.Close()
		uploaded = append(uploaded, cleanPath)
	}

	httputil.WriteJSON(w, http.StatusOK, uploadResponse{Uploaded: uploaded})
}
