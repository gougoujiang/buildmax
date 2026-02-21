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

// ArtifactResponse is one run output in the list response (snake_case). ID is chat_run_id.
type ArtifactResponse struct {
	ChatRunID        string `json:"chat_run_id"`
	ChatID           string `json:"chat_id"`
	WorkspaceID      string `json:"workspace_id"`
	CreatedAt        int64  `json:"created_at"`
	ChatInputSnippet string `json:"chat_input_snippet"`
}

func artifactWithChatToResponse(a model.ArtifactWithChat) ArtifactResponse {
	return ArtifactResponse{
		ChatRunID:        a.ArtifactID, // ArtifactID is chat_run_id from query
		ChatID:           a.ChatID,
		WorkspaceID:      a.WorkspaceID,
		CreatedAt:        a.CreatedAt,
		ChatInputSnippet: a.ChatInputSnippet,
	}
}

// listWorkspaceArtifactsHandler handles GET /api/workspaces/{workspace_id}/artifacts.
// Optional query param: chat_id. Response uses chat_run_id for each run output.
func (s *Server) listWorkspaceArtifactsHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.RunOutputLister, "artifacts not configured") {
		return
	}
	var chatIDPtr *string
	if cid := r.URL.Query().Get("chat_id"); cid != "" {
		chatIDPtr = &cid
	}
	list, err := s.cfg.RunOutputLister.ListRunOutputsByWorkspace(r.Context(), workspaceID, chatIDPtr)
	if err != nil {
		writeInternalError(w, err, "handler", "list_artifacts", "workspace_id", workspaceID)
		return
	}
	out := make([]ArtifactResponse, len(list))
	for i := range list {
		out[i] = artifactWithChatToResponse(list[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// ArtifactItemResponse is one item in GET .../artifacts/{id}/items (snake_case).
type ArtifactItemResponse struct {
	RelativePath string `json:"relative_path"`
}

// listArtifactItemsHandler handles GET /api/workspaces/{workspace_id}/artifacts/{chat_run_id}/items.
// Returns the list of output files for that run.
func (s *Server) listArtifactItemsHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.RunOutputLister, "artifacts not configured") || !s.requireStore(w, s.cfg.ChatRunStore, "chat runs not configured") {
		return
	}
	chatRunID := r.PathValue("chat_run_id")
	if chatRunID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_run_id required")
		return
	}
	run, chat, err := s.cfg.ChatRunStore.GetChatRunWithChat(r.Context(), chatRunID)
	if err != nil {
		writeInternalError(w, err, "handler", "artifact_items", "artifact_id", chatRunID)
		return
	}
	if run == nil || chat == nil {
		writeJSONError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if chat.WorkspaceID != workspaceID {
		writeJSONError(w, http.StatusNotFound, "artifact not found")
		return
	}
	items, err := s.cfg.RunOutputLister.GetChatRunOutputFiles(r.Context(), chatRunID)
	if err != nil {
		writeInternalError(w, err, "handler", "artifact_items", "artifact_id", chatRunID)
		return
	}
	out := make([]ArtifactItemResponse, len(items))
	for i := range items {
		out[i] = ArtifactItemResponse{RelativePath: items[i].RelativePath}
	}
	writeJSON(w, http.StatusOK, out)
}

const artifactResultFilename = "result.md"

// artifactContentHandler handles GET /api/workspaces/{workspace_id}/artifacts/{chat_run_id}/content.
// Optional query param path: file path relative to the run output (default "result.md").
func (s *Server) artifactContentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := s.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !s.requireStore(w, s.cfg.RunOutputLister, "artifacts not configured") || !s.requireStore(w, s.cfg.ChatRunStore, "chat runs not configured") || !s.requireStore(w, s.cfg.ArtifactStorage, "artifact storage not configured") {
		return
	}
	chatRunID := r.PathValue("chat_run_id")
	if chatRunID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_run_id required")
		return
	}
	run, chat, err := s.cfg.ChatRunStore.GetChatRunWithChat(r.Context(), chatRunID)
	if err != nil {
		writeInternalError(w, err, "handler", "artifact_content", "artifact_id", chatRunID)
		return
	}
	if run == nil || chat == nil {
		writeJSONError(w, http.StatusNotFound, "artifact not found")
		return
	}
	if chat.WorkspaceID != workspaceID {
		writeJSONError(w, http.StatusNotFound, "artifact not found")
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
	// Allow result.md or any path that appears in chat_run_artifact for this run
	allowed := pathParam == artifactResultFilename
	if !allowed {
		items, listErr := s.cfg.RunOutputLister.GetChatRunOutputFiles(r.Context(), chatRunID)
		if listErr != nil {
			writeInternalError(w, listErr, "handler", "artifact_content", "artifact_id", chatRunID)
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
	var data []byte
	if pathParam == artifactResultFilename {
		data, err = s.cfg.ArtifactStorage.GetResult(r.Context(), workspaceID, chat.ChatID, chatRunID)
	} else {
		data, err = s.cfg.ArtifactStorage.GetArtifactFile(r.Context(), workspaceID, chat.ChatID, chatRunID, pathParam)
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, blob.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "artifact content not found")
			return
		}
		writeInternalError(w, err, "handler", "artifact_content", "artifact_id", chatRunID)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
