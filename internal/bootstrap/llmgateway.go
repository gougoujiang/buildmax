package bootstrap

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/model"
	llm "github.com/gougoujiang/buildmax/internal/infra/llm"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

// conversationTargetID is the catalog ID of the target derived from
// conversation.model.
//
// The stored catalog can be empty — a fresh deployment has no model rows — so
// the server still needs one model of its own to answer Tier 1 conversations.
// That is what this entry is: a bootstrap path, not a catalog feature.
const conversationTargetID = "conversation"

// conversationCredentialRef names where the derived target's credential came
// from. Stored models use their own ID as the reference, so the two namespaces
// cannot collide.
const conversationCredentialRef = "conversation.model.api_key"

// llmRouting is the gateway wiring for one server process.
type llmRouting struct {
	// Router resolves models and supplies clients.
	Router *llmgateway.Router
	// Tier1TargetID is the catalog target Server-owned conversation inference
	// uses. Empty means no conversation model is configured.
	Tier1TargetID string
}

// denyAllPolicies grants no team any alias.
//
// Managed team access is opt-in: a deployment with models in the catalog but no
// llm.aliases has told us which models exist, not who may call them.
type denyAllPolicies struct{}

func (denyAllPolicies) PolicyForTeam(_ context.Context, teamID string) (llmgateway.TeamPolicy, error) {
	if teamID == "" {
		return llmgateway.TeamPolicy{}, llmgateway.ErrTeamRequired
	}
	return llmgateway.TeamPolicy{}, nil
}

// bootstrapCatalog serves the stored model catalog plus the one model derived
// from server.yaml.
//
// The derived entry is checked first and is not in the database, so a
// deployment can answer conversations before an operator has added a single
// model row.
type bootstrapCatalog struct {
	derived *llmgateway.Target
	stored  *llmgateway.StoreCatalog
}

func (c *bootstrapCatalog) Target(ctx context.Context, id string) (llmgateway.Target, error) {
	if c.derived != nil && id == conversationTargetID {
		return *c.derived, nil
	}
	if c.stored == nil {
		return llmgateway.Target{}, llmgateway.ErrTargetNotFound
	}
	return c.stored.Target(ctx, id)
}

// buildLLMRouting assembles the catalog, team policy, and router.
//
// It returns nil when the deployment has neither a database nor a conversation
// model, which leaves Tier 1 unwired exactly as before the gateway existed.
func buildLLMRouting(sc config.ServerConfig, models model.LLMModelStore) (*llmRouting, error) {
	catalog := &bootstrapCatalog{}
	if entry := sc.Conversation.Model; entry.APIKey != "" {
		derived := derivedConversationTarget(entry)
		catalog.derived = &derived
	}
	if models != nil {
		catalog.stored = &llmgateway.StoreCatalog{Models: models}
	}
	if catalog.derived == nil && catalog.stored == nil {
		return nil, nil
	}

	policies, err := teamPolicySource(sc.LLM)
	if err != nil {
		return nil, err
	}

	tier1 := sc.Conversation.ModelTarget
	if tier1 == "" && catalog.derived != nil {
		tier1 = conversationTargetID
	}

	return &llmRouting{
		Router: &llmgateway.Router{
			Resolver: &llmgateway.Resolver{Catalog: catalog, Policies: policies},
			Factory:  newClientFactory(sc.Conversation.Model.APIKey, models),
		},
		Tier1TargetID: tier1,
	}, nil
}

