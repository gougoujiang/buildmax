package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	llm "github.com/gougoujiang/buildmax/internal/infra/llm"
	"github.com/gougoujiang/buildmax/internal/infra/llmremote"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
	"github.com/gougoujiang/buildmax/internal/util"
)

// The managed contract: a call through the gateway must produce the same core
// content, tool calls, and usage as a direct call to the same provider. Both
// paths here end at one fake upstream, so a difference can only come from the
// protocol, the handler, or the remote client.
//
// This test lives in the server package because it drives the real handler with
// the real client. internal/infra may not import internal/server, and the
// architecture boundary test reads test files too.

// fakeUpstream is an OpenAI-compatible provider serving one canned response.
func fakeUpstream(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

// managedGateway starts the real handler over a catalog whose single target is
// the given upstream, and returns a managed client pointed at it.
func managedGateway(t *testing.T, upstreamURL string) *llmremote.Client {
	t.Helper()

	target := llmgateway.Target{
		ID:            "mt_fast",
		Name:          "Fast",
		ProviderType:  llmgateway.ProviderOpenAICompatible,
		Endpoint:      upstreamURL,
		CredentialRef: "ref",
		UpstreamModel: "vendor/x",
		Capabilities:  llmgateway.NewCapabilitySet(llmgateway.BaselineCapabilities()...),
		Enabled:       true,
	}
	catalog, err := llmgateway.NewStaticCatalog([]llmgateway.Target{target})
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

	svc := &llmgateway.Service{
		Router: &llmgateway.Router{
			Resolver: &llmgateway.Resolver{Catalog: catalog, Policies: policies},
			Factory: func(_ context.Context, target llmgateway.Target) (cllm.LLMClient, error) {
				return directClient(target.Endpoint), nil
			},
		},
		Ledger: &llmStubLedger{},
	}

	h := NewHandler(Config{
		JWTSecret:  llmTestSecret,
		TeamStore:  llmTestTeamStore(),
		LLMGateway: svc,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return llmremote.NewClient(llmremote.Config{
		ServerURL:   server.URL,
		Token:       util.SignJWT(llmTestUser, llmTestSecret),
		TeamID:      llmTestTeam,
		CallTimeout: 10 * time.Second,
	})
}

func directClient(upstreamURL string) cllm.LLMClient {
	return llm.NewClient(llm.Config{
		APIKey:      "upstream-key",
		BaseURL:     upstreamURL,
		Model:       "vendor/x",
		CallTimeout: 10 * time.Second,
	})
}

func TestManagedCallMatchesADirectCall(t *testing.T) {
	tests := []struct {
		name     string
		upstream string
	}{
		{
			name: "content, tool calls, and usage",
			upstream: `{
				"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"vendor/x",
				"choices":[{"index":0,"finish_reason":"tool_calls","message":{
					"role":"assistant","content":"hi there",
					"tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a\"}"}}]
				}}],
				"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}
			}`,
		},
		{
			name: "content only",
			upstream: `{
				"id":"chatcmpl-2","object":"chat.completion","created":1,"model":"vendor/x",
				"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"plain answer"}}],
				"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}
			}`,
		},
		{
			name: "provider reports no usage",
			upstream: `{
				"id":"chatcmpl-3","object":"chat.completion","created":1,"model":"vendor/x",
				"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"no usage here"}}]
			}`,
		},
		{
			name: "empty content with a tool call only",
			upstream: `{
				"id":"chatcmpl-4","object":"chat.completion","created":1,"model":"vendor/x",
				"choices":[{"index":0,"finish_reason":"tool_calls","message":{
					"role":"assistant","content":"",
					"tool_calls":[{"id":"call_9","type":"function","function":{"name":"bash","arguments":"{}"}}]
				}}],
				"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
			}`,
		},
	}

	messages := []cllm.Message{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "hello"},
	}
	tools := []cllm.ToolDef{{
		Name:        "read_file",
		Description: "reads a file",
		Parameters:  map[string]any{"type": "object"},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			upstream := fakeUpstream(t, tc.upstream)

			wantContent, wantToolCalls, wantUsage, err := directClient(upstream.URL).
				ChatCompletionBlocking(context.Background(), messages, tools)
			if err != nil {
				t.Fatalf("direct call: %v", err)
			}

			gotContent, gotToolCalls, gotUsage, err := managedGateway(t, upstream.URL).
				ChatCompletionBlocking(context.Background(), messages, tools)
			if err != nil {
				t.Fatalf("managed call: %v", err)
			}

			if gotContent != wantContent {
				t.Errorf("content: managed %q, direct %q", gotContent, wantContent)
			}
			if gotUsage != wantUsage {
				t.Errorf("usage: managed %+v, direct %+v", gotUsage, wantUsage)
			}
			if len(gotToolCalls) != len(wantToolCalls) {
				t.Fatalf("tool calls: managed %d, direct %d", len(gotToolCalls), len(wantToolCalls))
			}
			for i := range wantToolCalls {
				if gotToolCalls[i] != wantToolCalls[i] {
					t.Errorf("tool call %d: managed %+v, direct %+v", i, gotToolCalls[i], wantToolCalls[i])
				}
			}
		})
	}
}

// fakeStreamingUpstream is an OpenAI-compatible provider that streams.
func fakeStreamingUpstream(t *testing.T, chunks []string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("the test server cannot flush")
			return
		}
		for _, chunk := range chunks {
			_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	t.Cleanup(server.Close)
	return server
}

func TestManagedStreamMatchesADirectStream(t *testing.T) {
	chunks := []string{
		`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"}}]}`,
		`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"lo th"}}]}`,
		`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"ere"}}]}`,
		`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`,
	}
	upstream := fakeStreamingUpstream(t, chunks)
	messages := []cllm.Message{{Role: "user", Content: "hi"}}

	var directDeltas []string
	wantContent, _, wantUsage, err := directClient(upstream.URL).ChatCompletionStreaming(
		context.Background(), messages, nil,
		func(d string) { directDeltas = append(directDeltas, d) })
	if err != nil {
		t.Fatalf("direct stream: %v", err)
	}

	var managedDeltas []string
	gotContent, _, gotUsage, err := managedGateway(t, upstream.URL).ChatCompletionStreaming(
		context.Background(), messages, nil,
		func(d string) { managedDeltas = append(managedDeltas, d) })
	if err != nil {
		t.Fatalf("managed stream: %v", err)
	}

	if gotContent != wantContent {
		t.Errorf("content: managed %q, direct %q", gotContent, wantContent)
	}
	if gotUsage != wantUsage {
		t.Errorf("usage: managed %+v, direct %+v", gotUsage, wantUsage)
	}
	// Deltas must survive the extra hop intact, not merely add up to the same
	// text: a managed stream that batched everything would still pass a
	// content-only check while feeling nothing like a direct one.
	if len(managedDeltas) != len(directDeltas) {
		t.Fatalf("delta count: managed %d %v, direct %d %v",
			len(managedDeltas), managedDeltas, len(directDeltas), directDeltas)
	}
	for i := range directDeltas {
		if managedDeltas[i] != directDeltas[i] {
			t.Errorf("delta %d: managed %q, direct %q", i, managedDeltas[i], directDeltas[i])
		}
	}
}

// TestClientDisconnectCancelsTheUpstream is the whole point of propagating a
// context through the gateway: an abandoned call must stop costing tokens, not
// run to completion with nobody listening.
func TestClientDisconnectCancelsTheUpstream(t *testing.T) {
	upstreamCanceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte(`data: {"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"first"}}]}` + "\n\n"))
		flusher.Flush()

		// Hold the response open until the caller goes away.
		<-r.Context().Done()
		close(upstreamCanceled)
	}))
	t.Cleanup(upstream.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := managedGateway(t, upstream.URL)
	done := make(chan error, 1)
	go func() {
		_, _, _, err := client.ChatCompletionStreaming(ctx,
			[]cllm.Message{{Role: "user", Content: "hi"}}, nil,
			func(string) { cancel() })
		done <- err
	}()

	select {
	case <-upstreamCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("the upstream request was never canceled")
	}

	select {
	case err := <-done:
		if err == nil {
			t.Error("a canceled stream returned success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the managed call did not return after cancellation")
	}
}

// TestManagedClientSatisfiesTheAgentContract proves the two implementations are
// interchangeable where the agent loop expects a client, not merely similar.
func TestManagedClientSatisfiesTheAgentContract(t *testing.T) {
	upstream := fakeUpstream(t, `{
		"id":"chatcmpl-5","object":"chat.completion","created":1,"model":"vendor/x",
		"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],
		"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`)

	clients := map[string]cllm.LLMClient{
		"direct":  directClient(upstream.URL),
		"managed": managedGateway(t, upstream.URL),
	}
	for name, client := range clients {
		t.Run(name, func(t *testing.T) {
			content, _, usage, err := client.ChatCompletionBlocking(context.Background(),
				[]cllm.Message{{Role: "user", Content: "hi"}}, nil)
			if err != nil {
				t.Fatalf("ChatCompletionBlocking: %v", err)
			}
			if content != "ok" || usage.TotalTokens != 2 {
				t.Errorf("content=%q usage=%+v", content, usage)
			}
		})
	}
}

// TestManagedCallHidesTheUpstream checks that nothing about how the deployment
// reaches the provider survives the round trip to the client.
func TestManagedCallHidesTheUpstream(t *testing.T) {
	upstream := fakeUpstream(t, `{
		"id":"chatcmpl-6","object":"chat.completion","created":1,"model":"vendor/x",
		"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],
		"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
	}`)

	client := managedGateway(t, upstream.URL)
	models, err := client.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("got %d models, want 1", len(models))
	}
	// The alias is what a client learns. The catalog ID and the provider's own
	// model identifier are not.
	if models[0].Alias != "default" {
		t.Errorf("alias = %q", models[0].Alias)
	}
	if models[0].Name == "vendor/x" || models[0].Name == "mt_fast" {
		t.Errorf("listing exposed an internal identifier: %q", models[0].Name)
	}
}
