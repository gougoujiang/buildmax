package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

// The managed inference wire contract.
//
// These types are a versioned BuildMax protocol, not a JSON projection of the
// core Go structs and not an OpenAI-compatible surface. Provider request
// bodies, upstream URLs, credentials, and free-form generation parameters are
// not accepted: a client names a team alias and nothing else about where the
// call goes. See docs/design/llm-gateway.md section 8.

// llmMessageDTO is one chat message.
type llmMessageDTO struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []llmToolCallDTO `json:"tool_calls,omitempty"`
}

// llmToolCallDTO is a tool invocation.
type llmToolCallDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// llmToolDTO is a tool the model may call.
type llmToolDTO struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// llmMetadataDTO is correlation context. It is never authorization input: team
// and user identity come from authentication, not from here.
type llmMetadataDTO struct {
	Surface   string  `json:"surface,omitempty"`
	SessionID *string `json:"session_id,omitempty"`
}

// llmCompletionRequestDTO is the managed completion request.
type llmCompletionRequestDTO struct {
	CallID   *string         `json:"call_id,omitempty"`
	Model    string          `json:"model,omitempty"`
	Messages []llmMessageDTO `json:"messages"`
	Tools    []llmToolDTO    `json:"tools,omitempty"`
	Stream   bool            `json:"stream,omitempty"`
	Metadata *llmMetadataDTO `json:"metadata,omitempty"`
}

// llmUsageDTO is the token usage for one call.
type llmUsageDTO struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// llmCompletionResponseDTO is a finished managed call. It deliberately returns
// BuildMax shapes rather than an upstream response body.
type llmCompletionResponseDTO struct {
	LLMCallID string           `json:"llm_call_id"`
	Model     string           `json:"model"`
	Content   string           `json:"content"`
	ToolCalls []llmToolCallDTO `json:"tool_calls,omitempty"`
	// Usage is absent when the provider reported none. An absent usage is not
	// the same fact as zero tokens.
	Usage *llmUsageDTO `json:"usage,omitempty"`
}

// llmModelDTO is one alias a team may use. It exposes no endpoint, credential,
// provider type, or upstream model identifier.
type llmModelDTO struct {
	Alias        string   `json:"alias"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
	Default      bool     `json:"default"`
}

type llmModelsResponseDTO struct {
	Models []llmModelDTO `json:"models"`
}

// listLLMModelsHandler serves GET /api/teams/{team_id}/llm/models.
func (h *Handler) listLLMModelsHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireLLMGateway(w) {
		return
	}
	_, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.TeamStore, "teams not configured")
	if !ok {
		return
	}
	models, err := h.cfg.LLMGateway.Models(r.Context(), teamID)
	if err != nil {
		h.writeLLMGatewayError(w, err, "llm_models", teamID)
		return
	}
	out := make([]llmModelDTO, 0, len(models))
	for _, m := range models {
		capabilities := make([]string, 0, len(m.Capabilities))
		for _, c := range m.Capabilities {
			capabilities = append(capabilities, string(c))
		}
		out = append(out, llmModelDTO{
			Alias:        m.Alias,
			Name:         m.Name,
			Capabilities: capabilities,
			Default:      m.Default,
		})
	}
	httputil.WriteJSON(w, http.StatusOK, llmModelsResponseDTO{Models: out})
}

// llmCompletionsHandler serves POST /api/teams/{team_id}/llm/completions.
func (h *Handler) llmCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	if !h.requireLLMGateway(w) {
		return
	}
	userID, teamID, ok := h.withUserPathTeamAndStore(w, r, h.cfg.TeamStore, "teams not configured")
	if !ok {
		return
	}

	// Unknown fields are rejected rather than ignored: a client that thinks it
	// set a generation parameter must not be told silently that it did.
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req llmCompletionRequestDTO
	if err := decoder.Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Stream {
		httputil.WriteJSONError(w, http.StatusNotImplemented, "streaming is not implemented yet")
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

	result, err := h.cfg.LLMGateway.Complete(r.Context(), cmd)
	if err != nil {
		h.writeLLMGatewayError(w, err, "llm_completions", teamID)
		return
	}

	resp := llmCompletionResponseDTO{
		LLMCallID: result.LLMCallID,
		Model:     result.Alias,
		Content:   result.Content,
		ToolCalls: fromCoreToolCalls(result.ToolCalls),
	}
	if result.UsageReported {
		resp.Usage = &llmUsageDTO{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		}
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

func toCoreMessages(in []llmMessageDTO) ([]cllm.Message, error) {
	if len(in) == 0 {
		return nil, errors.New("messages required")
	}
	out := make([]cllm.Message, 0, len(in))
	for _, m := range in {
		if m.Role == "" {
			return nil, errors.New("every message needs a role")
		}
		out = append(out, cllm.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			ToolCalls:  toCoreToolCalls(m.ToolCalls),
		})
	}
	return out, nil
}

func toCoreToolCalls(in []llmToolCallDTO) []cllm.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]cllm.ToolCall, 0, len(in))
	for _, tc := range in {
		out = append(out, cllm.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}
	return out
}

func fromCoreToolCalls(in []cllm.ToolCall) []llmToolCallDTO {
	if len(in) == 0 {
		return nil
	}
	out := make([]llmToolCallDTO, 0, len(in))
	for _, tc := range in {
		out = append(out, llmToolCallDTO{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}
	return out
}

func toCoreTools(in []llmToolDTO) []cllm.ToolDef {
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

// llmErrorResponse carries the stable classification alongside the message, so
// a client can branch on a code instead of matching prose.
type llmErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
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
	httputil.WriteJSON(w, status, llmErrorResponse{Error: message, Code: class})
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
