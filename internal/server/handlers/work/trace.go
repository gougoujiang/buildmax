package work

import (
	"bytes"
	"errors"
	"net/http"
	"os"

	blob "github.com/gougoujiang/buildmax/internal/infra/objectstore"
	"github.com/gougoujiang/buildmax/internal/infra/trace"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	tools "github.com/gougoujiang/buildmax/internal/tool"
)

// TraceResponse is the run-diagnostics view of one task run's durable trace.
//
// It answers what the run used, what it touched, how long it took, what it
// cost, why it ended, and what confined it. It never carries model output,
// tool arguments, or tool results — those stay in the trace file.
type TraceResponse struct {
	TaskRunID string `json:"task_run_id"`
	trace.Summary
	// FilesChanged lists the paths the run wrote or edited, deduplicated and in
	// first-touch order. Derived from the tool calls rather than recorded
	// separately, so it is only as complete as the trace.
	FilesChanged []string `json:"files_changed,omitempty"`
}

// getTaskRunTraceHandler serves GET
// /api/teams/{team_id}/task-runs/{task_run_id}/trace.
func (h *Handler) getTaskRunTraceHandler(w http.ResponseWriter, r *http.Request) {
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.TaskRuns, "task runs not configured")
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.PersistStorage, "run storage not configured") {
		return
	}
	taskRunID, ok := httputil.PathValue(w, r, "task_run_id")
	if !ok {
		return
	}
	run, task, ok := h.getArtifactRunAndTaskForTeam(w, r, teamID, taskRunID)
	if !ok {
		return
	}
	// A run with no recorded trace is a normal outcome — it failed before an
	// agent started, tracing was off, or it predates the trace_path column.
	// Say so distinctly rather than returning an empty summary, which would
	// read as a run that did nothing.
	if run.TracePath == nil || *run.TracePath == "" {
		httputil.WriteJSONError(w, http.StatusNotFound, "no trace was recorded for this run")
		return
	}
	data, err := h.readRunGlobal(r.Context(), task, taskRunID, *run.TracePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, blob.ErrNotFound) {
			httputil.WriteJSONError(w, http.StatusNotFound, "this run's trace is no longer in storage")
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "task_run_trace", "task_run_id", taskRunID)
		return
	}
	summary, err := trace.Summarize(bytes.NewReader(data))
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "task_run_trace_parse", "task_run_id", taskRunID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, TraceResponse{
		TaskRunID:    taskRunID,
		Summary:      summary,
		FilesChanged: filesChanged(summary),
	})
}

// filesChanged picks the mutating tool calls out of a summary. Which tools
// change a file is knowledge this layer owns — internal/tool/names.go is the
// source of truth for the names, and the trace package deliberately surfaces
// every file_path it sees without judging it.
func filesChanged(s trace.Summary) []string {
	var out []string
	seen := make(map[string]bool)
	for _, t := range s.Tools {
		if t.Path == "" || t.Denied {
			continue
		}
		if t.Name != tools.ToolNameWrite && t.Name != tools.ToolNameEdit {
			continue
		}
		if seen[t.Path] {
			continue
		}
		seen[t.Path] = true
		out = append(out, t.Path)
	}
	return out
}
