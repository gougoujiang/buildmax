package worker

import (
	"encoding/json"
	"log/slog"
	"net/http"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/workerclient"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

func (h *Handler) getTaskRun(w http.ResponseWriter, r *http.Request) {
	taskRunID := r.PathValue("task_run_id")
	if taskRunID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_run_id required")
		return
	}
	if h.cfg.TaskRuns == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "task runs not configured")
		return
	}
	run, task, err := h.cfg.TaskRuns.GetTaskRunWithTask(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "get_worker_task_run", "task_run_id", taskRunID)
		return
	}
	if run == nil || task == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "run not found")
		return
	}
	// The agent's instructions are appended to the run's system prompt. Resolving them here
	// rather than at task creation means an edited definition takes effect on the next run,
	// which is what someone editing the field expects. A deleted agent still answers, because
	// a run that already names it has to finish under the identity it was started with.
	agentInstructions := ""
	var runAgent *agentdef.Agent
	if task.AgentID != nil && *task.AgentID != "" && h.cfg.Agents != nil {
		if a, aerr := h.cfg.Agents.GetAgentIncludingDeleted(r.Context(), *task.AgentID); aerr != nil {
			// A run missing its instructions is worse than a run that never had any, but
			// refusing to dispatch it is worse still. Say so and continue.
			componentLog().Warn("worker handler: agent instructions unavailable", "task_run_id", taskRunID, "agent_id", *task.AgentID, "err", aerr)
		} else if a != nil && a.TeamID == task.TeamID {
			agentInstructions = a.Instructions
			runAgent = a
			h.recordAgentRevision(r, run, a.Revision)
		}
	}

	// Resolved here, beside the agent revision, and against the run's team
	// rather than the agent's: an agent that failed the team check above is
	// treated as no agent, plugins included.
	pins, pluginRefusal := h.resolvePluginPins(r, run, task, runAgent)
	h.recordPluginPins(r, run, pins)

	httputil.WriteJSON(w, http.StatusOK, workerclient.GetTaskRunResponse{
		Run: workerclient.TaskRunRun{
			ID:     run.ID,
			TaskID: run.TaskID,
			Input:  run.Input,
			Status: run.Status,
			// The worker polls this route while it executes, so this field is
			// how a cancel reaches a run that is already under way.
			CancelRequested: run.CancelRequestedAt != nil,
			CreatedAt:       run.CreatedAt,
		},
		Task: workerclient.TaskRunTask{
			ID:                task.ID,
			ConversationID:    task.ConversationID,
			TeamID:            task.TeamID,
			UserID:            task.CreatedBy,
			SessionID:         task.SessionID,
			LastRunID:         task.LastRunID,
			AgentInstructions: agentInstructions,
		},
		Plugins:     toWirePlugins(pins),
		PluginError: pluginRefusal,
		// The server decides how the run reaches a model. A worker executes
		// model-chosen code, so it is told the transport and alias rather than
		// choosing them — and it is told nothing else about the model, because
		// endpoint, upstream identifier, and credential stay on this side.
		LLM: h.cfg.WorkerLLM,
	})
}

func (h *Handler) postStream(w http.ResponseWriter, r *http.Request) {
	taskRunID := r.PathValue("task_run_id")
	if taskRunID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_run_id required")
		return
	}
	if h.cfg.TaskRuns == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "task runs not configured")
		return
	}
	run, _, err := h.cfg.TaskRuns.GetTaskRunWithTask(r.Context(), taskRunID)
	if err != nil || run == nil {
		if err != nil {
			httputil.WriteInternalError(w, err, "worker handler error", "handler", "post_worker_stream", "task_run_id", taskRunID)
		} else {
			httputil.WriteJSONError(w, http.StatusNotFound, "run not found")
		}
		return
	}
	var req workerclient.StreamDeltaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	h.cfg.Hub.Append(run.TaskID, req.Delta)
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handlePatchRunning(w http.ResponseWriter, r *http.Request, taskRunID string, req *workerclient.PatchTaskRunRequest) bool {
	updated, err := h.cfg.TaskRuns.TransitionTaskRun(r.Context(), model.TransitionTaskRunInput{
		TaskRunID:      taskRunID,
		ExpectedStatus: model.RunStatusScheduled,
		NewStatus:      model.RunStatusRunning,
		StartedAt:      req.StartedAt,
		SessionID:      req.SessionID,
	})
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "patch_worker_task_run", "task_run_id", taskRunID)
		return false
	}
	if !updated {
		httputil.WriteJSONError(w, http.StatusConflict, "run not scheduled or already running")
		return false
	}
	return true
}

