package work

import (
	"bytes"
	"errors"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"net/http"
	"os"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
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
	// Workspace reports what happened to this run's workspace checkpoint: whether
	// it restored a base and whether it committed a result. Read-only status the
	// run already recorded; empty fields mean the step did not apply.
	Workspace TraceWorkspace `json:"workspace"`
}

// TraceWorkspace is the run's workspace-checkpoint state for the run-details
// view. Statuses are coretask.WorkspaceRestoreStatus / WorkspaceCheckpointStatus
// values; errors are bounded operator text, present only on a failure.
type TraceWorkspace struct {
	RestoreStatus    string `json:"restore_status,omitempty"`
	RestoreError     string `json:"restore_error,omitempty"`
	CheckpointStatus string `json:"checkpoint_status,omitempty"`
	CheckpointError  string `json:"checkpoint_error,omitempty"`
}

// getTaskRunTraceHandler serves GET
// /api/spaces/{space_id}/task-runs/{task_run_id}/trace.
func (h *Handler) getTaskRunTraceHandler(w http.ResponseWriter, r *http.Request) {
	_, spaceID, ok := h.guard().UserAndPathSpace(w, r, h.cfg.TaskRuns, "task runs not configured")
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
	run, task, ok := h.runAndTaskForSpace(w, r, spaceID, taskRunID)
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
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, apierr.ErrNotFound) {
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
		Workspace:    workspaceState(run),
	})
}

// workspaceState reads the run's recorded workspace-checkpoint status for the
// read-only run-details view. It resolves the bounded error pointers to strings
// so the response carries no nulls the client must special-case.
func workspaceState(run *coretask.Run) TraceWorkspace {
	ws := TraceWorkspace{
		RestoreStatus:    run.WorkspaceRestoreStatus,
		CheckpointStatus: run.WorkspaceCheckpointStatus,
	}
	if run.WorkspaceRestoreError != nil {
		ws.RestoreError = *run.WorkspaceRestoreError
	}
	if run.WorkspaceCheckpointError != nil {
		ws.CheckpointError = *run.WorkspaceCheckpointError
	}
	return ws
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
