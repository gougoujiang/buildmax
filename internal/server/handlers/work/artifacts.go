package work

import (
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

type runOutputResponse struct {
	TaskRunID        string    `json:"task_run_id"`
	TaskID           string    `json:"task_id"`
	ConversationID   string    `json:"conversation_id"`
	UserID           string    `json:"user_id"`
	CreatedAt        time.Time `json:"created_at"`
	TaskInputSnippet string    `json:"task_input_snippet"`
}

func artifactWithTaskToResponse(a model.ArtifactWithTask) runOutputResponse {
	return runOutputResponse{
		TaskRunID:        a.ArtifactID,
		TaskID:           a.TaskID,
		ConversationID:   a.ConversationID,
		UserID:           a.UserID,
		CreatedAt:        a.CreatedAt,
		TaskInputSnippet: a.TaskInputSnippet,
	}
}

func (h *Handler) listTaskArtifactsHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.RunOutputs, "artifacts not configured")
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.Tasks, "tasks not configured") {
		return
	}
	taskID, ok := httputil.PathValue(w, r, "task_id")
	if !ok {
		return
	}
	task, _, ok := h.getTaskForTeam(w, r, teamID, taskID)
	if !ok {
		return
	}
	list, err := h.cfg.RunOutputs.ListRunOutputsByConversation(r.Context(), task.ConversationID, &task.ID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_artifacts", "task_id", taskID)
		return
	}
	out := make([]runOutputResponse, len(list))
	for i := range list {
		out[i] = artifactWithTaskToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

type runOutputItemResponse struct {
	RelativePath string `json:"relative_path"`
}

func (h *Handler) getArtifactRunAndTaskAny(w http.ResponseWriter, r *http.Request, taskRunID string) (run *model.TaskRun, task *model.Task, ok bool) {
	if !httputil.RequireStore(w, h.cfg.TaskRuns, "task runs not configured") {
		return nil, nil, false
	}
	var err error
	run, task, err = h.cfg.TaskRuns.GetTaskRunWithTask(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "artifact", "task_run_id", taskRunID)
		return nil, nil, false
	}
	if run == nil || task == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "artifact not found")
		return nil, nil, false
	}
	return run, task, true
}

func (h *Handler) getArtifactRunAndTaskForTeam(w http.ResponseWriter, r *http.Request, teamID, taskRunID string) (run *model.TaskRun, task *model.Task, ok bool) {
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
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.RunOutputs, "artifacts not configured")
	if !ok {
		return
	}
	taskRunID, ok := httputil.PathValue(w, r, "task_run_id")
	if !ok {
		return
	}
	_, _, ok = h.getArtifactRunAndTaskForTeam(w, r, teamID, taskRunID)
	if !ok {
		return
	}
	items, err := h.cfg.RunOutputs.GetTaskRunOutputFiles(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "artifact_items", "artifact_id", taskRunID)
		return
	}
	out := make([]runOutputItemResponse, len(items))
	for i := range items {
		out[i] = runOutputItemResponse{RelativePath: items[i].RelativePath}
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

const artifactResultFilename = "result.md"

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
	if !allowed && h.cfg.RunOutputs != nil {
		items, listErr := h.cfg.RunOutputs.GetTaskRunOutputFiles(r.Context(), taskRunID)
		if listErr != nil {
			httputil.WriteInternalError(w, listErr, "handler error", "handler", "artifact_content", "task_run_id", taskRunID)
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
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.RunOutputs, "artifacts not configured")
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.RunOutputs, "artifacts not configured") || !httputil.RequireStore(w, h.cfg.RunOutputStorage, "artifact storage not configured") {
		return
	}
	taskRunID, ok := httputil.PathValue(w, r, "task_run_id")
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
		data, err = h.cfg.RunOutputStorage.GetResult(r.Context(), blob.RunRef{
			CreatedBy:      task.CreatedBy,
			ConversationID: task.ConversationID,
			TaskID:         task.ID,
			TaskRunID:      taskRunID,
		})
	} else {
		data, err = h.cfg.RunOutputStorage.GetRunOutputFile(r.Context(), blob.RunObjectRef{
			CreatedBy:      task.CreatedBy,
			ConversationID: task.ConversationID,
			TaskID:         task.ID,
			TaskRunID:      taskRunID,
			RelPath:        pathParam,
		})
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, blob.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "artifact content not found")
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "artifact_content", "task_run_id", taskRunID)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
