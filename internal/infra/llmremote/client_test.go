package llmremote_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/infra/llmremote"
	"github.com/gougoujiang/buildmax/internal/infra/llmwire"
)

// fakeGateway records the last request and replies with a canned response.
type fakeGateway struct {
	server *httptest.Server

	gotPath   string
	gotAuth   string
	gotMethod string
	gotBody   llmwire.CompletionRequest
	gotRaw    []byte

	status int
	body   string
}

func newFakeGateway(t *testing.T) *fakeGateway {
	t.Helper()
	g := &fakeGateway{status: http.StatusOK}
	g.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.gotPath = r.URL.Path
		g.gotAuth = r.Header.Get("Authorization")
		g.gotMethod = r.Method
		if r.Body != nil {
			raw := make([]byte, 0)
			buf := make([]byte, 4096)
			for {
				n, err := r.Body.Read(buf)
				raw = append(raw, buf[:n]...)
				if err != nil {
					break
				}
			}
			g.gotRaw = raw
			_ = json.Unmarshal(raw, &g.gotBody)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(g.status)
		_, _ = w.Write([]byte(g.body))
	}))
	t.Cleanup(g.server.Close)
	return g
}

func (g *fakeGateway) client(cfg llmremote.Config) *llmremote.Client {
	cfg.ServerURL = g.server.URL
	if cfg.TeamID == "" {
		cfg.TeamID = "tm_one"
	}
	return llmremote.NewClient(cfg)
}

func TestBlockingCallShapesTheRequest(t *testing.T) {
	gateway := newFakeGateway(t)
	gateway.body = `{"llm_call_id":"lc_1","model":"fast","content":"hi"}`

	client := gateway.client(llmremote.Config{Token: "tok", Alias: "fast", Surface: "cli"})
	_, _, _, err := client.ChatCompletionBlocking(context.Background(),
		[]cllm.Message{{Role: "user", Content: "hello"}},
		[]cllm.ToolDef{{Name: "read_file", Description: "reads", Parameters: map[string]any{"type": "object"}}},
	)
	if err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}

	if gateway.gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gateway.gotMethod)
	}
	if gateway.gotPath != "/api/teams/tm_one/llm/completions" {
		t.Errorf("path = %q", gateway.gotPath)
	}
	if gateway.gotAuth != "Bearer tok" {
		t.Errorf("authorization = %q", gateway.gotAuth)
	}
	if gateway.gotBody.Model != "fast" {
		t.Errorf("model = %q, want the alias", gateway.gotBody.Model)
	}
	if len(gateway.gotBody.Messages) != 1 || gateway.gotBody.Messages[0].Content != "hello" {
		t.Errorf("messages = %+v", gateway.gotBody.Messages)
	}
	if len(gateway.gotBody.Tools) != 1 || gateway.gotBody.Tools[0].Name != "read_file" {
		t.Errorf("tools = %+v", gateway.gotBody.Tools)
	}
	if gateway.gotBody.Metadata == nil || gateway.gotBody.Metadata.Surface != "cli" {
		t.Errorf("metadata = %+v", gateway.gotBody.Metadata)
	}
	if gateway.gotBody.Stream {
		t.Error("a blocking call asked for streaming")
	}
}

// TestRequestCarriesNoRoutingDetail is the client half of the rule that a
// managed caller names an alias and nothing about where the call goes.
func TestRequestCarriesNoRoutingDetail(t *testing.T) {
	gateway := newFakeGateway(t)
	gateway.body = `{"llm_call_id":"lc_1","model":"fast","content":"hi"}`

	client := gateway.client(llmremote.Config{Token: "tok", Alias: "fast"})
	if _, _, _, err := client.ChatCompletionBlocking(context.Background(),
		[]cllm.Message{{Role: "user", Content: "hello"}}, nil); err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}

	for _, forbidden := range []string{"api_url", "api_key", "base_url", "endpoint", "temperature", "provider"} {
		if strings.Contains(string(gateway.gotRaw), forbidden) {
			t.Errorf("request body carries %q: %s", forbidden, gateway.gotRaw)
		}
	}
}

func TestBlockingCallDecodesTheResponse(t *testing.T) {
	gateway := newFakeGateway(t)
	gateway.body = `{
		"llm_call_id":"lc_1",
		"model":"fast",
		"content":"hi there",
		"tool_calls":[{"id":"call_1","name":"read_file","arguments":"{\"path\":\"a\"}"}],
		"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}
	}`

	client := gateway.client(llmremote.Config{Token: "tok"})
	content, toolCalls, usage, err := client.ChatCompletionBlocking(context.Background(),
		[]cllm.Message{{Role: "user", Content: "hello"}}, nil)
	if err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	if content != "hi there" {
		t.Errorf("content = %q", content)
	}
	if len(toolCalls) != 1 || toolCalls[0].ID != "call_1" || toolCalls[0].Name != "read_file" {
		t.Errorf("tool calls = %+v", toolCalls)
	}
	if usage.TotalTokens != 14 || usage.PromptTokens != 10 {
		t.Errorf("usage = %+v", usage)
	}
}

// TestAbsentUsageIsZero keeps "unknown" and "zero" the same on both clients:
// the local provider client also reports a zero Usage when none is sent.
func TestAbsentUsageIsZero(t *testing.T) {
	gateway := newFakeGateway(t)
	gateway.body = `{"llm_call_id":"lc_1","model":"fast","content":"hi"}`

	client := gateway.client(llmremote.Config{Token: "tok"})
	_, _, usage, err := client.ChatCompletionBlocking(context.Background(),
		[]cllm.Message{{Role: "user"}}, nil)
	if err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	if usage != (cllm.Usage{}) {
		t.Errorf("usage = %+v, want the zero value", usage)
	}
}

