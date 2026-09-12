package work

import (
	"net/http"

	coretask "github.com/icloudbb/buildmax/internal/core/task"
	"github.com/icloudbb/buildmax/internal/server/httputil"
)

// runAndTaskForSpace resolves a task run and its task, and confirms the task
// belongs to spaceID. It is the shared front of the run-scoped routes (trace,
// LLM calls, provenance): a caller that can see a task run in its space can
// reach what that run recorded. It writes the response and returns ok=false on
// any failure.
func (h *Handler) runAndTaskForSpace(w http.ResponseWriter, r *http.Request, spaceID, taskRunID string) (run *coretask.Run, task *coretask.Task, ok bool) {
	run, task, ok = h.runAndTaskAny(w, r, taskRunID)
	if !ok {
		return nil, nil, false
	}
	if task.SpaceID != spaceID {
		httputil.WriteJSONError(w, http.StatusNotFound, "task run not found")
		return nil, nil, false
	}
	return run, task, true
}

func (h *Handler) runAndTaskAny(w http.ResponseWriter, r *http.Request, taskRunID string) (run *coretask.Run, task *coretask.Task, ok bool) {
	if !httputil.RequireStore(w, h.cfg.TaskRuns, "task runs not configured") {
		return nil, nil, false
	}
	var err error
	run, task, err = h.cfg.TaskRuns.GetTaskRunWithTask(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "task_run", "task_run_id", taskRunID)
		return nil, nil, false
	}
	if run == nil || task == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "task run not found")
		return nil, nil, false
	}
	return run, task, true
}
