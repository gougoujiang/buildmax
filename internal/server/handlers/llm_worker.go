package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/infra/llmwire"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

// workerSurface labels calls that arrive on the worker route. It is set here
// rather than read from the request: a worker states what it is doing, not who
// it is doing it for.
const workerSurface = "worker"

// workerLLMCompletionsHandler serves
// POST /api/worker/task-runs/{task_run_id}/llm/completions.
//
// It exists so a worker can use operator-approved models without holding an
// upstream provider credential. Every attribution — team, task, run — is
// derived from server state; the only thing taken from the worker is the
// prompt it wants answered.
//
// The worker token authenticates the caller as *a* worker, not as the worker
// for this run, so the run's status is what scopes it: a call is accepted only
// while the run is executing. Without that, any holder of the token could spend
// a team's quota against a run that finished weeks ago.
func (h *Handler) workerLLMCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireLLMGateway(w) {
		return
	}
	if !h.requireStore(w, h.cfg.TaskRunStore, "task runs not configured") {
		return
	}
	taskRunID, ok := pathValueRequired(w, r, "task_run_id")
	if !ok {
		return
	}
	run, task, err := h.cfg.TaskRunStore.GetTaskRunWithTask(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "worker_llm_completions", "task_run_id", taskRunID)
		return
	}
	if run == nil || task == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "task run not found")
		return
	}
	if run.Status != string(model.RunStatusRunning) {
		httputil.WriteJSONError(w, http.StatusConflict, "task run is not executing")
		return
	}

	// Unknown fields are rejected rather than ignored, matching the user route:
	// a client that thinks it set a generation parameter must not be told
	// silently that it did.
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req llmwire.CompletionRequest
	if err := decoder.Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	messages, err := toCoreMessages(req.Messages)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	cmd := llmgateway.CompleteRequest{
		TeamID:       task.TeamID,
		TaskRunID:    &run.TaskRunID,
		TaskID:       &task.TaskID,
		ClientCallID: req.CallID,
		Alias:        req.Model,
		Messages:     messages,
		Tools:        toCoreTools(req.Tools),
		Surface:      workerSurface,
	}
	if req.Metadata != nil {
		cmd.SessionID = req.Metadata.SessionID
	}

	if req.Stream {
		h.streamLLMCompletion(w, r, cmd, task.TeamID)
		return
	}

	result, err := h.cfg.LLMGateway.Complete(r.Context(), cmd)
	if err != nil {
		h.writeLLMGatewayError(w, err, "worker_llm_completions", task.TeamID)
		return
	}

	resp := llmwire.CompletionResponse{
		LLMCallID: result.LLMCallID,
		Model:     result.Alias,
		Content:   result.Content,
		ToolCalls: fromCoreToolCalls(result.ToolCalls),
	}
	if result.UsageReported {
		resp.Usage = &llmwire.Usage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		}
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}
