package handlers

import (
	"encoding/json"
	"github.com/gougoujiang/buildmax/internal/server/handlers/llmhttp"
	"net/http"

	"github.com/gougoujiang/buildmax/internal/infra/llmwire"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

// The managed inference wire contract lives in internal/infra/llmwire, so the
// handler and the remote client marshal one definition rather than two that can
// drift. See docs/design/llm-gateway.md section 8.

// listLLMModelsHandler serves GET /api/teams/{team_id}/llm/models.
func (h *Handler) listLLMModelsHandler(w http.ResponseWriter, r *http.Request) {
	// Team membership is checked before the gateway, so an unauthenticated
	// caller learns nothing about whether this deployment offers managed
	// inference. Every other team-scoped route authenticates first, and an
	// authorization matrix is only meaningful if they all agree.
	_, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.TeamStore, "teams not configured")
	if !ok {
		return
	}
	if !llmhttp.RequireGateway(w, h.cfg.LLMGateway) {
		return
	}
	models, err := h.cfg.LLMGateway.Models(r.Context(), teamID)
	if err != nil {
		llmhttp.WriteError(w, err, "llm_models", teamID)
		return
	}
	out := make([]llmwire.Model, 0, len(models))
	for _, m := range models {
		capabilities := make([]string, 0, len(m.Capabilities))
		for _, c := range m.Capabilities {
			capabilities = append(capabilities, string(c))
		}
		out = append(out, llmwire.Model{
			Alias:        m.Alias,
			Name:         m.Name,
			Capabilities: capabilities,
			Default:      m.Default,
		})
	}
	httputil.WriteJSON(w, http.StatusOK, llmwire.ModelsResponse{Models: out})
}

// llmCompletionsHandler serves POST /api/teams/{team_id}/llm/completions.
func (h *Handler) llmCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	// Authorize before the gateway check; see listLLMModelsHandler.
	userID, teamID, ok := h.guard().UserAndPathTeam(w, r, h.cfg.TeamStore, "teams not configured")
	if !ok {
		return
	}
	if !llmhttp.RequireGateway(w, h.cfg.LLMGateway) {
		return
	}

	// Unknown fields are rejected rather than ignored: a client that thinks it
	// set a generation parameter must not be told silently that it did.
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

	profile, err := llmhttp.CallProfile(req.CallProfile)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	cmd := llmgateway.CompleteRequest{
		TeamID:       teamID,
		UserID:       &userID,
		ClientCallID: req.CallID,
		Alias:        req.Model,
		Messages:     messages,
		Tools:        llmhttp.CoreTools(req.Tools),
		CallProfile:  profile,
	}
	if req.Metadata != nil {
		cmd.Surface = req.Metadata.Surface
		cmd.SessionID = req.Metadata.SessionID
	}

	if req.Stream {
		llmhttp.Stream(w, r, h.cfg.LLMGateway, cmd, teamID)
		return
	}

	result, err := h.cfg.LLMGateway.Complete(r.Context(), cmd)
	if err != nil {
		llmhttp.WriteError(w, err, "llm_completions", teamID)
		return
	}

	resp := llmwire.CompletionResponse{
		LLMCallID:     result.LLMCallID,
		Model:         result.Alias,
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

// streamLLMCompletion serves a streaming managed call over SSE.
//

// sseWriter emits typed server-sent events, flushing each one.
//
