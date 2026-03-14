package portal

import (
	"errors"
	"net/http"
	"os"

	"buildmax/internal/storage/blob"
	"buildmax/internal/storage/entity"
)

type ArtifactResponse struct {
	ChatRunID        string `json:"chat_run_id"`
	ChatID           string `json:"chat_id"`
	WorkspaceID      string `json:"workspace_id"`
	CreatedAt        int64  `json:"created_at"`
	ChatInputSnippet string `json:"chat_input_snippet"`
}

func artifactWithChatToResponse(a entity.ArtifactWithChat) ArtifactResponse {
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

// getArtifactRunAndChat loads run and chat for chatRunID and verifies workspace; writes error and returns (nil, nil, false) on failure.
func (h *Handler) getArtifactRunAndChat(w http.ResponseWriter, r *http.Request, workspaceID, chatRunID string) (run *entity.ChatRun, chat *entity.Chat, ok bool) {
	if !h.requireStore(w, h.cfg.ChatRunStore, "chat runs not configured") {
		return nil, nil, false
	}
	var err error
	run, chat, err = h.cfg.ChatRunStore.GetChatRunWithChat(r.Context(), chatRunID)
	if err != nil {
		writeInternalError(w, err, "handler", "artifact", "chat_run_id", chatRunID)
		return nil, nil, false
	}
	if run == nil || chat == nil {
		writeJSONError(w, http.StatusNotFound, "artifact not found")
		return nil, nil, false
	}
	if chat.WorkspaceID != workspaceID {
		writeJSONError(w, http.StatusNotFound, "artifact not found")
		return nil, nil, false
	}
	return run, chat, true
}

func (h *Handler) listArtifactItemsHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.RunOutputLister, "artifacts not configured") {
		return
	}
	chatRunID, ok := pathValueRequired(w, r, "chat_run_id")
	if !ok {
		return
	}
	_, _, ok = h.getArtifactRunAndChat(w, r, workspaceID, chatRunID)
	if !ok {
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

// resolveArtifactPath returns the validated path parameter (default result.md) and whether it is allowed for this run; writes error and returns ("", false) on failure.
func (h *Handler) resolveArtifactPath(w http.ResponseWriter, r *http.Request, chatRunID string) (pathParam string, ok bool) {
	pathParam = r.URL.Query().Get("path")
	if pathParam == "" {
		pathParam = artifactResultFilename
	}
	var err error
	pathParam, err = blob.CleanRelPath(pathParam)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return "", false
	}
	allowed := pathParam == artifactResultFilename
	if !allowed && h.cfg.RunOutputLister != nil {
		items, listErr := h.cfg.RunOutputLister.GetChatRunOutputFiles(r.Context(), chatRunID)
		if listErr != nil {
			writeInternalError(w, listErr, "handler", "artifact_content", "chat_run_id", chatRunID)
			return "", false
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
		return "", false
	}
	return pathParam, true
}

func (h *Handler) artifactContentHandler(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.withWorkspaceAuth(w, r, "workspace_id")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.RunOutputLister, "artifacts not configured") || !h.requireStore(w, h.cfg.ArtifactStorage, "artifact storage not configured") {
		return
	}
	chatRunID, ok := pathValueRequired(w, r, "chat_run_id")
	if !ok {
		return
	}
	_, chat, ok := h.getArtifactRunAndChat(w, r, workspaceID, chatRunID)
	if !ok {
		return
	}
	pathParam, ok := h.resolveArtifactPath(w, r, chatRunID)
	if !ok {
		return
	}
	var data []byte
	var err error
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
		writeInternalError(w, err, "handler", "artifact_content", "chat_run_id", chatRunID)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
