package worker

import (
	"encoding/json"
	"github.com/gougoujiang/buildmax/internal/server/handlers/llmhttp"
	"net/http"

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
// upstream provider credential. Every attribution — user, team, task, run —
// comes from the run token the server minted at dispatch; the only thing taken
// from the worker is the prompt it wants answered.
//
// Server state is still consulted, but as verification rather than derivation.
// A call is accepted only while the run is executing, so a token that outlives
// its run cannot go on spending a team's quota, and the run's team must match
// the token's, so a token and a reassigned run cannot disagree silently.
func (h *Handler) workerLLMCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	taskRunID, ok := httputil.PathValue(w, r, "task_run_id")
	if !ok {
		return
	}
	claims, ok := h.requireRunToken(w, r, taskRunID)
	if !ok {
		return
	}
	if !llmhttp.RequireGateway(w, h.cfg.Gateway) {
		return
	}
	if !httputil.RequireStore(w, h.cfg.TaskRuns, "task runs not configured") {
		return
	}
	run, task, err := h.cfg.TaskRuns.GetTaskRunWithTask(r.Context(), taskRunID)
	if err != nil {
		httputil.WriteInternalError(w, err, "worker handler error", "handler", "worker_llm_completions", "task_run_id", taskRunID)
		return
	}
	if run == nil || task == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "task run not found")
		return
	}
	if !requireRunning(w, run.Status) {
		return
	}
	if task.TeamID != claims.TeamID {
		httputil.WriteJSONError(w, http.StatusForbidden, "this run token does not authorize that task run")
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
	messages, err := llmhttp.CoreMessages(req.Messages)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// The user is recorded as well as the team. A task run belongs to whoever
	// created it, and a ledger that only says "some worker" cannot answer whose
	// work spent the tokens.
	userID := claims.UserID
	profile, err := llmhttp.CallProfile(req.CallProfile)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	cmd := llmgateway.CompleteRequest{
		TeamID:       claims.TeamID,
		UserID:       &userID,
		TaskRunID:    &run.ID,
		TaskID:       &task.ID,
		ClientCallID: req.CallID,
		Model:        req.Model,
		Messages:     messages,
		Tools:        llmhttp.CoreTools(req.Tools),
		Surface:      workerSurface,
		CallProfile:  profile,
	}
	if req.Metadata != nil {
		cmd.SessionID = req.Metadata.SessionID
	}

	if req.Stream {
		llmhttp.Stream(w, r, h.cfg.Gateway, cmd, claims.TeamID)
		return
	}

	result, err := h.cfg.Gateway.Complete(r.Context(), cmd)
	if err != nil {
		llmhttp.WriteError(w, err, "worker_llm_completions", claims.TeamID)
		return
	}

	resp := llmwire.CompletionResponse{
		LLMCallID:     result.LLMCallID,
		Model:         result.Model,
		Content:       result.Content,
		ToolCalls:     llmhttp.WireToolCalls(result.ToolCalls),
		ProviderState: llmhttp.WireProviderState(result.ProviderState),
	}
	if result.UsageReported {
		resp.Usage = &llmwire.Usage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
			CacheReadTokens:  result.Usage.CacheReadTokens,
			CacheWriteTokens: result.Usage.CacheWriteTokens,
		}
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}
