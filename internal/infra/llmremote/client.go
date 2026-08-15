// Package llmremote implements the core LLM contract against a BuildMax
// managed gateway instead of a provider.
//
// It is the client half of internal/infra/llmwire. The agent loop cannot tell
// the difference between this and internal/infra/llm: both satisfy
// core/llm.LLMClient, and the caller never learns which upstream served a call.
//
// Mirrors the design in docs/design/llm-gateway.md.
package llmremote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/infra/llmwire"
)

// ErrStreamingNotSupported is returned until the managed protocol carries
// streaming. Falling back to a blocking call would silently change where a
// caller's tokens go and how long it waits, so this fails instead.
var ErrStreamingNotSupported = errors.New("managed streaming is not implemented yet")

// maxErrorBodyBytes bounds how much of a failure response is read. A gateway
// should answer with a small JSON error; anything larger is not worth holding.
const maxErrorBodyBytes = 64 << 10

// Config holds managed client settings.
type Config struct {
	// ServerURL is the BuildMax server base URL.
	ServerURL string
	// Token authenticates the caller. It is a BuildMax credential, never a
	// provider key.
	Token string
	// TeamID scopes the call. The server verifies membership regardless.
	TeamID string
	// Alias is the team model alias to call. Empty uses the team default.
	Alias string
	// ContextWindow is the usable context size for this alias; 0 disables
	// windowing. The protocol does not report it per call, so it comes from
	// model discovery or local configuration.
	ContextWindow int
	// Surface labels where the call came from, for correlation only.
	Surface string
	// CallTimeout bounds one request; 0 means no client-side deadline, leaving
	// the server's own per-target timeout in charge.
	CallTimeout time.Duration
	// HTTPClient is optional; http.DefaultClient is used when nil.
	HTTPClient *http.Client
}

// Client calls a BuildMax managed gateway. It satisfies core/llm.LLMClient.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// NewClient builds a managed client.
func NewClient(cfg Config) *Client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	cfg.ServerURL = strings.TrimRight(cfg.ServerURL, "/")
	return &Client{cfg: cfg, httpClient: httpClient}
}

// ContextWindow returns the configured context window (0 = no windowing).
func (c *Client) ContextWindow() int {
	if c == nil {
		return 0
	}
	return c.cfg.ContextWindow
}

// ChatCompletionBlocking runs one managed call.
func (c *Client) ChatCompletionBlocking(ctx context.Context, messages []cllm.Message, tools []cllm.ToolDef) (string, []cllm.ToolCall, cllm.Usage, error) {
	if c == nil {
		return "", nil, cllm.Usage{}, errors.New("managed llm client is not configured")
	}
	if c.cfg.ServerURL == "" || c.cfg.TeamID == "" {
		return "", nil, cllm.Usage{}, errors.New("managed llm client needs a server URL and a team")
	}

	body, err := json.Marshal(llmwire.CompletionRequest{
		Model:    c.cfg.Alias,
		Messages: toWireMessages(messages),
		Tools:    toWireTools(tools),
		Metadata: c.metadata(),
	})
	if err != nil {
		return "", nil, cllm.Usage{}, fmt.Errorf("encode managed request: %w", err)
	}

	if c.cfg.CallTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.cfg.CallTimeout)
		defer cancel()
	}

	url := c.cfg.ServerURL + fmt.Sprintf(llmwire.CompletionsPath, c.cfg.TeamID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", nil, cllm.Usage{}, fmt.Errorf("build managed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", nil, cllm.Usage{}, fmt.Errorf("managed gateway unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", nil, cllm.Usage{}, gatewayError(resp)
	}

	var out llmwire.CompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", nil, cllm.Usage{}, fmt.Errorf("decode managed response: %w", err)
	}

	// An absent usage object means the provider reported none. The zero value
	// here says "unknown", the same thing the local client reports when a
	// provider omits usage.
	var usage cllm.Usage
	if out.Usage != nil {
		usage = cllm.Usage{
			PromptTokens:     out.Usage.PromptTokens,
			CompletionTokens: out.Usage.CompletionTokens,
			TotalTokens:      out.Usage.TotalTokens,
		}
	}
	return out.Content, fromWireToolCalls(out.ToolCalls), usage, nil
}

