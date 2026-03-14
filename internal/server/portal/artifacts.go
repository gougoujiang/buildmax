package portal

import (
	"errors"
	"net/http"
	"os"

	"buildmax/internal/model"
	"buildmax/internal/storage/blob"
)

type ArtifactResponse struct {
	ChatRunID        string `json:"chat_run_id"`
	ChatID           string `json:"chat_id"`
	WorkspaceID      string `json:"workspace_id"`
	CreatedAt        int64  `json:"created_at"`
	ChatInputSnippet string `json:"chat_input_snippet"`
}

func artifactWithChatToResponse(a model.ArtifactWithChat) ArtifactResponse {
	return ArtifactResponse{
		ChatRunID:        a.ArtifactID,
		ChatID:           a.ChatID,
		WorkspaceID:      a.WorkspaceID,
		CreatedAt:        a.CreatedAt,
		ChatInputSnippet: a.ChatInputSnippet,
	}
}

func (h *Handler) listWorkspaceArtifactsHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.RunOutputLister, "artifacts not configured") {
		return
	}
	var chatIDPtr *string
	if cid := r.URL.Query().Get("chat_id"); cid != "" {
		chatIDPtr = &cid
	}
	list, err := h.cfg.RunOutputLister.ListRunOutputsByWorkspace(r.Context(), workspaceID, chatIDPtr)
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

type ArtifactItemResponse struct {
	RelativePath string `json:"relative_path"`
}

func (h *Handler) listArtifactItemsHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.RunOutputLister, "artifacts not configured") || !h.requireStore(w, h.cfg.ChatRunStore, "chat runs not configured") {
		return
	}
	chatRunID := r.PathValue("chat_run_id")
	if chatRunID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_run_id required")
		return
	}
	run, chat, err := h.cfg.ChatRunStore.GetChatRunWithChat(r.Context(), chatRunID)
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
	items, err := h.cfg.RunOutputLister.GetChatRunOutputFiles(r.Context(), chatRunID)
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

func (h *Handler) artifactContentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.RunOutputLister, "artifacts not configured") || !h.requireStore(w, h.cfg.ChatRunStore, "chat runs not configured") || !h.requireStore(w, h.cfg.ArtifactStorage, "artifact storage not configured") {
		return
	}
	chatRunID := r.PathValue("chat_run_id")
	if chatRunID == "" {
		writeJSONError(w, http.StatusBadRequest, "chat_run_id required")
		return
	}
	run, chat, err := h.cfg.ChatRunStore.GetChatRunWithChat(r.Context(), chatRunID)
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
	pathParam, err = blob.CleanRelPath(pathParam)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}
	allowed := pathParam == artifactResultFilename
	if !allowed {
		items, listErr := h.cfg.RunOutputLister.GetChatRunOutputFiles(r.Context(), chatRunID)
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
		data, err = h.cfg.ArtifactStorage.GetResult(r.Context(), workspaceID, chat.ChatID, chatRunID)
	} else {
		data, err = h.cfg.ArtifactStorage.GetArtifactFile(r.Context(), workspaceID, chat.ChatID, chatRunID, pathParam)
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
