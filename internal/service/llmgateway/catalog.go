// Package llmgateway resolves a team's model alias to an operator-approved
// upstream target. It owns the model catalog, team model policy, and the
// capability contract.
//
// The package deliberately does not open provider connections, read
// configuration files, or speak HTTP: process wiring supplies an already-built
// catalog and policy, and higher layers turn a resolved target into a client.
//
// Mirrors the design in docs/design/llm-gateway.md.
package llmgateway

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// legacyPromptCacheMode is what the deprecated prompt_cache bool becomes when
// it was set. Force rather than the weaker default: it was written when the
// only alternative was no caching at all, so reading it as anything less takes
// away what the operator asked for.
const legacyPromptCacheMode = "force"

// Provider types name the wire protocol a target speaks. A provider type
// selects a client implementation; it is not a vendor name. Claude reached
// through an OpenAI-compatible gateway is ProviderOpenAICompatible, and Claude
// reached at Anthropic's own endpoint is ProviderAnthropic.
//
// The values match config.LLMProvider* so an operator reads the same word in
// settings.yaml, in the catalog, and in the call ledger. This package keeps its
// own copies rather than importing config, because it loads nothing.
const (
	// ProviderOpenAICompatible is OpenAI Chat Completions, the protocol served
	// by OpenRouter, LiteLLM, vLLM, and local inference servers.
	ProviderOpenAICompatible = "openai_compatible"
	// ProviderOpenAI is OpenAI's own Responses API.
	ProviderOpenAI = "openai"
	// ProviderAnthropic is the Anthropic Messages API.
	ProviderAnthropic = "anthropic"
	// ProviderOllama is a local Ollama daemon's own API. A target naming it
	// points at a runtime the deployment reaches directly, which is why it is
	// the one provider with no credential.
	ProviderOllama = "ollama"
)

// KnownProvider reports whether name is a provider type BuildMax implements.
// An empty name is not known: a target must state the protocol it speaks.
func KnownProvider(name string) bool {
	switch name {
	case ProviderOpenAICompatible, ProviderOpenAI, ProviderAnthropic, ProviderOllama:
		return true
	}
	return false
}

// Providers returns every implemented provider type, for help text and error
// messages that must not drift from the list above.
func Providers() []string {
	return []string{ProviderOpenAICompatible, ProviderOpenAI, ProviderAnthropic, ProviderOllama}
}

// ProviderNeedsCredential reports whether a target speaking this protocol must
// have a secret behind its CredentialRef.
//
// Every hosted protocol does, and a catalog entry without one is a
// misconfiguration that must fail at the first call rather than send an
// unauthenticated request. A local runtime has no credential to hold: what
// authorizes the call is being able to reach the daemon at all, which is a
// property of the deployment's network rather than of the catalog.
func ProviderNeedsCredential(name string) bool {
	return name != ProviderOllama
}

// Target is one operator-approved upstream in the model catalog.
//
// A Target never carries a credential. CredentialRef names a secret that
// process wiring resolves separately, so a resolved target can be compared,
// listed, and mentioned in diagnostics without leaking provider access.
type Target struct {
	// ID is the opaque catalog identifier referenced by team policy.
	ID string
	// Name is the operator-facing display name.
	Name string
	// ProviderType selects the client implementation, e.g. ProviderOpenAICompatible.
	ProviderType string
	// Endpoint is the upstream base URL. Only an operator may set it; it is
	// never accepted from a client request.
	Endpoint string
	// CredentialRef names the secret used for this target, not the secret.
	CredentialRef string
	// UpstreamModel is the provider's own model identifier.
	UpstreamModel string
	// ContextWindow is the usable context size; 0 means the client default.
	ContextWindow int
	// CallTimeout bounds one upstream call; 0 means the client default.
	CallTimeout time.Duration
	// MaxTokens caps one response; 0 means the client default.
	MaxTokens int
	// Reasoning is the effort level the upstream is asked for; off means none.
	// The gateway carries the resulting state without reading it.
	Reasoning string
	// CacheMode and CacheTTL are the operator's prompt-cache policy: which
	// calls ask the upstream to cache the stable prefix of a request, and for
	// how long. They are carried as plain strings for the same reason the
	// provider constants above are redefined here — this package resolves what
	// a team may call without depending on how a process reads configuration.
	// A managed caller never supplies either: the operator's target does.
	CacheMode string
	CacheTTL  string
	// Vision says the upstream accepts image input.
	Vision bool
	// Capabilities is what this target declares it can do.
	Capabilities CapabilitySet
	// Enabled allows an operator to retire a target without deleting it.
	Enabled bool
}

// Catalog provides operator-approved targets by catalog ID. Implementations
// return ErrTargetNotFound for an unknown ID.
type Catalog interface {
	Target(ctx context.Context, id string) (Target, error)
}

// StaticCatalog is an immutable in-memory catalog, used for deployment-wide
// configuration before a database-backed catalog exists.
type StaticCatalog struct {
	targets map[string]Target
	ids     []string
}

// NewStaticCatalog validates the targets and builds a catalog. Validation
// happens once at startup so an operator misconfiguration surfaces there
// rather than on a user's first call.
func NewStaticCatalog(targets []Target) (*StaticCatalog, error) {
	byID := make(map[string]Target, len(targets))
	ids := make([]string, 0, len(targets))
	for i, target := range targets {
		if err := validateTarget(i, target); err != nil {
			return nil, err
		}
		if _, duplicate := byID[target.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate target id %q", ErrInvalidCatalog, target.ID)
		}
		byID[target.ID] = target
		ids = append(ids, target.ID)
	}
	slices.Sort(ids)
	return &StaticCatalog{targets: byID, ids: ids}, nil
}

func validateTarget(index int, target Target) error {
	switch {
	case target.ID == "":
		return fmt.Errorf("%w: target %d has no id", ErrInvalidCatalog, index)
	case target.ProviderType == "":
		return fmt.Errorf("%w: target %q has no provider type", ErrInvalidCatalog, target.ID)
	case target.Endpoint == "":
		return fmt.Errorf("%w: target %q has no endpoint", ErrInvalidCatalog, target.ID)
	case target.UpstreamModel == "":
		return fmt.Errorf("%w: target %q has no upstream model", ErrInvalidCatalog, target.ID)
	case len(target.Capabilities) == 0:
		return fmt.Errorf("%w: target %q declares no capabilities", ErrInvalidCatalog, target.ID)
	case target.ContextWindow < 0:
		return fmt.Errorf("%w: target %q has a negative context window", ErrInvalidCatalog, target.ID)
	case target.CallTimeout < 0:
		return fmt.Errorf("%w: target %q has a negative call timeout", ErrInvalidCatalog, target.ID)
	case target.MaxTokens < 0:
		return fmt.Errorf("%w: target %q has a negative max tokens", ErrInvalidCatalog, target.ID)
	}
	return nil
}

// Target returns the target for the catalog ID.
func (c *StaticCatalog) Target(_ context.Context, id string) (Target, error) {
	if c == nil {
		return Target{}, ErrCatalogNotConfigured
	}
	target, ok := c.targets[id]
	if !ok {
		return Target{}, ErrTargetNotFound
	}
	return target, nil
}

// IDs returns every catalog ID in a stable order. Policy validation uses it to
// reject an alias pointing at a target that does not exist.
func (c *StaticCatalog) IDs() []string {
	if c == nil || len(c.ids) == 0 {
		return nil
	}
	out := make([]string, len(c.ids))
	copy(out, c.ids)
	return out
}
