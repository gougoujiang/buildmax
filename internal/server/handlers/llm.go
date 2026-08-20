package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
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
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.TeamStore, "teams not configured")
	if !ok {
		return
	}
	if !h.requireLLMGateway(w) {
		return
	}
	models, err := h.cfg.LLMGateway.Models(r.Context(), teamID)
	if err != nil {
		h.writeLLMGatewayError(w, err, "llm_models", teamID)
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
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.TeamStore, "teams not configured")
	if !ok {
		return
	}
	if !h.requireLLMGateway(w) {
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
	messages, err := toCoreMessages(req.Messages)
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
		Tools:        toCoreTools(req.Tools),
	}
	if req.Metadata != nil {
		cmd.Surface = req.Metadata.Surface
		cmd.SessionID = req.Metadata.SessionID
	}

	if req.Stream {
		h.streamLLMCompletion(w, r, cmd, teamID)
		return
	}

	result, err := h.cfg.LLMGateway.Complete(r.Context(), cmd)
	if err != nil {
		h.writeLLMGatewayError(w, err, "llm_completions", teamID)
		return
	}

	resp := llmwire.CompletionResponse{
		LLMCallID:     result.LLMCallID,
		Model:         result.Alias,
		Content:       result.Content,
		ToolCalls:     fromCoreToolCalls(result.ToolCalls),
		ProviderState: fromCoreProviderState(result.ProviderState),
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

func toCoreMessages(in []llmwire.Message) ([]cllm.Message, error) {
	if len(in) == 0 {
		return nil, errors.New("messages required")
	}
	out := make([]cllm.Message, 0, len(in))
	for _, m := range in {
		if m.Role == "" {
			return nil, errors.New("every message needs a role")
		}
		out = append(out, cllm.Message{
			Role:          m.Role,
			Content:       m.Content,
			ToolCallID:    m.ToolCallID,
			ToolCalls:     toCoreToolCalls(m.ToolCalls),
			ProviderState: toCoreProviderState(m.ProviderState),
		})
	}
	return out, nil
}

// toCoreProviderState and fromCoreProviderState carry reasoning state across
// the gateway boundary. The server relays it without reading it: the content is
// the upstream's, and only the adapter that produced it may interpret it.
func toCoreProviderState(in *llmwire.ProviderState) *cllm.ProviderState {
	if in == nil {
		return nil
	}
	return &cllm.ProviderState{Protocol: in.Protocol, Data: in.Data}
}

func fromCoreProviderState(in *cllm.ProviderState) *llmwire.ProviderState {
	if in == nil {
		return nil
	}
	return &llmwire.ProviderState{Protocol: in.Protocol, Data: in.Data}
}

func toCoreToolCalls(in []llmwire.ToolCall) []cllm.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]cllm.ToolCall, 0, len(in))
	for _, tc := range in {
		out = append(out, cllm.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}
	return out
}

func fromCoreToolCalls(in []cllm.ToolCall) []llmwire.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]llmwire.ToolCall, 0, len(in))
	for _, tc := range in {
		out = append(out, llmwire.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}
	return out
}

func toCoreTools(in []llmwire.Tool) []cllm.ToolDef {
	if len(in) == 0 {
		return nil
	}
	out := make([]cllm.ToolDef, 0, len(in))
	for _, t := range in {
		def := cllm.ToolDef{Name: t.Name, Description: t.Description}
		if len(t.Parameters) > 0 {
			var params any
			if err := json.Unmarshal(t.Parameters, &params); err == nil {
				def.Parameters = params
			}
		}
		out = append(out, def)
	}
	return out
}