func derivedConversationTarget(entry config.ServerModelEntry) llmgateway.Target {
	name := entry.Name
	if name == "" {
		name = entry.Model
	}
	apiURL := entry.APIURL
	if apiURL == "" {
		apiURL = config.DefaultOpenRouterBaseURL
	}
	modelName := entry.Model
	if modelName == "" {
		modelName = config.DefaultModel
	}
	providerType := entry.Provider
	if providerType == "" {
		providerType = llmgateway.ProviderOpenAICompatible
	}
	return llmgateway.Target{
		ID:            conversationTargetID,
		Name:          name,
		ProviderType:  providerType,
		Endpoint:      apiURL,
		CredentialRef: conversationCredentialRef,
		UpstreamModel: modelName,
		ContextWindow: entry.ContextWindow,
		CallTimeout:   time.Duration(entry.CallTimeout) * time.Second,
		MaxTokens:     entry.MaxTokens,
		Reasoning:     entry.Reasoning,
		PromptCache:   entry.PromptCache,
		Vision:        entry.Vision,
		Capabilities:  llmgateway.NewCapabilitySet(llmgateway.BaselineCapabilities()...),
		Enabled:       true,
	}
}

// teamPolicySource builds the deployment-wide policy.
//
// It does not cross-check the catalog: models are added and retired through the
// database while the server runs, so an alias naming a model that does not
// exist yet fails its own calls rather than stopping the server.
func teamPolicySource(cfg config.ServerLLMConfig) (llmgateway.PolicySource, error) {
	if len(cfg.Aliases) == 0 {
		return denyAllPolicies{}, nil
	}
	aliases := make(map[string]string, len(cfg.Aliases))
	maps.Copy(aliases, cfg.Aliases)

	defaultAlias := cfg.DefaultAlias
	if defaultAlias == "" && len(aliases) == 1 {
		for alias := range aliases {
			defaultAlias = alias
		}
	}
	source, err := llmgateway.NewPolicySource(llmgateway.TeamPolicy{
		DefaultAlias: defaultAlias,
		Aliases:      aliases,
	})
	if err != nil {
		return nil, fmt.Errorf("llm.aliases: %w", err)
	}
	return source, nil
}

// newClientFactory builds provider clients for approved targets. It is the only
// place a credential reference becomes a real credential.
func newClientFactory(conversationKey string, models model.LLMModelStore) llmgateway.ClientFactory {
	return func(ctx context.Context, target llmgateway.Target) (cllm.LLMClient, error) {
		if !llmgateway.KnownProvider(target.ProviderType) {
			return nil, fmt.Errorf("model %q uses unsupported provider %q; use one of %s",
				target.Name, target.ProviderType, strings.Join(llmgateway.Providers(), ", "))
		}
		apiKey, err := resolveCredential(ctx, target.CredentialRef, conversationKey, models)
		if err != nil {
			return nil, fmt.Errorf("model %q: %w", target.Name, err)
		}
		// The provider type is the catalog's word for a wire protocol, and the
		// client package uses the same values, so the target selects an adapter
		// without a translation table to keep in step.
		client, err := llm.NewClient(llm.Config{
			Provider:      target.ProviderType,
			APIKey:        apiKey,
			BaseURL:       target.Endpoint,
			Model:         target.UpstreamModel,
			ContextWindow: target.ContextWindow,
			MaxTokens:     target.MaxTokens,
			Reasoning:     target.Reasoning,
			PromptCache:   target.PromptCache,
			Vision:        target.Vision,
			Surface:       model.LLMCallSurfaceServer,
			CallTimeout:   target.CallTimeout,
		})
		if err != nil {
			return nil, fmt.Errorf("model %q: %w", target.Name, err)
		}
		return client, nil
	}
}

func resolveCredential(ctx context.Context, ref, conversationKey string, models model.LLMModelStore) (string, error) {
	if ref == conversationCredentialRef {
		if conversationKey == "" {
			return "", fmt.Errorf("conversation.model.api_key is not set")
		}
		return conversationKey, nil
	}
	if models == nil {
		return "", fmt.Errorf("no model store to read a credential from")
	}
	key, err := models.LLMModelCredential(ctx, ref)
	if err != nil {
		return "", err
	}
	if key == "" {
		return "", fmt.Errorf("no credential stored")
	}
	return key, nil
}
