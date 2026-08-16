package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/model"
	httpserver "github.com/gougoujiang/buildmax/internal/server"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

func conversationModel() config.ServerModelEntry {
	return config.ServerModelEntry{
		Model:         "openai/gpt-4o",
		Name:          "GPT-4o",
		APIURL:        "https://openrouter.ai/api/v1",
		APIKey:        "conversation-key",
		ContextWindow: 128000,
		CallTimeout:   300,
	}
}

// fakeModels is an in-memory llm_model table.
type fakeModels struct {
	rows        map[string]model.LLMModel
	credentials map[string]string
	err         error
}

func newFakeModels(rows ...model.LLMModel) *fakeModels {
	m := &fakeModels{rows: map[string]model.LLMModel{}, credentials: map[string]string{}}
	for _, r := range rows {
		m.rows[r.LLMModelID] = r
		m.credentials[r.LLMModelID] = "key-for-" + r.LLMModelID
	}
	return m
}

func (m *fakeModels) CreateLLMModel(context.Context, model.CreateLLMModelInput) (*model.LLMModel, error) {
	return nil, errors.New("not used")
}

func (m *fakeModels) GetLLMModel(_ context.Context, id string) (*model.LLMModel, error) {
	if m.err != nil {
		return nil, m.err
	}
	row, ok := m.rows[id]
	if !ok {
		return nil, nil
	}
	return &row, nil
}

func (m *fakeModels) ListLLMModels(context.Context) ([]model.LLMModel, error) {
	out := make([]model.LLMModel, 0, len(m.rows))
	for _, r := range m.rows {
		out = append(out, r)
	}
	return out, nil
}

func (m *fakeModels) SetLLMModelEnabled(context.Context, string, bool) error { return nil }

func (m *fakeModels) LLMModelCredential(_ context.Context, id string) (string, error) {
	key, ok := m.credentials[id]
	if !ok {
		return "", errors.New("model not found")
	}
	return key, nil
}

func catalogRow(id string) model.LLMModel {
	return model.LLMModel{
		LLMModelID:    id,
		Name:          strings.ToUpper(id),
		ProviderType:  llmgateway.ProviderOpenAICompatible,
		APIURL:        "https://upstream.example.com/v1",
		Model:         "vendor/" + id,
		ContextWindow: 64000,
		CallTimeout:   120,
		Capabilities:  []string{"text_chat", "tool_calls", "streaming_text", "usage_reporting"},
		Enabled:       true,
	}
}

// TestBuildLLMRoutingWithoutConfiguration is the regression guard for a
// deployment that has neither a database nor a conversation model.
func TestBuildLLMRoutingWithoutConfiguration(t *testing.T) {
	routing, err := buildLLMRouting(config.ServerConfig{}, nil)
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}
	if routing != nil {
		t.Fatalf("routing = %+v, want nil", routing)
	}
}

// TestBuildLLMRoutingDerivesConversationTarget covers the deployment that only
// configures conversation.model, which is every server built before this
// feature and every fresh one whose catalog is still empty.
func TestBuildLLMRoutingDerivesConversationTarget(t *testing.T) {
	sc := config.ServerConfig{Conversation: config.ServerConvConfig{Model: conversationModel()}}

	routing, err := buildLLMRouting(sc, newFakeModels())
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}
	if routing.Tier1TargetID != conversationTargetID {
		t.Errorf("Tier1TargetID = %q, want %q", routing.Tier1TargetID, conversationTargetID)
	}

	routed, err := routing.Router.ClientForTarget(context.Background(), routing.Tier1TargetID, llmgateway.BaselineCapabilities())
	if err != nil {
		t.Fatalf("ClientForTarget: %v", err)
	}
	if routed.Client == nil {
		t.Fatal("no client for the conversation target")
	}
	if got := routed.Resolution.Target.UpstreamModel; got != "openai/gpt-4o" {
		t.Errorf("UpstreamModel = %q, want %q", got, "openai/gpt-4o")
	}
	if routed.Resolution.Alias != "" {
		t.Errorf("Alias = %q, want empty for a deployment-owned target", routed.Resolution.Alias)
	}
}

