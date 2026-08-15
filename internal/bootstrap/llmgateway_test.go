package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
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

func catalogTarget(id string) config.ServerLLMTarget {
	return config.ServerLLMTarget{
		ID:            id,
		Name:          strings.ToUpper(id),
		Model:         "vendor/" + id,
		APIURL:        "https://upstream.example.com/v1",
		APIKey:        id + "-key",
		ContextWindow: 64000,
		CallTimeout:   120,
	}
}

// TestBuildLLMRoutingWithoutConfiguration is the regression guard for the
// existing deployment shape: nothing configured means nothing wired, exactly as
// before the gateway existed.
func TestBuildLLMRoutingWithoutConfiguration(t *testing.T) {
	routing, err := buildLLMRouting(config.ServerConfig{})
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}
	if routing != nil {
		t.Fatalf("routing = %+v, want nil", routing)
	}
}

// TestBuildLLMRoutingDerivesConversationTarget covers the deployment that only
// configures conversation.model, which is every server built before this
// feature. Tier 1 must still resolve, through the derived catalog entry.
func TestBuildLLMRoutingDerivesConversationTarget(t *testing.T) {
	sc := config.ServerConfig{Conversation: config.ServerConvConfig{Model: conversationModel()}}

	routing, err := buildLLMRouting(sc)
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}
	if routing == nil {
		t.Fatal("routing is nil")
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
	if got := routed.Resolution.Target.ContextWindow; got != 128000 {
		t.Errorf("ContextWindow = %d, want %d", got, 128000)
	}
	if routed.Resolution.Alias != "" {
		t.Errorf("Alias = %q, want empty for a deployment-owned target", routed.Resolution.Alias)
	}
}

// TestBuildLLMRoutingDeniesTeamsByDefault records that a catalog is not a grant:
// managed team access needs llm.aliases.
func TestBuildLLMRoutingDeniesTeamsByDefault(t *testing.T) {
	sc := config.ServerConfig{
		Conversation: config.ServerConvConfig{Model: conversationModel()},
		LLM:          config.ServerLLMConfig{Targets: []config.ServerLLMTarget{catalogTarget("fast")}},
	}

	routing, err := buildLLMRouting(sc)
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}

	_, err = routing.Router.ClientFor(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"})
	if !errors.Is(err, llmgateway.ErrTeamNotAuthorized) {
		t.Errorf("want ErrTeamNotAuthorized, got %v", err)
	}
	if _, err := routing.Router.Available(context.Background(), "tm_one"); !errors.Is(err, llmgateway.ErrTeamNotAuthorized) {
		t.Errorf("Available: want ErrTeamNotAuthorized, got %v", err)
	}
	// The deployment's own model still resolves.
	if _, err := routing.Router.ClientForTarget(context.Background(), conversationTargetID, nil); err != nil {
		t.Errorf("ClientForTarget: %v", err)
	}
}

