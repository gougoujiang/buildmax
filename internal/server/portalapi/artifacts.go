package portalapi

import (
	"errors"
	"net/http"
	"os"

	"buildmax/internal/infra/db"
	blob "buildmax/internal/infra/objectstore"
	"buildmax/internal/server/httputil"
)

type ArtifactResponse struct {
	TaskRunID        string `json:"task_run_id"`
	TaskID           string `json:"task_id"`
	ConversationID   string `json:"conversation_id"`
	UserID           string `json:"user_id"`
	CreatedAt        int64  `json:"created_at"`
	TaskInputSnippet string `json:"task_input_snippet"`
}

func artifactWithTaskToResponse(a db.ArtifactWithTask) ArtifactResponse {
	return ArtifactResponse{
		TaskRunID:        a.ArtifactID,
		TaskID:           a.TaskID,
		ConversationID:   a.ConversationID,
		UserID:           a.UserID,
		CreatedAt:        a.CreatedAt,
		TaskInputSnippet: a.TaskInputSnippet,
	}
}

func (h *Handler) listTaskArtifactsHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.RunOutputLister, "artifacts not configured")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.TaskStore, "tasks not configured") {
		return
	}
	taskID, ok := pathValueRequired(w, r, "task_id")
	if !ok {
		return
	}
	task, _, ok := h.getTaskForTeam(w, r, teamID, taskID)
	if !ok {
		return
	}
	list, err := h.cfg.RunOutputLister.ListRunOutputsByConversation(r.Context(), task.ConversationID, &task.TaskID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_artifacts", "task_id", taskID)
		return
	}
	out := make([]ArtifactResponse, len(list))
	for i := range list {
		out[i] = artifactWithTaskToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

type ArtifactItemResponse struct {
	RelativePath string `json:"relative_path"`
}

func (h *Handler) getArtifactRunAndTaskAny(w http.ResponseWriter, r *http.Request, taskRunID string) (run *db.TaskRun, task *db.Task, ok bool) {
	if !h.requireStore(w, h.cfg.TaskRunStore, "task runs not configured") {
		return nil, nil, false
	}
	var err error
	run, task, err = h.cfg.TaskRunStore.GetTaskRunWithTask(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "artifact", "task_run_id", taskRunID)
		return nil, nil, false
	}
	if run == nil || task == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "artifact not found")
		return nil, nil, false
	}
	return run, task, true
}

func (h *Handler) getArtifactRunAndTaskForTeam(w http.ResponseWriter, r *http.Request, teamID, taskRunID string) (run *db.TaskRun, task *db.Task, ok bool) {
	run, task, ok = h.getArtifactRunAndTaskAny(w, r, taskRunID)
	if !ok {
		return nil, nil, false
	}
	if _, ok = h.getConversationForTeam(w, r, teamID, task.ConversationID); !ok {
		httputil.WriteJSONError(w, http.StatusNotFound, "artifact not found")
		return nil, nil, false
	}
	return run, task, true
}

func (h *Handler) listArtifactItemsHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.RunOutputLister, "artifacts not configured")
	if !ok {
		return
	}
	taskRunID, ok := pathValueRequired(w, r, "task_run_id")
	if !ok {
		return
	}
	_, _, ok = h.getArtifactRunAndTaskForTeam(w, r, teamID, taskRunID)
	if !ok {
		return
	}
	items, err := h.cfg.RunOutputLister.GetTaskRunOutputFiles(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "artifact_items", "artifact_id", taskRunID)
		return
	}
	out := make([]ArtifactItemResponse, len(items))
	for i := range items {
		out[i] = ArtifactItemResponse{RelativePath: items[i].RelativePath}
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

const artifactResultFilename = "result.md"

// resolveArtifactPath returns the validated path parameter (default result.md) and whether it is allowed for this run; writes error and returns ("", false) on failure.
func (h *Handler) resolveArtifactPath(w http.ResponseWriter, r *http.Request, taskRunID string) (pathParam string, ok bool) {
	pathParam = r.URL.Query().Get("path")
	if pathParam == "" {
		pathParam = artifactResultFilename
	}
	var err error
	pathParam, err = blob.CleanRelPath(pathParam)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid path")
		return "", false
	}
	allowed := pathParam == artifactResultFilename
	if !allowed && h.cfg.RunOutputLister != nil {
		items, listErr := h.cfg.RunOutputLister.GetTaskRunOutputFiles(r.Context(), taskRunID)
		if listErr != nil {
			httputil.WriteInternalError(w, listErr, "portal handler error", "handler", "artifact_content", "task_run_id", taskRunID)
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
		httputil.WriteJSONError(w, http.StatusNotFound, "file not found in artifact")
		return "", false
	}
	return pathParam, true
}

func (h *Handler) artifactContentHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.RunOutputLister, "artifacts not configured")
	if !ok {
		return
	}
	if !h.requireStore(w, h.cfg.RunOutputLister, "artifacts not configured") || !h.requireStore(w, h.cfg.ArtifactStorage, "artifact storage not configured") {
		return
	}
	taskRunID, ok := pathValueRequired(w, r, "task_run_id")
	if !ok {
		return
	}
	_, task, ok := h.getArtifactRunAndTaskForTeam(w, r, teamID, taskRunID)
	if !ok {
		return
	}
	pathParam, ok := h.resolveArtifactPath(w, r, taskRunID)
	if !ok {
		return
	}
	var data []byte
	var err error
	if pathParam == artifactResultFilename {
		data, err = h.cfg.ArtifactStorage.GetResult(r.Context(), blob.RunRef{
			UserID:         task.CreatedBy,
			ConversationID: task.ConversationID,
			TaskID:         task.TaskID,
			TaskRunID:      taskRunID,
		})
	} else {
		data, err = h.cfg.ArtifactStorage.GetArtifactFile(r.Context(), blob.RunObjectRef{
			UserID:         task.CreatedBy,
			ConversationID: task.ConversationID,
			TaskID:         task.TaskID,
			TaskRunID:      taskRunID,
			RelPath:        pathParam,
		})
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, blob.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "artifact content not found")
			return
		}
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "artifact_content", "task_run_id", taskRunID)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
