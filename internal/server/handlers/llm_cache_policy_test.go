package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/infra/llmremote"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

// profileRecordingClient captures what the gateway handed the provider.
//
// The route's job is to validate what a caller said and forward it; how a
// policy then shapes a provider request is the client's job and is tested where
// that request is built. Stubbing here keeps the two questions apart.
type profileRecordingClient struct {
	calls []cllm.CallProfile
}

func (c *profileRecordingClient) ChatCompletionBlocking(_ context.Context, req cllm.Request) (cllm.Completion, error) {
	c.calls = append(c.calls, req.Profile)
	return cllm.Completion{Content: "ok"}, nil
}

func (c *profileRecordingClient) ChatCompletionStreaming(_ context.Context, req cllm.Request, onDelta func(string)) (cllm.Completion, error) {
	c.calls = append(c.calls, req.Profile)
	onDelta("ok")
	return cllm.Completion{Content: "ok"}, nil
}

func (c *profileRecordingClient) ContextWindow() int { return 0 }

// profileServer serves the team completion route against a recording client.
func profileServer(t *testing.T, client cllm.LLMClient) *httptest.Server {
	t.Helper()
	target := llmgateway.Target{
		ID:            "mt_fast",
		Name:          "Fast",
		ProviderType:  llmgateway.ProviderAnthropic,
		Endpoint:      "https://upstream.invalid",
		CredentialRef: "ref",
		UpstreamModel: "claude-sonnet-5",
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
			Factory: func(context.Context, llmgateway.Target) (cllm.LLMClient, error) {
				return client, nil
			},
		},
		Ledger: &llmStubLedger{},
	}
	h := NewHandler(Config{JWTSecret: llmTestSecret, TeamStore: llmTestTeamStore(), LLMGateway: svc})
	mux := http.NewServeMux()
	h.Register(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// What a caller says the call is for has to survive the route, because it is
// the only thing the gateway has to decide caching with. A profile dropped here
// would silently turn every managed agent turn into an uncached one.
func TestTeamCompletionCarriesTheCallProfile(t *testing.T) {
	for _, profile := range []cllm.CallProfile{
		cllm.ProfileAgentTurn, cllm.ProfileTitle, cllm.ProfileCompaction, cllm.ProfileProbe,
	} {
		t.Run(string(profile), func(t *testing.T) {
			client := &profileRecordingClient{}
			server := profileServer(t, client)
			remote := llmremote.NewClient(llmremote.Config{
				ServerURL:   server.URL,
				Token:       testsupport.SignJWT(llmTestUser, llmTestSecret),
				TeamID:      llmTestTeam,
				CallTimeout: 10 * time.Second,
			})
			if _, err := remote.ChatCompletionBlocking(context.Background(), cllm.Request{
				Messages: []cllm.Message{{Role: "user", Content: "hi"}},
				Profile:  profile,
			}); err != nil {
				t.Fatalf("managed call: %v", err)
			}
			if len(client.calls) != 1 || client.calls[0] != profile {
				t.Errorf("provider saw %v, want one call with profile %q", client.calls, profile)
			}
		})
	}
}

// A client that predates the field sends none, and the gateway then has no
// evidence that anything will read the prefix back. It is left absent rather
// than promoted to a default: a claim nobody made must not become one.
func TestTeamCompletionAcceptsAnAbsentProfile(t *testing.T) {
	client := &profileRecordingClient{}
	server := profileServer(t, client)

	resp := postCompletion(t, server, `{"messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(client.calls) != 1 || client.calls[0] != "" {
		t.Errorf("provider saw %v, want one call with no profile", client.calls)
	}
}

// A profile this build does not know is refused rather than absorbed. A newer
// client must not believe it asked for one thing and be charged for another.
func TestTeamCompletionRefusesAnUnknownProfile(t *testing.T) {
	client := &profileRecordingClient{}
	server := profileServer(t, client)

	resp := postCompletion(t, server,
		`{"messages":[{"role":"user","content":"hi"}],"call_profile":"cache_everything"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if msg, _ := body["error"].(string); !strings.Contains(msg, "call_profile") {
		t.Errorf("error %q does not name the field that was wrong", msg)
	}
	if len(client.calls) != 0 {
		t.Error("a refused request still reached the provider")
	}
}

// The client-provided profile is operational input, not authorization input. A
// hand-crafted request cannot name a mode, a retention, or a cache key: the
// wire contract has no field for any of them, and an unknown field is refused
// rather than ignored. Without this a local client could spend the operator's
// money on retention the operator never chose.
func TestTeamCompletionRefusesACachePolicy(t *testing.T) {
	client := &profileRecordingClient{}
	server := profileServer(t, client)

	for _, body := range []string{
		`{"messages":[{"role":"user","content":"hi"}],"cache_control":{"mode":"force"}}`,
		`{"messages":[{"role":"user","content":"hi"}],"cache_mode":"force"}`,
		`{"messages":[{"role":"user","content":"hi"}],"cache_ttl":"1h"}`,
		`{"messages":[{"role":"user","content":"hi"}],"prompt_cache_key":"mine"}`,
	} {
		resp := postCompletion(t, server, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s produced status %d, want 400", body, resp.StatusCode)
		}
	}
	if len(client.calls) != 0 {
		t.Error("a refused request still reached the provider")
	}
}

func postCompletion(t *testing.T, server *httptest.Server, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost,
		server.URL+"/api/teams/"+llmTestTeam+"/llm/completions", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(llmTestUser, llmTestSecret))
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}
