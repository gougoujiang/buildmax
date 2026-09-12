package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/icloudbb/buildmax/internal/core/apierr"
	coretask "github.com/icloudbb/buildmax/internal/core/task"
	"github.com/icloudbb/buildmax/internal/infra/workerclient"
	"github.com/icloudbb/buildmax/internal/server/httputil"
	workspacesvc "github.com/icloudbb/buildmax/internal/service/workspace"
)

// WorkspaceRunStore reads the base checkpoint a run may restore from and records
// how that restore ended. The worker derives Space and Task from its run token,
// never the other way round, so these take only the run id.
type WorkspaceRunStore interface {
	GetRunWorkspaceBase(ctx context.Context, taskRunID string) (*coretask.WorkspaceCheckpoint, error)
	RecordWorkspaceRestore(ctx context.Context, taskRunID string, status coretask.WorkspaceRestoreStatus, errMessage *string) error
}

// getWorkspaceBase answers the descriptor of the checkpoint this run restores
// from, or 204 when the run has none (the first run of a Task, which seeds
// instead). No storage key crosses this boundary: the worker addresses the
// payload from its own space and the digest here.
func (h *Handler) getWorkspaceBase(w http.ResponseWriter, r *http.Request) {
	taskRunID := r.PathValue("task_run_id")
	if h.cfg.WorkspaceRuns == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "workspace checkpoints not configured")
		return
	}
	base, err := h.cfg.WorkspaceRuns.GetRunWorkspaceBase(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "get_workspace_base", "task_run_id", taskRunID)
		return
	}
	if base == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, workerclient.WorkspaceBaseResponse{
		CheckpointID:      base.ID,
		PayloadFormat:     base.PayloadFormat,
		PayloadSHA256:     base.PayloadSHA256,
		SizeBytes:         base.SizeBytes,
		UncompressedBytes: base.UncompressedBytes,
		EntryCount:        base.EntryCount,
	})
}

// postWorkspaceRestore records whether the run restored its base. A failed
// restore is a fact the run reports, not an error of this call.
func (h *Handler) postWorkspaceRestore(w http.ResponseWriter, r *http.Request) {
	taskRunID := r.PathValue("task_run_id")
	if h.cfg.WorkspaceRuns == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "workspace checkpoints not configured")
		return
	}
	var req workerclient.WorkspaceRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	status := coretask.WorkspaceRestoreStatus(req.Status)
	if status != coretask.WorkspaceRestoreRestored && status != coretask.WorkspaceRestoreFailed {
		httputil.WriteJSONError(w, http.StatusBadRequest, "status must be restored or failed")
		return
	}
	var errMessage *string
	if status == coretask.WorkspaceRestoreFailed && req.Error != "" {
		errMessage = &req.Error
	}
	if err := h.cfg.WorkspaceRuns.RecordWorkspaceRestore(r.Context(), taskRunID, status, errMessage); err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "run not found")
			return
		}
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "post_workspace_restore", "task_run_id", taskRunID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// postWorkspaceCheckpoint finalizes a seed the worker captured and uploaded. The
// Space and Task come from the run the token names, never from the body.
func (h *Handler) postWorkspaceCheckpoint(w http.ResponseWriter, r *http.Request) {
	taskRunID := r.PathValue("task_run_id")
	if h.cfg.Checkpoints == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "workspace checkpoints not configured")
		return
	}
	if h.cfg.TaskRuns == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "task runs not configured")
		return
	}
	run, task, err := h.cfg.TaskRuns.GetTaskRunWithTask(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "post_workspace_checkpoint", "task_run_id", taskRunID)
		return
	}
	if run == nil || task == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	if !requireRunning(w, run.Status) {
		return
	}
	var req workerclient.SeedCheckpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	cp, err := h.cfg.Checkpoints.RecordBase(r.Context(), workspacesvc.RecordBaseInput{
		SpaceID:         task.SpaceID,
		TaskID:          task.ID,
		SourceTaskRunID: taskRunID,
		Payload: workspacesvc.PayloadDescriptor{
			Format:            req.PayloadFormat,
			SHA256:            req.PayloadSHA256,
			SizeBytes:         req.SizeBytes,
			UncompressedBytes: req.UncompressedBytes,
			EntryCount:        req.EntryCount,
		},
	})
	if err != nil {
		writeCheckpointError(w, err, taskRunID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, workerclient.SeedCheckpointResponse{CheckpointID: cp.ID})
}

// writeCheckpointError maps a finalize failure to a status the worker can act
// on: a malformed descriptor or missing bytes is the worker's to fix, a
// conflicting kind is a losing race, and the rest is the server's problem.
func writeCheckpointError(w http.ResponseWriter, err error, taskRunID string) {
	switch {
	case errors.Is(err, workspacesvc.ErrInvalidDescriptor):
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid checkpoint descriptor")
	case errors.Is(err, workspacesvc.ErrPayloadMissing):
		httputil.WriteJSONError(w, http.StatusConflict, "checkpoint payload bytes are not in the store")
	case errors.Is(err, coretask.ErrCheckpointConflict):
		httputil.WriteJSONError(w, http.StatusConflict, "a different checkpoint already exists for this run and kind")
	case errors.Is(err, apierr.ErrNotFound):
		httputil.WriteJSONError(w, http.StatusNotFound, "run not found")
	default:
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "post_workspace_checkpoint", "task_run_id", taskRunID)
	}
}
