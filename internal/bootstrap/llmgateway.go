package bootstrap

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	llm "github.com/gougoujiang/buildmax/internal/infra/llm"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

// conversationTargetID is the catalog ID of the target derived from
// conversation.model, so a server configured before the catalog existed still
// has a named entry to route Tier 1 inference through.
const conversationTargetID = "conversation"

// conversationCredentialRef and llmTargetCredentialRef name where a target's
// credential came from. They are configuration paths, not secrets: a Target
// carries the reference and the factory holds the value.
const conversationCredentialRef = "conversation.model.api_key"

func llmTargetCredentialRef(id string) string { return "llm.targets." + id + ".api_key" }

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
// Managed team access is opt-in: a deployment that configures a catalog but no
// llm.aliases has told us which models exist, not who may call them.
type denyAllPolicies struct{}

func (denyAllPolicies) PolicyForTeam(_ context.Context, teamID string) (llmgateway.TeamPolicy, error) {
	if teamID == "" {
		return llmgateway.TeamPolicy{}, llmgateway.ErrTeamRequired
	}
	return llmgateway.TeamPolicy{}, nil
}

// buildLLMRouting builds the model catalog, team policy, and router from
// server.yaml. It returns nil when the deployment configures no model at all,
// which leaves Tier 1 conversation unwired exactly as before the gateway.
func buildLLMRouting(sc config.ServerConfig) (*llmRouting, error) {
	targets, credentials, err := catalogTargets(sc)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, nil
	}

	catalog, err := llmgateway.NewStaticCatalog(targets)
	if err != nil {
		return nil, fmt.Errorf("model catalog: %w", err)
	}

	policies, err := teamPolicySource(sc.LLM, catalog.IDs())
	if err != nil {
		return nil, err
	}

	tier1, err := tier1TargetID(sc, catalog.IDs())
	if err != nil {
		return nil, err
	}

	return &llmRouting{
		Router: &llmgateway.Router{
			Resolver: &llmgateway.Resolver{Catalog: catalog, Policies: policies},
			Factory:  newClientFactory(credentials),
		},
		Tier1TargetID: tier1,
	}, nil
}

// catalogTargets converts configured targets, plus the target derived from
// conversation.model, into catalog entries and their credentials.
func catalogTargets(sc config.ServerConfig) ([]llmgateway.Target, map[string]string, error) {
	targets := make([]llmgateway.Target, 0, len(sc.LLM.Targets)+1)
	credentials := make(map[string]string, len(sc.LLM.Targets)+1)

	if model := sc.Conversation.Model; model.APIKey != "" {
		targets = append(targets, derivedConversationTarget(model))
		credentials[conversationCredentialRef] = model.APIKey
	}

	for i, entry := range sc.LLM.Targets {
		target, err := configuredTarget(i, entry)
		if err != nil {
			return nil, nil, err
		}
		targets = append(targets, target)
		credentials[target.CredentialRef] = entry.APIKey
	}
	return targets, credentials, nil
}

func derivedConversationTarget(model config.ServerModelEntry) llmgateway.Target {
	name := model.Name
	if name == "" {
		name = model.Model
	}
	apiURL := model.APIURL
	if apiURL == "" {
		apiURL = config.DefaultOpenRouterBaseURL
	}
	modelName := model.Model
	if modelName == "" {
		modelName = config.DefaultModel
	}
	return llmgateway.Target{
		ID:            conversationTargetID,
		Name:          name,
		ProviderType:  llmgateway.ProviderOpenAICompatible,
		Endpoint:      apiURL,
		CredentialRef: conversationCredentialRef,
		UpstreamModel: modelName,
		ContextWindow: model.ContextWindow,
		CallTimeout:   time.Duration(model.CallTimeout) * time.Second,
		Capabilities:  llmgateway.NewCapabilitySet(llmgateway.BaselineCapabilities()...),
		Enabled:       true,
	}
}