func TestGatewayErrorsAreClassified(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantCode string
		wantIn   string
	}{
		{
			name:     "quota",
			status:   http.StatusTooManyRequests,
			body:     `{"error":"quota exceeded: token limit","code":"quota_exceeded"}`,
			wantCode: "quota_exceeded",
			wantIn:   "token limit",
		},
		{
			name:     "unknown alias",
			status:   http.StatusBadRequest,
			body:     `{"error":"model is not available to this team","code":"unknown_alias"}`,
			wantCode: "unknown_alias",
		},
		{
			name:     "upstream",
			status:   http.StatusBadGateway,
			body:     `{"error":"model provider unavailable","code":"upstream_error"}`,
			wantCode: "upstream_error",
		},
		{
			name:   "unauthorized with no body",
			status: http.StatusUnauthorized,
			body:   ``,
			wantIn: "401",
		},
		{
			name:   "html error page is not echoed",
			status: http.StatusBadGateway,
			body:   `<html><body>nginx upstream 10.0.0.7 refused</body></html>`,
			wantIn: "502",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gateway := newFakeGateway(t)
			gateway.status = tc.status
			gateway.body = tc.body

			client := gateway.client(llmremote.Config{Token: "tok"})
			_, _, _, err := client.ChatCompletionBlocking(context.Background(),
				[]cllm.Message{{Role: "user"}}, nil)
			if err == nil {
				t.Fatal("a failure status returned no error")
			}

			var gwErr *llmremote.GatewayError
			if !errors.As(err, &gwErr) {
				t.Fatalf("want *GatewayError, got %T: %v", err, err)
			}
			if gwErr.StatusCode != tc.status {
				t.Errorf("status = %d, want %d", gwErr.StatusCode, tc.status)
			}
			if tc.wantCode != "" && gwErr.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", gwErr.Code, tc.wantCode)
			}
			if tc.wantIn != "" && !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error %q does not mention %q", err, tc.wantIn)
			}
			// A non-BuildMax body is reported by status, never echoed: it can
			// name internal hosts.
			if tc.name == "html error page is not echoed" && strings.Contains(err.Error(), "10.0.0.7") {
				t.Errorf("error leaked the upstream body: %v", err)
			}
		})
	}
}

func TestStreamingIsRefusedNotFakedWithABlockingCall(t *testing.T) {
	gateway := newFakeGateway(t)
	gateway.body = `{"llm_call_id":"lc_1","model":"fast","content":"hi"}`

	client := gateway.client(llmremote.Config{Token: "tok"})
	_, _, _, err := client.ChatCompletionStreaming(context.Background(),
		[]cllm.Message{{Role: "user"}}, nil, func(string) {})
	if !errors.Is(err, llmremote.ErrStreamingNotSupported) {
		t.Fatalf("want ErrStreamingNotSupported, got %v", err)
	}
	if gateway.gotPath != "" {
		t.Error("streaming silently fell back to a blocking request")
	}
}

func TestClientRequiresServerAndTeam(t *testing.T) {
	client := llmremote.NewClient(llmremote.Config{Token: "tok"})
	if _, _, _, err := client.ChatCompletionBlocking(context.Background(),
		[]cllm.Message{{Role: "user"}}, nil); err == nil {
		t.Error("a client with no server URL made a call")
	}

	var nilClient *llmremote.Client
	if _, _, _, err := nilClient.ChatCompletionBlocking(context.Background(), nil, nil); err == nil {
		t.Error("a nil client made a call")
	}
	if nilClient.ContextWindow() != 0 {
		t.Error("a nil client reported a context window")
	}
}

func TestContextWindowIsConfigured(t *testing.T) {
	client := llmremote.NewClient(llmremote.Config{ContextWindow: 128000})
	if got := client.ContextWindow(); got != 128000 {
		t.Errorf("ContextWindow() = %d, want 128000", got)
	}
}

func TestModelsListing(t *testing.T) {
	gateway := newFakeGateway(t)
	gateway.body = `{"models":[{"alias":"default","name":"Fast","capabilities":["text_chat"],"default":true}]}`

	client := gateway.client(llmremote.Config{Token: "tok"})
	models, err := client.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if gateway.gotPath != "/api/teams/tm_one/llm/models" {
		t.Errorf("path = %q", gateway.gotPath)
	}
	if len(models) != 1 || models[0].Alias != "default" || !models[0].Default {
		t.Errorf("models = %+v", models)
	}
}

func TestServerURLTrailingSlashIsTolerated(t *testing.T) {
	gateway := newFakeGateway(t)
	gateway.body = `{"llm_call_id":"lc_1","content":"hi"}`

	client := llmremote.NewClient(llmremote.Config{
		ServerURL: gateway.server.URL + "/",
		TeamID:    "tm_one",
		Token:     "tok",
	})
	if _, _, _, err := client.ChatCompletionBlocking(context.Background(),
		[]cllm.Message{{Role: "user"}}, nil); err != nil {
		t.Fatalf("ChatCompletionBlocking: %v", err)
	}
	if gateway.gotPath != "/api/teams/tm_one/llm/completions" {
		t.Errorf("path = %q, want no doubled slash", gateway.gotPath)
	}
}
