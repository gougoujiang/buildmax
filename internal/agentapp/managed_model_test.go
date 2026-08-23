package agentapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/infra/llm"
	"github.com/gougoujiang/buildmax/internal/infra/llmremote"
)

func directEntry() config.ModelEntry {
	return config.ModelEntry{
		Model:  "openai/gpt-4o-mini",
		Name:   "Direct",
		APIURL: "https://openrouter.ai/api/v1",
		APIKey: "provider-key",
	}
}

func managedEntry() config.ModelEntry {
	return config.ModelEntry{
		Model:     "default",
		Name:      "Team Default",
		Transport: config.TransportBuildMax,
		ServerURL: "https://buildmax.example.com",
		TeamID:    "tm_example",
	}
}

func cacheFor(entries []config.ModelEntry, token ManagedTokenFunc) *LLMClientCache {
	return &LLMClientCache{
		settings:     config.Settings{Models: entries},
		managedToken: token,
		surface:      "cli",
		clients:      make(map[string]cllm.LLMClient),
	}
}

func TestBuildsADirectClient(t *testing.T) {
	cache := cacheFor([]config.ModelEntry{directEntry()}, nil)

	client, err := cache.Get("Direct")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, ok := client.(*llm.LLMClient); !ok {
		t.Errorf("direct entry produced %T", client)
	}
}

func TestBuildsAManagedClient(t *testing.T) {
	var askedFor string
	cache := cacheFor([]config.ModelEntry{managedEntry()}, func(serverURL string) (string, error) {
		askedFor = serverURL
		return "token", nil
	})

	client, err := cache.Get("Team Default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, ok := client.(*llmremote.Client); !ok {
		t.Fatalf("managed entry produced %T", client)
	}
	if askedFor != "https://buildmax.example.com" {
		t.Errorf("the token was requested for %q", askedFor)
	}
}

// TestManagedEntryNeverFallsBackToDirect is the invariant behind having two
// named transports: a managed call that cannot be authorized must fail, not
// quietly reach a provider by some other route.
func TestManagedEntryNeverFallsBackToDirect(t *testing.T) {
	tests := []struct {
		name   string
		entry  config.ModelEntry
		token  ManagedTokenFunc
		wantIn string
	}{
		{
			name:   "surface offers no managed inference",
			entry:  managedEntry(),
			token:  nil,
			wantIn: "does not support",
		},
		{
			name: "no team",
			entry: func() config.ModelEntry {
				e := managedEntry()
				e.TeamID = ""
				return e
			}(),
			token:  func(string) (string, error) { return "token", nil },
			wantIn: "team_id is required",
		},
		{
			name:   "login belongs elsewhere",
			entry:  managedEntry(),
			token:  func(string) (string, error) { return "", errors.New("logged in to another server") },
			wantIn: "another server",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := cacheFor([]config.ModelEntry{tc.entry}, tc.token)
			client, err := cache.Get(tc.entry.Name)
			if err == nil {
				t.Fatalf("a broken managed entry produced %T", client)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error %q does not mention %q", err, tc.wantIn)
			}
		})
	}
}

// TestManagedEntryCarriesNoProviderCredential records that a managed entry is
// configuration only: the credential arrives from the login state.
func TestManagedEntryCarriesNoProviderCredential(t *testing.T) {
	cfg := toModelConfig(managedEntry())
	if cfg.APIKey != "" {
		t.Errorf("a managed entry carries an api_key: %q", cfg.APIKey)
	}
	if !cfg.IsManaged() {
		t.Error("the entry did not resolve as managed")
	}
	if cfg.ProviderModel != "default" {
		t.Errorf("ProviderModel = %q, want the team alias", cfg.ProviderModel)
	}

	// A direct entry stays direct, including when transport is unset.
	if toModelConfig(directEntry()).IsManaged() {
		t.Error("a direct entry resolved as managed")
	}
	explicit := directEntry()
	explicit.Transport = config.TransportDirect
	if toModelConfig(explicit).IsManaged() {
		t.Error("an explicitly direct entry resolved as managed")
	}
}

