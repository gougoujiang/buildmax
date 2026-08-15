package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
	"github.com/gougoujiang/buildmax/internal/util"
)

const (
	llmTestSecret = "test-llm-secret"
	llmTestUser   = "u_llm"
	llmTestTeam   = "tm_llm"
)

// llmStubClient answers every call the same way.
type llmStubClient struct {
	content string
	usage   cllm.Usage
	err     error
}

func (c *llmStubClient) ChatCompletionBlocking(context.Context, []cllm.Message, []cllm.ToolDef) (string, []cllm.ToolCall, cllm.Usage, error) {
	if c.err != nil {
		return "", nil, cllm.Usage{}, c.err
	}
	return c.content, nil, c.usage, nil
}

func (c *llmStubClient) ChatCompletionStreaming(context.Context, []cllm.Message, []cllm.ToolDef, func(string)) (string, []cllm.ToolCall, cllm.Usage, error) {
	return "", nil, cllm.Usage{}, errors.New("not used")
}

func (c *llmStubClient) ContextWindow() int { return 0 }

// llmStubLedger accepts every write.
type llmStubLedger struct{ opened int }

func (l *llmStubLedger) OpenLLMCall(_ context.Context, call *model.LLMCall) (*model.LLMCall, error) {
	l.opened++
	stored := *call
	stored.LLMCallID = "lc_stub"
	return &stored, nil
}

func (l *llmStubLedger) CompleteLLMCall(context.Context, string, model.LLMCallOutcome) error {
	return nil
}

func (l *llmStubLedger) GetLLMCall(context.Context, string) (*model.LLMCall, error) { return nil, nil }

func (l *llmStubLedger) GetLLMCallByClientID(context.Context, string, string) (*model.LLMCall, error) {
	return nil, nil
}

// llmDenyQuota refuses every team.
type llmDenyQuota struct{}

func (llmDenyQuota) Check(context.Context, string, int, int) (bool, string) {
	return false, "quota exceeded: token limit"
}

func llmTestTeamStore() *mock.MockTeamStore {
	return &mock.MockTeamStore{
		Teams: []model.Team{
			{TeamID: llmTestTeam, Name: "LLM Team", CreatedBy: llmTestUser, CreatedAt: time.Now().Unix()},
		},
		Members: []model.TeamMember{
			{TeamID: llmTestTeam, UserID: llmTestUser, Role: model.TeamRoleOwner, CreatedAt: time.Now().Unix()},
		},
	}
}

func llmTestService(t *testing.T, client cllm.LLMClient, quota llmgateway.QuotaChecker) *llmgateway.Service {
	t.Helper()

	fast := llmgateway.Target{
		ID:            "mt_fast",
		Name:          "Fast",
		ProviderType:  llmgateway.ProviderOpenAICompatible,
		Endpoint:      "https://SECRET-ENDPOINT.internal/v1",
		CredentialRef: "SECRET-CREDENTIAL",
		UpstreamModel: "SECRET-UPSTREAM-MODEL",
		Capabilities:  llmgateway.NewCapabilitySet(llmgateway.BaselineCapabilities()...),
		Enabled:       true,
	}
	catalog, err := llmgateway.NewStaticCatalog([]llmgateway.Target{fast})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	policies, err := llmgateway.NewStaticPolicySource(llmgateway.TeamPolicy{
		DefaultAlias: "default",
		Aliases:      map[string]string{"default": "mt_fast"},
	}, catalog.IDs())
	if err != nil {
		t.Fatalf("NewStaticPolicySource: %v", err)
	}
	return &llmgateway.Service{
		Router: &llmgateway.Router{
			Resolver: &llmgateway.Resolver{Catalog: catalog, Policies: policies},
			Factory: func(context.Context, llmgateway.Target) (cllm.LLMClient, error) {
				return client, nil
			},
		},
		Ledger: &llmStubLedger{},
		Quota:  quota,
	}
}

func llmRequest(t *testing.T, method, path, body string, gateway *llmgateway.Service, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	h := NewHandler(Config{
		JWTSecret:  llmTestSecret,
		TeamStore:  llmTestTeamStore(),
		LLMGateway: gateway,
	})
	mux := http.NewServeMux()
	h.Register(mux)

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+util.SignJWT(llmTestUser, llmTestSecret))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func completionsPath() string { return "/api/teams/" + llmTestTeam + "/llm/completions" }
func modelsPath() string      { return "/api/teams/" + llmTestTeam + "/llm/models" }

const helloBody = `{"messages":[{"role":"user","content":"hello"}]}`