func TestBuildLLMRoutingWithTeamAliases(t *testing.T) {
	sc := config.ServerConfig{
		Conversation: config.ServerConvConfig{Model: conversationModel()},
		LLM: config.ServerLLMConfig{
			DefaultAlias: "default",
			Aliases:      map[string]string{"default": "fast", "deep": "reasoning"},
			Targets:      []config.ServerLLMTarget{catalogTarget("fast"), catalogTarget("reasoning")},
		},
	}

	routing, err := buildLLMRouting(sc)
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}

	routed, err := routing.Router.ClientFor(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"})
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	if routed.Resolution.Alias != "default" || routed.Resolution.Target.ID != "fast" {
		t.Errorf("resolved %q -> %q, want default -> fast", routed.Resolution.Alias, routed.Resolution.Target.ID)
	}

	models, err := routing.Router.Available(context.Background(), "tm_one")
	if err != nil {
		t.Fatalf("Available: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("Available returned %d models, want 2", len(models))
	}
}

func TestBuildLLMRoutingSingleAliasNeedsNoDefault(t *testing.T) {
	sc := config.ServerConfig{
		LLM: config.ServerLLMConfig{
			Aliases: map[string]string{"only": "fast"},
			Targets: []config.ServerLLMTarget{catalogTarget("fast")},
		},
	}

	routing, err := buildLLMRouting(sc)
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}
	routed, err := routing.Router.ClientFor(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"})
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	if routed.Resolution.Alias != "only" {
		t.Errorf("Alias = %q, want %q", routed.Resolution.Alias, "only")
	}
	// No conversation model is configured, so Tier 1 stays unwired.
	if routing.Tier1TargetID != "" {
		t.Errorf("Tier1TargetID = %q, want empty", routing.Tier1TargetID)
	}
}

func TestBuildLLMRoutingTier1UsesNamedTarget(t *testing.T) {
	sc := config.ServerConfig{
		Conversation: config.ServerConvConfig{Model: conversationModel(), ModelTarget: "fast"},
		LLM:          config.ServerLLMConfig{Targets: []config.ServerLLMTarget{catalogTarget("fast")}},
	}

	routing, err := buildLLMRouting(sc)
	if err != nil {
		t.Fatalf("buildLLMRouting: %v", err)
	}
	if routing.Tier1TargetID != "fast" {
		t.Errorf("Tier1TargetID = %q, want %q", routing.Tier1TargetID, "fast")
	}
	routed, err := routing.Router.ClientForTarget(context.Background(), routing.Tier1TargetID, llmgateway.BaselineCapabilities())
	if err != nil {
		t.Fatalf("ClientForTarget: %v", err)
	}
	if got := routed.Resolution.Target.UpstreamModel; got != "vendor/fast" {
		t.Errorf("UpstreamModel = %q, want %q", got, "vendor/fast")
	}
}

func TestBuildLLMRoutingRejectsBadConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		sc      config.ServerConfig
		wantErr string
	}{
		{
			name: "target without id",
			sc: config.ServerConfig{LLM: config.ServerLLMConfig{
				Targets: []config.ServerLLMTarget{{Model: "vendor/x", APIURL: "https://x", APIKey: "k"}},
			}},
			wantErr: "id is required",
		},
		{
			name: "target reusing the reserved conversation id",
			sc: config.ServerConfig{LLM: config.ServerLLMConfig{
				Targets: []config.ServerLLMTarget{catalogTarget(conversationTargetID)},
			}},
			wantErr: "reserved",
		},
		{
			name: "target without a credential",
			sc: config.ServerConfig{LLM: config.ServerLLMConfig{
				Targets: []config.ServerLLMTarget{{ID: "fast", Model: "vendor/x", APIURL: "https://x"}},
			}},
			wantErr: "api_key is required",
		},
		{
			name: "target without an endpoint",
			sc: config.ServerConfig{LLM: config.ServerLLMConfig{
				Targets: []config.ServerLLMTarget{{ID: "fast", Model: "vendor/x", APIKey: "k"}},
			}},
			wantErr: "no endpoint",
		},
		{
			name: "unknown capability",
			sc: config.ServerConfig{LLM: config.ServerLLMConfig{
				Targets: []config.ServerLLMTarget{func() config.ServerLLMTarget {
					target := catalogTarget("fast")
					target.Capabilities = []string{"telepathy"}
					return target
				}()},
			}},
			wantErr: `unknown capability "telepathy"`,
		},
		{
			name: "alias pointing at no target",
			sc: config.ServerConfig{LLM: config.ServerLLMConfig{
				Aliases: map[string]string{"default": "missing"},
				Targets: []config.ServerLLMTarget{catalogTarget("fast")},
			}},
			wantErr: "unknown target",
		},
		{
			name: "several aliases without a default",
			sc: config.ServerConfig{LLM: config.ServerLLMConfig{
				Aliases: map[string]string{"a": "fast", "b": "fast"},
				Targets: []config.ServerLLMTarget{catalogTarget("fast")},
			}},
			wantErr: "no default alias",
		},
		{
			name: "conversation target that does not exist",
			sc: config.ServerConfig{
				Conversation: config.ServerConvConfig{Model: conversationModel(), ModelTarget: "missing"},
			},
			wantErr: "is not a configured llm.targets id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			routing, err := buildLLMRouting(tc.sc)
			if err == nil {
				t.Fatalf("buildLLMRouting accepted the config: %+v", routing)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

// TestLLMRoutingErrorsCarryNoCredential keeps startup diagnostics safe to log.
func TestLLMRoutingErrorsCarryNoCredential(t *testing.T) {
	const secret = "SUPER-SECRET-KEY"

	target := catalogTarget("fast")
	target.APIKey = secret
	target.Capabilities = []string{"telepathy"}

	_, err := buildLLMRouting(config.ServerConfig{
		LLM: config.ServerLLMConfig{Targets: []config.ServerLLMTarget{target}},
	})
	if err == nil {
		t.Fatal("buildLLMRouting accepted an unknown capability")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("startup error leaked the credential: %v", err)
	}

	// A target whose credential was never stored must fail without echoing one.
	factory := newClientFactory(nil)
	_, err = factory(context.Background(), llmgateway.Target{
		ID:            "fast",
		ProviderType:  llmgateway.ProviderOpenAICompatible,
		CredentialRef: "llm.targets.fast.api_key",
	})
	if err == nil {
		t.Fatal("the factory built a client with no credential")
	}
	if !strings.Contains(err.Error(), "no configured credential") {
		t.Errorf("unexpected factory error: %v", err)
	}
}

func TestClientFactoryRejectsUnsupportedProvider(t *testing.T) {
	factory := newClientFactory(map[string]string{"ref": "key"})

	_, err := factory(context.Background(), llmgateway.Target{
		ID:            "fast",
		ProviderType:  "anthropic_native",
		CredentialRef: "ref",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported provider") {
		t.Errorf("want an unsupported-provider error, got %v", err)
	}
}

func TestParseCapabilitiesDefaultsToProviderContract(t *testing.T) {
	set, err := parseCapabilities(nil)
	if err != nil {
		t.Fatalf("parseCapabilities: %v", err)
	}
	if missing := set.Missing(llmgateway.BaselineCapabilities()); len(missing) > 0 {
		t.Errorf("default set is missing %v", missing)
	}

	set, err = parseCapabilities([]string{string(llmgateway.CapabilityTextChat)})
	if err != nil {
		t.Fatalf("parseCapabilities: %v", err)
	}
	if set.Has(llmgateway.CapabilityToolCalls) {
		t.Error("an explicit list must not be widened to the baseline")
	}
}

// TestWireLLMPreservesExistingDeployments checks what the handler
// layer actually receives, not just the routing struct.
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

	// No model configured: Tier 1 stays unwired and startup still succeeds.
	var unconfigured httpserver.Config
	if err := wireLLM(&unconfigured, config.ServerConfig{}, nil, nil); err != nil {
		t.Fatalf("wireLLM without a model: %v", err)
	}
	if unconfigured.Conv.ConversationLLMClient != nil || unconfigured.Conv.TitleGenerator != nil {
		t.Error("Tier 1 was wired with no conversation model configured")
	}
}

// TestWireLLMFailsStartupOnBadCatalog keeps a misconfigured catalog
// from starting a server that silently has no Tier 1 model.
func TestWireLLMFailsStartupOnBadCatalog(t *testing.T) {
	var cfg httpserver.Config
	sc := config.ServerConfig{
		Conversation: config.ServerConvConfig{Model: conversationModel()},
		LLM: config.ServerLLMConfig{
			Aliases: map[string]string{"default": "missing"},
			Targets: []config.ServerLLMTarget{catalogTarget("fast")},
		},
	}

	if err := wireLLM(&cfg, sc, nil, nil); err == nil {
		t.Fatal("a dangling alias did not fail startup")
	}
	if cfg.Conv.ConversationLLMClient != nil {
		t.Error("a failed wiring left a client behind")
	}
}