func (h *Handler) handlePatchTerminalStatus(w http.ResponseWriter, r *http.Request, taskRunID string, req *workerclient.PatchTaskRunRequest) bool {
	relativePaths := []string(nil)
	if req.Artifact != nil {
		relativePaths = req.Artifact.RelativePaths
		if len(relativePaths) == 0 && req.Artifact.RelativePath != "" {
			relativePaths = []string{req.Artifact.RelativePath}
		}
		if len(relativePaths) == 0 {
			relativePaths = []string{"result.md"}
		}
	}
	updated, err := h.cfg.TaskRuns.TransitionTaskRun(r.Context(), model.TransitionTaskRunInput{
		TaskRunID:             taskRunID,
		ExpectedStatus:        model.RunStatusRunning,
		NewStatus:             model.RunStatus(req.Status),
		StartedAt:             req.StartedAt,
		EndedAt:               req.EndedAt,
		Output:                req.Output,
		ErrorMessage:          req.ErrorMessage,
		SessionID:             req.SessionID,
		PromptTokens:          req.PromptTokens,
		CompletionTokens:      req.CompletionTokens,
		TracePath:             req.TracePath,
		ArtifactRelativePaths: relativePaths,
	})
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "patch_worker_task_run", "task_run_id", taskRunID)
		return false
	}
	if !updated {
		// A recovery loop or an earlier retry already committed the outcome. A
		// worker report is idempotent and must not rewrite that terminal state.
		return true
	}
	h.announcer().Announce(r.Context(), taskRunID, req.Status, req.Output, req.ErrorMessage)
	return true
}

func (h *Handler) patchTaskRun(w http.ResponseWriter, r *http.Request) {
	taskRunID := r.PathValue("task_run_id")
	if taskRunID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "task_run_id required")
		return
	}
	if h.cfg.TaskRuns == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "task runs not configured")
		return
	}
	var req workerclient.PatchTaskRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Status == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "status required")
		return
	}
	if req.Status == string(model.RunStatusRunning) {
		if h.handlePatchRunning(w, r, taskRunID, &req) {
			httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		}
		return
	}
	if !model.RunStatusTerminal(req.Status) {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid run status")
		return
	}
	if h.handlePatchTerminalStatus(w, r, taskRunID, &req) {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// Identity belongs in an attr, not in every message string.
func componentLog() *slog.Logger { return slog.With("component", "worker_api") }

// recordAgentRevision notes which definition this run was handed.
//
// Instructions are resolved on every poll so an edit takes effect on the next
// run; the record is written once so an edit during this one cannot rewrite what
// already ran. A failure to record is logged and dropped: a worker waiting for
// its instructions must not be held up by bookkeeping.
func (h *Handler) recordAgentRevision(r *http.Request, run *model.TaskRun, revision int) {
	if run.AgentRevision != nil || revision <= 0 || h.cfg.TaskRuns == nil {
		return
	}
	if err := h.cfg.TaskRuns.RecordTaskRunAgentRevision(r.Context(), run.ID, revision); err != nil {
		componentLog().Warn("worker handler: agent revision not recorded", "task_run_id", run.ID, "revision", revision, "err", err)
	}
}