func TestLLMCompletionsSucceeds(t *testing.T) {
	svc := llmTestService(t, &llmStubClient{
		content: "hi there",
		usage:   cllm.Usage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
	}, nil)

	rec := llmRequest(t, http.MethodPost, completionsPath(), helloBody, svc, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	var resp struct {
		LLMCallID string `json:"llm_call_id"`
		Model     string `json:"model"`
		Content   string `json:"content"`
		Usage     *struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Content != "hi there" {
		t.Errorf("content = %q, want %q", resp.Content, "hi there")
	}
	if resp.Model != "default" {
		t.Errorf("model = %q, want the alias %q", resp.Model, "default")
	}
	if resp.LLMCallID != "lc_stub" {
		t.Errorf("llm_call_id = %q", resp.LLMCallID)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 14 {
		t.Errorf("usage = %+v, want 14 total tokens", resp.Usage)
	}
}

// TestLLMCompletionsOmitsUnreportedUsage keeps "unknown" distinguishable from
// "zero" on the wire.
func TestLLMCompletionsOmitsUnreportedUsage(t *testing.T) {
	svc := llmTestService(t, &llmStubClient{content: "hi"}, nil)

	rec := llmRequest(t, http.MethodPost, completionsPath(), helloBody, svc, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "usage") {
		t.Errorf("body carries a usage object when the provider reported none: %s", rec.Body)
	}
}

func TestLLMCompletionsErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		gateway    func(t *testing.T) *llmgateway.Service
		auth       bool
		wantStatus int
		wantCode   string
	}{
		{
			name:       "no auth",
			body:       helloBody,
			gateway:    func(t *testing.T) *llmgateway.Service { return llmTestService(t, &llmStubClient{}, nil) },
			auth:       false,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "gateway not configured",
			body:       helloBody,
			gateway:    func(*testing.T) *llmgateway.Service { return nil },
			auth:       true,
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "unknown alias",
			body:       `{"model":"reasoning","messages":[{"role":"user","content":"hi"}]}`,
			gateway:    func(t *testing.T) *llmgateway.Service { return llmTestService(t, &llmStubClient{}, nil) },
			auth:       true,
			wantStatus: http.StatusBadRequest,
			wantCode:   llmgateway.ErrorClassUnknownAlias,
		},
		{
			name: "over quota",
			body: helloBody,
			gateway: func(t *testing.T) *llmgateway.Service {
				return llmTestService(t, &llmStubClient{content: "hi"}, llmDenyQuota{})
			},
			auth:       true,
			wantStatus: http.StatusTooManyRequests,
			wantCode:   llmgateway.ErrorClassQuotaExceeded,
		},
		{
			name: "upstream failure",
			body: helloBody,
			gateway: func(t *testing.T) *llmgateway.Service {
				return llmTestService(t, &llmStubClient{err: errors.New("boom")}, nil)
			},
			auth:       true,
			wantStatus: http.StatusBadGateway,
			wantCode:   llmgateway.ErrorClassUpstream,
		},
		{
			name:       "unknown request field",
			body:       `{"messages":[{"role":"user","content":"hi"}],"temperature":0.9}`,
			gateway:    func(t *testing.T) *llmgateway.Service { return llmTestService(t, &llmStubClient{}, nil) },
			auth:       true,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "streaming not implemented",
			body:       `{"messages":[{"role":"user","content":"hi"}],"stream":true}`,
			gateway:    func(t *testing.T) *llmgateway.Service { return llmTestService(t, &llmStubClient{}, nil) },
			auth:       true,
			wantStatus: http.StatusNotImplemented,
		},
		{
			name:       "no messages",
			body:       `{"messages":[]}`,
			gateway:    func(t *testing.T) *llmgateway.Service { return llmTestService(t, &llmStubClient{}, nil) },
			auth:       true,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "message without a role",
			body:       `{"messages":[{"content":"hi"}]}`,
			gateway:    func(t *testing.T) *llmgateway.Service { return llmTestService(t, &llmStubClient{}, nil) },
			auth:       true,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := llmRequest(t, http.MethodPost, completionsPath(), tc.body, tc.gateway(t), tc.auth)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body)
			}
			if tc.wantCode == "" {
				return
			}
			var resp struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", resp.Code, tc.wantCode)
			}
		})
	}
}

// TestLLMCompletionsHidesProviderDetail is the wire-level half of the rule that
// upstream error bodies stay server-side: they can carry account identifiers,
// endpoints, and request fragments.
func TestLLMCompletionsHidesProviderDetail(t *testing.T) {
	providerErr := errors.New("401 from https://SECRET-ENDPOINT.internal/v1: key sk-SECRET for account acct_9 is revoked")
	svc := llmTestService(t, &llmStubClient{err: providerErr}, nil)

	rec := llmRequest(t, http.MethodPost, completionsPath(), helloBody, svc, true)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	for _, secret := range []string{"SECRET-ENDPOINT", "sk-SECRET", "acct_9", "401 from"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("response leaked %q: %s", secret, rec.Body)
		}
	}
}

func TestLLMCompletionsRejectsNonMembers(t *testing.T) {
	svc := llmTestService(t, &llmStubClient{content: "hi"}, nil)
	h := NewHandler(Config{
		JWTSecret:  llmTestSecret,
		TeamStore:  llmTestTeamStore(),
		LLMGateway: svc,
	})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, completionsPath(), strings.NewReader(helloBody))
	req.Header.Set("Authorization", "Bearer "+util.SignJWT("u_outsider", llmTestSecret))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("a non-member reached the gateway: %s", rec.Body)
	}
}

func TestLLMModelsListing(t *testing.T) {
	svc := llmTestService(t, &llmStubClient{}, nil)

	rec := llmRequest(t, http.MethodGet, modelsPath(), "", svc, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	var resp struct {
		Models []struct {
			Alias        string   `json:"alias"`
			Name         string   `json:"name"`
			Capabilities []string `json:"capabilities"`
			Default      bool     `json:"default"`
		} `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Models) != 1 {
		t.Fatalf("got %d models, want 1", len(resp.Models))
	}
	if resp.Models[0].Alias != "default" || !resp.Models[0].Default {
		t.Errorf("model = %+v", resp.Models[0])
	}
	if len(resp.Models[0].Capabilities) != 4 {
		t.Errorf("capabilities = %v, want the baseline four", resp.Models[0].Capabilities)
	}

	// A listing must not disclose how the deployment reaches a provider.
	for _, secret := range []string{"SECRET-ENDPOINT", "SECRET-CREDENTIAL", "SECRET-UPSTREAM-MODEL", "mt_fast"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Errorf("models listing leaked %q: %s", secret, rec.Body)
		}
	}
}

func TestLLMModelsUnconfigured(t *testing.T) {
	rec := llmRequest(t, http.MethodGet, modelsPath(), "", nil, true)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}