func configuredTarget(index int, entry config.ServerLLMTarget) (llmgateway.Target, error) {
	if entry.ID == "" {
		return llmgateway.Target{}, fmt.Errorf("llm.targets[%d]: id is required", index)
	}
	if entry.ID == conversationTargetID {
		return llmgateway.Target{}, fmt.Errorf("llm.targets[%d]: id %q is reserved for conversation.model", index, conversationTargetID)
	}
	if entry.APIKey == "" {
		return llmgateway.Target{}, fmt.Errorf("llm.targets[%d] (%s): api_key is required", index, entry.ID)
	}
	provider := entry.Provider
	if provider == "" {
		provider = llmgateway.ProviderOpenAICompatible
	}
	capabilities, err := parseCapabilities(entry.Capabilities)
	if err != nil {
		return llmgateway.Target{}, fmt.Errorf("llm.targets[%d] (%s): %w", index, entry.ID, err)
	}
	name := entry.Name
	if name == "" {
		name = entry.ID
	}
	return llmgateway.Target{
		ID:            entry.ID,
		Name:          name,
		ProviderType:  provider,
		Endpoint:      entry.APIURL,
		CredentialRef: llmTargetCredentialRef(entry.ID),
		UpstreamModel: entry.Model,
		ContextWindow: entry.ContextWindow,
		CallTimeout:   time.Duration(entry.CallTimeout) * time.Second,
		Capabilities:  capabilities,
		Enabled:       !entry.Disabled,
	}, nil
}

// parseCapabilities maps configured capability names onto the service contract.
// An empty list means the capability set an OpenAI-compatible client already
// guarantees; that is the provider contract, not a guess from a vendor name.
func parseCapabilities(names []string) (llmgateway.CapabilitySet, error) {
	if len(names) == 0 {
		return llmgateway.NewCapabilitySet(llmgateway.BaselineCapabilities()...), nil
	}
	known := llmgateway.NewCapabilitySet(llmgateway.BaselineCapabilities()...)
	parsed := make([]llmgateway.Capability, 0, len(names))
	for _, name := range names {
		capability := llmgateway.Capability(name)
		if !known.Has(capability) {
			return nil, fmt.Errorf("unknown capability %q", name)
		}
		parsed = append(parsed, capability)
	}
	return llmgateway.NewCapabilitySet(parsed...), nil
}

func teamPolicySource(cfg config.ServerLLMConfig, knownTargetIDs []string) (llmgateway.PolicySource, error) {
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
	policy := llmgateway.TeamPolicy{DefaultAlias: defaultAlias, Aliases: aliases}
	source, err := llmgateway.NewStaticPolicySource(policy, knownTargetIDs)
	if err != nil {
		return nil, fmt.Errorf("llm.aliases: %w", err)
	}
	return source, nil
}

func tier1TargetID(sc config.ServerConfig, knownTargetIDs []string) (string, error) {
	targetID := sc.Conversation.ModelTarget
	if targetID == "" {
		if sc.Conversation.Model.APIKey != "" {
			return conversationTargetID, nil
		}
		return "", nil
	}
	if slices.Contains(knownTargetIDs, targetID) {
		return targetID, nil
	}
	return "", fmt.Errorf("conversation.model_target %q is not a configured llm.targets id", targetID)
}

// newClientFactory builds provider clients for approved targets. It is the only
// place a target's credential reference becomes a real credential.
func newClientFactory(credentials map[string]string) llmgateway.ClientFactory {
	stored := make(map[string]string, len(credentials))
	maps.Copy(stored, credentials)

	return func(_ context.Context, target llmgateway.Target) (cllm.LLMClient, error) {
		if target.ProviderType != llmgateway.ProviderOpenAICompatible {
			return nil, fmt.Errorf("model target %q uses unsupported provider %q", target.ID, target.ProviderType)
		}
		apiKey := stored[target.CredentialRef]
		if apiKey == "" {
			return nil, fmt.Errorf("model target %q has no configured credential", target.ID)
		}
		return llm.NewClient(llm.Config{
			APIKey:        apiKey,
			BaseURL:       target.Endpoint,
			Model:         target.UpstreamModel,
			ContextWindow: target.ContextWindow,
			CallTimeout:   target.CallTimeout,
		}), nil
	}
}