// streamLLMCompletion serves a streaming managed call over SSE.
//
// Response headers are written on the first event, not up front. A call refused
// before any output — an unknown alias, quota, a duplicate call ID — is still a
// plain HTTP error with a status the client can act on; only a failure after
// the stream has started becomes an error event.
func (h *Handler) streamLLMCompletion(w http.ResponseWriter, r *http.Request, cmd llmgateway.CompleteRequest, teamID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.WriteJSONError(w, http.StatusInternalServerError, "streaming is not supported by this server")
		return
	}

	stream := &sseWriter{w: w, flusher: flusher}
	result, err := h.cfg.LLMGateway.Stream(r.Context(), cmd, func(delta string) {
		if delta == "" {
			return
		}
		_ = stream.send(llmwire.EventDelta, llmwire.DeltaEvent{Content: delta})
	})

	if err != nil {
		if !stream.started {
			h.writeLLMGatewayError(w, err, "llm_completions_stream", teamID)
			return
		}
		class := llmgateway.ErrorClassFor(err)
		_, message := llmGatewayStatus(class, err)
		_ = stream.send(llmwire.EventError, llmwire.ErrorEvent{
			Code:      class,
			Error:     message,
			Retryable: llmgateway.RetryableClass(class),
		})
		return
	}

	final := llmwire.CompletionResponse{
		LLMCallID:     result.LLMCallID,
		Model:         result.Alias,
		Content:       result.Content,
		ToolCalls:     fromCoreToolCalls(result.ToolCalls),
		ProviderState: fromCoreProviderState(result.ProviderState),
	}
	if result.UsageReported {
		final.Usage = &llmwire.Usage{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		}
	}
	_ = stream.send(llmwire.EventResult, final)
}

// sseWriter emits typed server-sent events, flushing each one.
//
// It holds no buffer of its own: a delta reaches the network as soon as it
// arrives, which is what makes a managed stream feel like a direct one.
type sseWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	started bool
}

func (s *sseWriter) send(event string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if !s.started {
		s.started = true
		header := s.w.Header()
		header.Set("Content-Type", "text/event-stream")
		header.Set("Cache-Control", "no-cache")
		header.Set("Connection", "keep-alive")
		// Reverse proxies in supported deployments must not buffer this
		// response; nginx honours the header, others need configuration.
		header.Set("X-Accel-Buffering", "no")
		s.w.WriteHeader(http.StatusOK)
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, body); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// requireLLMGateway reports whether managed inference is configured.
func (h *Handler) requireLLMGateway(w http.ResponseWriter) bool {
	if h.cfg.LLMGateway == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "llm gateway not configured")
		return false
	}
	return true
}

// writeLLMGatewayError maps a stable class onto an HTTP status. Provider error
// bodies never reach this path — only BuildMax classifications do.
func (h *Handler) writeLLMGatewayError(w http.ResponseWriter, err error, handlerName, teamID string) {
	class := llmgateway.ErrorClassFor(err)
	status, message := llmGatewayStatus(class, err)
	if status == http.StatusInternalServerError {
		httputil.WriteInternalError(w, err, "handler error", "handler", handlerName, "team_id", teamID, "code", class)
		return
	}
	httputil.WriteJSON(w, status, llmwire.ErrorResponse{Error: message, Code: class})
}

func llmGatewayStatus(class string, err error) (int, string) {
	switch class {
	case llmgateway.ErrorClassTeamRequired, llmgateway.ErrorClassInvalidRequest:
		return http.StatusBadRequest, err.Error()
	case llmgateway.ErrorClassTeamNotAuthorized:
		return http.StatusForbidden, "no model is available to this team"
	case llmgateway.ErrorClassUnknownAlias, llmgateway.ErrorClassTargetNotFound:
		return http.StatusBadRequest, "model is not available to this team"
	case llmgateway.ErrorClassTargetDisabled:
		return http.StatusBadRequest, "model is disabled"
	case llmgateway.ErrorClassCapability:
		return http.StatusBadRequest, err.Error()
	case llmgateway.ErrorClassQuotaExceeded:
		return http.StatusTooManyRequests, err.Error()
	case llmgateway.ErrorClassDuplicateCall:
		// The caller does not know whether its first attempt landed. Saying
		// "already used" answers that; it is not an invitation to retry.
		return http.StatusConflict, err.Error()
	case llmgateway.ErrorClassNotConfigured:
		return http.StatusServiceUnavailable, "llm gateway not configured"
	case llmgateway.ErrorClassCanceled:
		return http.StatusRequestTimeout, "call canceled"
	case llmgateway.ErrorClassUpstream:
		// The provider's own message stays server-side; it can carry account
		// identifiers, endpoints, and request fragments.
		return http.StatusBadGateway, "model provider unavailable"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}