// ChatCompletionStreaming is not implemented yet. See ErrStreamingNotSupported.
func (c *Client) ChatCompletionStreaming(_ context.Context, _ []cllm.Message, _ []cllm.ToolDef, _ func(string)) (string, []cllm.ToolCall, cllm.Usage, error) {
	return "", nil, cllm.Usage{}, ErrStreamingNotSupported
}

func (c *Client) metadata() *llmwire.Metadata {
	if c.cfg.Surface == "" {
		return nil
	}
	return &llmwire.Metadata{Surface: c.cfg.Surface}
}

// GatewayError is a refusal from the managed gateway. Code is the server's
// stable classification, so callers branch on it instead of matching prose.
type GatewayError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *GatewayError) Error() string {
	switch {
	case e.Message != "" && e.Code != "":
		return fmt.Sprintf("managed gateway refused the call (%s): %s", e.Code, e.Message)
	case e.Message != "":
		return fmt.Sprintf("managed gateway refused the call (HTTP %d): %s", e.StatusCode, e.Message)
	default:
		return fmt.Sprintf("managed gateway refused the call (HTTP %d)", e.StatusCode)
	}
}

// gatewayError reads a failure response into a classified error. The body is a
// BuildMax error shape by contract; anything else is reported by status alone
// rather than echoed back to the caller.
func gatewayError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	out := &GatewayError{StatusCode: resp.StatusCode}
	if err != nil || len(body) == 0 {
		return out
	}
	var parsed llmwire.ErrorResponse
	if json.Unmarshal(body, &parsed) == nil {
		out.Code = parsed.Code
		out.Message = parsed.Error
	}
	return out
}

func toWireMessages(in []cllm.Message) []llmwire.Message {
	if len(in) == 0 {
		return nil
	}
	out := make([]llmwire.Message, 0, len(in))
	for _, m := range in {
		out = append(out, llmwire.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			ToolCalls:  toWireToolCalls(m.ToolCalls),
		})
	}
	return out
}

func toWireToolCalls(in []cllm.ToolCall) []llmwire.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]llmwire.ToolCall, 0, len(in))
	for _, tc := range in {
		out = append(out, llmwire.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}
	return out
}

func fromWireToolCalls(in []llmwire.ToolCall) []cllm.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]cllm.ToolCall, 0, len(in))
	for _, tc := range in {
		out = append(out, cllm.ToolCall{ID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
	}
	return out
}

func toWireTools(in []cllm.ToolDef) []llmwire.Tool {
	if len(in) == 0 {
		return nil
	}
	out := make([]llmwire.Tool, 0, len(in))
	for _, t := range in {
		tool := llmwire.Tool{Name: t.Name, Description: t.Description}
		if t.Parameters != nil {
			if raw, err := json.Marshal(t.Parameters); err == nil {
				tool.Parameters = raw
			}
		}
		out = append(out, tool)
	}
	return out
}

// Models lists the aliases this client's team may use.
func (c *Client) Models(ctx context.Context) ([]llmwire.Model, error) {
	if c == nil || c.cfg.ServerURL == "" || c.cfg.TeamID == "" {
		return nil, errors.New("managed llm client needs a server URL and a team")
	}
	url := c.cfg.ServerURL + fmt.Sprintf(llmwire.ModelsPath, c.cfg.TeamID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build models request: %w", err)
	}
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("managed gateway unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, gatewayError(resp)
	}
	var out llmwire.ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode models response: %w", err)
	}
	return out.Models, nil
}

// Compile-time proof that the managed client is interchangeable with the
// provider client everywhere the agent loop expects one.
var _ cllm.LLMClient = (*Client)(nil)
