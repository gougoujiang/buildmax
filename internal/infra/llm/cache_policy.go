package llm

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// Cache strategies named in diagnostics. A strategy says how a request was
// shaped, not whether the provider then hit: only the provider can report that.
const (
	cacheStrategyNone            = "none"
	cacheStrategyAnthropicStatic = "anthropic_static_and_rolling"
	cacheStrategyOpenAIImplicit  = "openai_implicit"
	cacheCapabilitySupported     = "supported"
	cacheCapabilityUnsupported   = "unsupported"
	cacheCapabilityImplicitOnly  = "implicit"
)

// cacheCapability is what one protocol can be asked for.
//
// Capability belongs to a target rather than to a protocol label, but a direct
// entry has only its provider to go on, so the protocol is the best evidence
// available. Compatibility with a protocol is not a promise to implement its
// features: openai_compatible is a family of endpoints BuildMax has not tested,
// so it declares nothing rather than sending a field a gateway may reject.
type cacheCapability struct {
	// requestControls says the protocol takes cache instructions in the
	// request. False does not mean the provider never caches — it may cache
	// implicitly — only that asking is not a thing this protocol does.
	requestControls bool
	// ttls are the retentions the protocol documents, beyond the provider's
	// own default. Empty means retention is not selectable.
	ttls []string
	// strategy names the request shape used when controls are sent.
	strategy string
	// reported is what telemetry calls this capability.
	reported string
}

// cacheCapabilityFor returns what the named protocol supports.
func cacheCapabilityFor(provider string) cacheCapability {
	switch provider {
	case config.LLMProviderAnthropic:
		// This protocol has no automatic caching: nothing is cached unless the
		// request says where. 24h is in the vocabulary but not in this
		// protocol, so asking for it is refused rather than silently served at
		// a shorter retention.
		return cacheCapability{
			requestControls: true,
			ttls:            []string{config.CacheTTL5m, config.CacheTTL1h},
			strategy:        cacheStrategyAnthropicStatic,
			reported:        cacheCapabilitySupported,
		}
	case config.LLMProviderOpenAI:
		// Responses caches on its own and reports what it cached. The native
		// controls — prompt_cache_key, cache options, retention — are not wired
		// yet, so there is nothing to put in the request and no retention to
		// select. Reported as implicit rather than supported: a caller must not
		// read "supported" as "BuildMax asked for this".
		return cacheCapability{strategy: cacheStrategyOpenAIImplicit, reported: cacheCapabilityImplicitOnly}
	}
	// openai_compatible and ollama. The first is a protocol family, not a
	// feature guarantee; the second is a local runtime that reuses its own
	// cache with no request-side control and nothing to report.
	return cacheCapability{strategy: cacheStrategyNone, reported: cacheCapabilityUnsupported}
}

// supportsTTL reports whether the protocol documents this retention.
func (c cacheCapability) supportsTTL(ttl string) bool {
	if ttl == "" || ttl == config.CacheTTLProviderDefault {
		return true
	}
	return slices.Contains(c.ttls, ttl)
}

// validateCachePolicy refuses a policy this target cannot honour.
//
// Only force is refused for being unsupported. An auto policy on a target
// without controls is the normal case — most models are — and erroring on it
// would make the default mode unusable. Force is different: it is a caller
// saying it needs the cache, and serving that silently as no cache at all would
// answer a question nobody asked.
func validateCachePolicy(policy config.CacheControl, capability cacheCapability, provider string) error {
	if err := config.ValidateCacheControl(policy); err != nil {
		return err
	}
	if policy.Mode == config.CacheModeForce && !capability.requestControls {
		return fmt.Errorf("prompt cache mode %q is not available on provider %q: it takes no cache instructions in a request",
			config.CacheModeForce, provider)
	}
	if policy.Mode == config.CacheModeOff {
		// An off policy sends nothing, so a retention it names is inert rather
		// than wrong. Refusing it would fail a config whose author had already
		// said they want no caching.
		return nil
	}
	if !capability.supportsTTL(policy.TTL) {
		supported := append([]string{config.CacheTTLProviderDefault}, capability.ttls...)
		return fmt.Errorf("prompt cache ttl %q is not available on provider %q: use one of %s",
			policy.TTL, provider, strings.Join(supported, ", "))
	}
	return nil
}

// cacheDecision is what one call does about caching.
type cacheDecision struct {
	// send says to put cache instructions in this request.
	send bool
	// ttl is the retention to ask for, empty for the provider's default.
	ttl string
	// strategy and capability are what telemetry reports about the decision.
	strategy   string
	capability string
	// mode is the policy that produced the decision.
	mode string
}

// resolveCacheDecision combines the target's policy, the target's capability,
// and what this particular call is for.
//
// The profile is the part configuration cannot supply. A cache write costs more
// than ordinary input and only pays for itself when a later call reads it, so
// auto asks only where reuse is established: the agent loop, whose prefix goes
// out again on the next iteration. A title, a compaction summary, or a probe is
// asked once and never asked again with the same prefix, and buying a write for
// one is a straight loss.
func resolveCacheDecision(policy config.CacheControl, capability cacheCapability, profile cllm.CallProfile) cacheDecision {
	out := cacheDecision{
		mode:       policy.Mode,
		capability: capability.reported,
		strategy:   cacheStrategyNone,
	}
	if out.mode == "" {
		out.mode = config.CacheModeAuto
	}
	if !capability.requestControls {
		// Nothing to send. The strategy still names what the protocol does on
		// its own, so an implicit cache is not reported as no cache at all.
		out.strategy = capability.strategy
		if out.mode == config.CacheModeOff {
			out.strategy = cacheStrategyNone
		}
		return out
	}
	switch out.mode {
	case config.CacheModeOff:
		return out
	case config.CacheModeForce:
		out.send = true
	default:
		// An unrecognised profile is treated as one whose reuse is unknown, and
		// unknown reuse does not justify a write. A caller that means to pay
		// for one says force.
		out.send = profile == cllm.ProfileAgentTurn
	}
	if !out.send {
		return out
	}
	out.strategy = capability.strategy
	if policy.TTL != "" && policy.TTL != config.CacheTTLProviderDefault {
		out.ttl = policy.TTL
	}
	return out
}