// TestBuildLLMRoutingServesStoredModels is the point of this change: the
// catalog comes from the database, not from server.yaml.
func TestBuildLLMRoutingServesStoredModels(t *testing.T) {
	sc := config.ServerConfig{
		Conversation: config.ServerConvConfig{Model: conversationModel()},
		LLM: config.ServerLLMConfig{
			DefaultAlias: "default",
			Aliases:      map[string]string{"default": "lm_fast"},
		},
	}

	routing, err := buildLLMRouting(sc, newFakeModels(catalogRow("lm_fast")))
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}

	routed, err := routing.Router.ClientFor(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"})
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	if routed.Resolution.Target.ID != "lm_fast" {
		t.Errorf("Target.ID = %q, want %q", routed.Resolution.Target.ID, "lm_fast")
	}
	if routed.Resolution.Target.UpstreamModel != "vendor/lm_fast" {
		t.Errorf("UpstreamModel = %q", routed.Resolution.Target.UpstreamModel)
	}
	// The credential reference is the model ID; the key itself stays in the
	// store until the factory asks for it.
	if routed.Resolution.Target.CredentialRef != "lm_fast" {
		t.Errorf("CredentialRef = %q, want the model ID", routed.Resolution.Target.CredentialRef)
	}
}

// TestBuildLLMRoutingSurvivesAnEmptyCatalog records the state a fresh
// deployment is in before an operator runs `buildmax-server model add`.
func TestBuildLLMRoutingSurvivesAnEmptyCatalog(t *testing.T) {
	sc := config.ServerConfig{
		Conversation: config.ServerConvConfig{Model: conversationModel()},
		LLM: config.ServerLLMConfig{
			DefaultAlias: "default",
			Aliases:      map[string]string{"default": "lm_missing"},
		},
	}

	routing, err := buildLLMRouting(sc, newFakeModels())
	if err != nil {
		t.Fatalf("an alias with no model row failed startup: %v", err)
	}
	// Tier 1 still works.
	if _, err := routing.Router.ClientForTarget(context.Background(), conversationTargetID, nil); err != nil {
		t.Errorf("ClientForTarget: %v", err)
	}
	// The dangling alias fails its own call and is skipped in listings.
	if _, err := routing.Router.ClientFor(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"}); !errors.Is(err, llmgateway.ErrTargetNotFound) {
		t.Errorf("want ErrTargetNotFound, got %v", err)
	}
	models, err := routing.Router.Available(context.Background(), "tm_one")
	if err != nil {
		t.Fatalf("Available: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("Available returned %d models for a dangling alias", len(models))
	}
}

// TestBuildLLMRoutingDeniesTeamsByDefault records that a catalog is not a grant.
func TestBuildLLMRoutingDeniesTeamsByDefault(t *testing.T) {
	sc := config.ServerConfig{Conversation: config.ServerConvConfig{Model: conversationModel()}}

	routing, err := buildLLMRouting(sc, newFakeModels(catalogRow("lm_fast")))
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}
	if _, err := routing.Router.ClientFor(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"}); !errors.Is(err, llmgateway.ErrTeamNotAuthorized) {
		t.Errorf("want ErrTeamNotAuthorized, got %v", err)
	}
	if _, err := routing.Router.ClientForTarget(context.Background(), conversationTargetID, nil); err != nil {
		t.Errorf("the deployment's own model stopped resolving: %v", err)
	}
}

func TestBuildLLMRoutingTier1UsesAStoredModel(t *testing.T) {
	sc := config.ServerConfig{
		Conversation: config.ServerConvConfig{Model: conversationModel(), ModelTarget: "lm_fast"},
	}

	routing, err := buildLLMRouting(sc, newFakeModels(catalogRow("lm_fast")))
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}
	if routing.Tier1TargetID != "lm_fast" {
		t.Errorf("Tier1TargetID = %q", routing.Tier1TargetID)
	}
	routed, err := routing.Router.ClientForTarget(context.Background(), "lm_fast", llmgateway.BaselineCapabilities())
	if err != nil {
		t.Fatalf("ClientForTarget: %v", err)
	}
	if routed.Resolution.Target.UpstreamModel != "vendor/lm_fast" {
		t.Errorf("UpstreamModel = %q", routed.Resolution.Target.UpstreamModel)
	}
}

