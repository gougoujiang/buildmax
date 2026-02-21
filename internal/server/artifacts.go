package server

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"buildmax/internal/model"
	"buildmax/internal/storage/blob"
)

// ArtifactResponse is one artifact in the list response (snake_case).
type ArtifactResponse struct {
	ArtifactID       string `json:"artifact_id"`
	TaskID           string `json:"task_id"`
	WorkspaceID      string `json:"workspace_id"`
	CreatedAt        int64  `json:"created_at"`
	Seq              int    `json:"seq"`
	TaskInputSnippet string `json:"task_input_snippet"`
}

func artifactWithTaskToResponse(a model.ArtifactWithTask) ArtifactResponse {
	return ArtifactResponse{
		ArtifactID:       a.ArtifactID,
		TaskID:           a.TaskID,
		WorkspaceID:      a.WorkspaceID,
		CreatedAt:        a.CreatedAt,
		Seq:              a.Seq,
		TaskInputSnippet: a.TaskInputSnippet,
	}
}

// listWorkspaceArtifactsHandler handles GET /api/workspaces/{workspace_id}/artifacts.
// Optional query param: task_id.
func (s *Server) listWorkspaceArtifactsHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.ArtifactStore, "artifacts not configured") {
		return
	}
	var taskIDPtr *string
	if tid := r.URL.Query().Get("task_id"); tid != "" {
		taskIDPtr = &tid
	}
	list, err := s.cfg.ArtifactStore.ListArtifactsByWorkspace(r.Context(), workspaceID, taskIDPtr)
	if err != nil {
		writeInternalError(w, err, "handler", "list_artifacts", "workspace_id", workspaceID)
		return
	}
	out := make([]ArtifactResponse, len(list))
	for i := range list {
		out[i] = artifactWithTaskToResponse(list[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// ArtifactItemResponse is one item in GET .../artifacts/{id}/items (snake_case).
type ArtifactItemResponse struct {
	RelativePath string `json:"relative_path"`
}

// listArtifactItemsHandler handles GET /api/workspaces/{workspace_id}/artifacts/{artifact_id}/items.
// Returns the list of files (artifact_item rows) for that artifact.
func (s *Server) listArtifactItemsHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.ArtifactStore, "artifacts not configured") || !s.requireStore(w, s.cfg.TaskStore, "tasks not configured") {
		return
	}
	artifactID := r.PathValue("artifact_id")
	if artifactID == "" {
		writeJSONError(w, http.StatusBadRequest, "artifact_id required")
		return
	}
	artifact, err := s.cfg.ArtifactStore.GetArtifactByID(r.Context(), artifactID)
	if err != nil {
		writeInternalError(w, err, "handler", "artifact_items", "artifact_id", artifactID)
		return
	}
	if artifact == nil {
		writeJSONError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if _, ok := s.getTaskForWorkspace(w, r, workspaceID, artifact.TaskID); !ok {
		return
	}
	items, err := s.cfg.ArtifactStore.ListArtifactItems(r.Context(), artifactID)
	if err != nil {
		writeInternalError(w, err, "handler", "artifact_items", "artifact_id", artifactID)
		return
	}
	out := make([]ArtifactItemResponse, len(items))
	for i := range items {
		out[i] = ArtifactItemResponse{RelativePath: items[i].RelativePath}
	}
	writeJSON(w, http.StatusOK, out)
}

const artifactResultFilename = "result.md"

// artifactContentHandler handles GET /api/workspaces/{workspace_id}/artifacts/{artifact_id}/content.
// Optional query param path: file path relative to the artifact dir (default "result.md").
// Only paths that exist in artifact_item for this artifact or "result.md" are allowed.
// For the current single-file layout, the file on disk is result.md; item relative_path may differ.
func (s *Server) artifactContentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.ArtifactStore, "artifacts not configured") || !s.requireStore(w, s.cfg.TaskStore, "tasks not configured") || !s.requireStore(w, s.cfg.ArtifactStorage, "artifact storage not configured") {
		return
	}
	artifactID := r.PathValue("artifact_id")
	if artifactID == "" {
		writeJSONError(w, http.StatusBadRequest, "artifact_id required")
		return
	}
	artifact, err := s.cfg.ArtifactStore.GetArtifactByID(r.Context(), artifactID)
	if err != nil {
		writeInternalError(w, err, "handler", "artifact_content", "artifact_id", artifactID)
		return
	}
	if artifact == nil {
		writeJSONError(w, http.StatusNotFound, "artifact not found")
		return
	}
	task, ok := s.getTaskForWorkspace(w, r, workspaceID, artifact.TaskID)
	if !ok {
		return
	}
	pathParam := r.URL.Query().Get("path")
	if pathParam == "" {
		pathParam = artifactResultFilename
	}
	// Reject path traversal
	if strings.Contains(pathParam, "..") || filepath.Clean(pathParam) != pathParam || strings.HasPrefix(pathParam, "/") {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}
	// Allow result.md or any path that appears in artifact_item for this artifact
	allowed := pathParam == artifactResultFilename
	if !allowed {
		items, err := s.cfg.ArtifactStore.ListArtifactItems(r.Context(), artifactID)
		if err != nil {
			writeInternalError(w, err, "handler", "artifact_content", "artifact_id", artifactID)
			return
		}
		for _, it := range items {
			if it.RelativePath == pathParam {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		writeJSONError(w, http.StatusNotFound, "file not found in artifact")
		return
	}
	// Current layout: only result.md is stored; serve via ArtifactStorage (path includes task_run_id).
	data, err := s.cfg.ArtifactStorage.GetResult(r.Context(), workspaceID, task.TaskID, artifact.TaskRunID, artifactID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, blob.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "artifact content not found")
			return
		}
		writeInternalError(w, err, "handler", "artifact_content", "artifact_id", artifactID)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
