package worker

import (
	"net/http"

	coreartifact "github.com/gougoujiang/buildmax/internal/core/artifact"
	artifactroutes "github.com/gougoujiang/buildmax/internal/server/handlers/artifact"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// postArtifact publishes a file a run's agent chose to keep.
//
// The run token names the run, the run names the task, and the task names the
// team — a worker never says which team it is writing to, so a stolen run token
// cannot be pointed at another one. Provenance is `agent` rather than
// `task_run`: the agent decided to publish this, which is a different fact from
// the run having left files in its output directory.
func (h *Handler) postArtifact(w http.ResponseWriter, r *http.Request) {
	taskRunID := r.PathValue("task_run_id")
	if taskRunID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_run_id required")
		return
	}
	if h.cfg.Artifacts == nil || !h.cfg.Artifacts.Available() {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "artifacts not configured")
		return
	}
	if h.cfg.TaskRuns == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "task runs not configured")
		return
	}
	run, task, err := h.cfg.TaskRuns.GetTaskRunWithTask(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "post_worker_artifact", "task_run_id", taskRunID)
		return
	}
	if run == nil || task == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	if !requireRunning(w, run.Status) {
		return
	}
	if task.TeamID == "" {
		httputil.WriteJSONError(w, http.StatusConflict, "this run has no team to keep an artifact for")
		return
	}
	artifactroutes.ReceiveUpload(w, r, h.cfg.Artifacts, artifactroutes.ReceiveInput{
		TeamID:        task.TeamID,
		SourceType:    coreartifact.SourceAgent,
		SourceID:      taskRunID,
		CreatedByType: coreartifact.CreatorAgent,
	})
}