func TestDirectEntryStillRequiresAnAPIKey(t *testing.T) {
	entry := directEntry()
	entry.APIKey = ""
	cache := cacheFor([]config.ModelEntry{entry}, nil)

	if _, err := cache.Get(entry.Name); err == nil {
		t.Error("a direct entry with no api_key built a client")
	}
}

// runScopedCacheFor builds the cache a task run gets: managed calls carry a run
// token and name a run rather than a team.
func runScopedCacheFor(entries []config.ModelEntry, taskRunID string) *LLMClientCache {
	return &LLMClientCache{
		settings:         config.Settings{Models: entries},
		managedToken:     func(string) (string, error) { return "run-token", nil },
		managedTaskRunID: taskRunID,
		surface:          "worker",
		clients:          make(map[string]cllm.LLMClient),
	}
}

// TestRunScopedManagedEntryNeedsNoTeam is the difference between a worker and
// every other surface. A person picks a team to spend against; a task run
// already belongs to one, and the server reads it from the run token — so
// requiring team_id here would ask the worker to assert an identity the server
// is supposed to derive.
func TestRunScopedManagedEntryNeedsNoTeam(t *testing.T) {
	entry := managedEntry()
	entry.TeamID = ""

	client, err := runScopedCacheFor([]config.ModelEntry{entry}, "r_1").Get(entry.Name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, ok := client.(*llmremote.Client); !ok {
		t.Fatalf("a run-scoped managed entry produced %T", client)
	}

	// The same entry on a team-scoped surface still needs one.
	if _, err := cacheFor([]config.ModelEntry{entry}, func(string) (string, error) {
		return "token", nil
	}).Get(entry.Name); err == nil {
		t.Error("a managed entry with no team built a team-scoped client")
	}
}

// TestRunScopedManagedEntryDropsATeam records that a run never calls as a team
// even when configuration supplies one. The two identities select different
// gateway routes, and llmremote refuses a client holding both — so leaving the
// team in place would turn a stale config value into a failed run.
func TestRunScopedManagedEntryDropsATeam(t *testing.T) {
	client, err := runScopedCacheFor([]config.ModelEntry{managedEntry()}, "r_1").Get("Team Default")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	remote, ok := client.(*llmremote.Client)
	if !ok {
		t.Fatalf("a run-scoped managed entry produced %T", client)
	}
	if _, err := remote.Models(context.Background()); err == nil {
		t.Error("a run-scoped client offered model discovery")
	}
}

// TestRunScopedDirectEntryIsUnaffected keeps the direct path intact: a worker
// with a provider key behaves exactly as before, run scope or not.
func TestRunScopedDirectEntryIsUnaffected(t *testing.T) {
	client, err := runScopedCacheFor([]config.ModelEntry{directEntry()}, "r_1").Get("Direct")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, ok := client.(*llm.LLMClient); !ok {
		t.Errorf("direct entry produced %T", client)
	}
}

// TestBuildsALocalClientWithoutACredential is the other half of the credential
// exemption: a local entry must build, and a hosted one with the same gap must
// still fail here rather than at the first prompt.
func TestBuildsALocalClientWithoutACredential(t *testing.T) {
	local := config.ModelEntry{
		Model:    "qwen3:8b",
		Name:     "Local",
		Provider: config.LLMProviderOllama,
		APIURL:   "http://127.0.0.1:11434",
		// Set so building the client asks the daemon nothing.
		ContextWindow: 32_000,
	}
	cache := cacheFor([]config.ModelEntry{local}, nil)
	client, err := cache.Get("Local")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got := client.(*llm.LLMClient).Provider(); got != config.LLMProviderOllama {
		t.Errorf("provider = %q, want %q", got, config.LLMProviderOllama)
	}

	hosted := config.ModelEntry{Model: "openai/gpt-4o-mini", Name: "Hosted", APIURL: "https://example.test/v1"}
	if _, err := cacheFor([]config.ModelEntry{hosted}, nil).Get("Hosted"); err == nil {
		t.Error("a hosted entry with no api_key should fail at selection")
	}
}