func TestBuildLLMRoutingRejectsBadAliases(t *testing.T) {
	tests := []struct {
		name    string
		llm     config.ServerLLMConfig
		wantErr string
	}{
		{
			name:    "alias maps to nothing",
			llm:     config.ServerLLMConfig{Aliases: map[string]string{"default": ""}},
			wantErr: "maps to no target",
		},
		{
			name:    "several aliases without a default",
			llm:     config.ServerLLMConfig{Aliases: map[string]string{"a": "lm_fast", "b": "lm_fast"}},
			wantErr: "no default alias",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildLLMRouting(config.ServerConfig{LLM: tc.llm}, newFakeModels())
			if err == nil {
				t.Fatal("buildLLMRouting accepted the config")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

// TestStoredModelWithoutACredentialFails keeps a half-written catalog row from
// producing a client pointed at a provider with no key.
func TestStoredModelWithoutACredentialFails(t *testing.T) {
	models := newFakeModels(catalogRow("lm_fast"))
	models.credentials["lm_fast"] = ""

	routing, err := buildLLMRouting(config.ServerConfig{
		LLM: config.ServerLLMConfig{Aliases: map[string]string{"only": "lm_fast"}},
	}, models)
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}
	_, err = routing.Router.ClientFor(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"})
	if err == nil || !strings.Contains(err.Error(), "no credential stored") {
		t.Fatalf("want a missing-credential error, got %v", err)
	}
}

// TestStoredModelRowIsValidatedOnRead covers an incomplete row: it must fail its
// own calls rather than build a client pointed at nothing.
func TestStoredModelRowIsValidatedOnRead(t *testing.T) {
	broken := catalogRow("lm_broken")
	broken.APIURL = ""

	routing, err := buildLLMRouting(config.ServerConfig{
		LLM: config.ServerLLMConfig{Aliases: map[string]string{"only": "lm_broken"}},
	}, newFakeModels(broken))
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}
	if _, err := routing.Router.ClientFor(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"}); !errors.Is(err, llmgateway.ErrInvalidCatalog) {
		t.Fatalf("want ErrInvalidCatalog, got %v", err)
	}
}

func TestClientFactoryRejectsUnsupportedProvider(t *testing.T) {
	factory := newClientFactory("conversation-key", nil)

	_, err := factory(context.Background(), llmgateway.Target{
		Name:          "Native",
		ProviderType:  "anthropic_native",
		CredentialRef: conversationCredentialRef,
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported provider") {
		t.Errorf("want an unsupported-provider error, got %v", err)
	}
}

// TestLLMRoutingErrorsCarryNoCredential keeps call diagnostics safe to log.
func TestLLMRoutingErrorsCarryNoCredential(t *testing.T) {
	const secret = "SUPER-SECRET-KEY"

	models := newFakeModels(catalogRow("lm_fast"))
	models.credentials["lm_fast"] = secret
	models.err = errors.New("catalog unavailable")

	routing, err := buildLLMRouting(config.ServerConfig{
		LLM: config.ServerLLMConfig{Aliases: map[string]string{"only": "lm_fast"}},
	}, models)
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}
	if _, err := routing.Router.ClientFor(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"}); err == nil {
		t.Fatal("a failing catalog produced a client")
	} else if strings.Contains(err.Error(), secret) {
		t.Errorf("the error leaked the credential: %v", err)
	}
}

// TestWireLLMPreservesExistingDeployments checks what the handler layer
// actually receives.
func TestWireLLMPreservesExistingDeployments(t *testing.T) {
	var cfg httpserver.Config
	sc := config.ServerConfig{Conversation: config.ServerConvConfig{Model: conversationModel()}}

	if err := wireLLM(&cfg, sc, nil, nil); err != nil {
		t.Fatalf("wireLLM: %v", err)
	}
	if cfg.Conv.ConversationLLMClient == nil {
		t.Error("the Tier 1 client was not wired")
	}
	if cfg.Conv.TitleGenerator == nil {
		t.Error("the title generator was not wired")
	}
	// Without a store there is nowhere to record managed calls, so the gateway
	// stays off rather than serving unmetered inference.
	if cfg.Conv.LLMGateway != nil {
		t.Error("the gateway was wired with no store")
	}

	var unconfigured httpserver.Config
	if err := wireLLM(&unconfigured, config.ServerConfig{}, nil, nil); err != nil {
		t.Fatalf("wireLLM without a model: %v", err)
	}
	if unconfigured.Conv.ConversationLLMClient != nil || unconfigured.Conv.TitleGenerator != nil {
		t.Error("Tier 1 was wired with no conversation model configured")
	}
}

func TestWireLLMFailsStartupOnBadAliases(t *testing.T) {
	var cfg httpserver.Config
	sc := config.ServerConfig{
		Conversation: config.ServerConvConfig{Model: conversationModel()},
		LLM:          config.ServerLLMConfig{Aliases: map[string]string{"default": ""}},
	}

	if err := wireLLM(&cfg, sc, nil, nil); err == nil {
		t.Fatal("an alias mapping to nothing did not fail startup")
	}
	if cfg.Conv.ConversationLLMClient != nil {
		t.Error("a failed wiring left a client behind")
	}
}
