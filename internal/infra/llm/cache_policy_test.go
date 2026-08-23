package llm

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// The decision table from docs/design/prompt-cache-control.md section 4.
//
// What it protects is the one thing configuration cannot express: an agent turn
// and a title send the same shape of request, and only one of them will be sent
// again. Asking for a cache on the other buys a write nothing reads.
func TestResolveCacheDecision(t *testing.T) {
	anthropic := cacheCapabilityFor(config.LLMProviderAnthropic)
	responses := cacheCapabilityFor(config.LLMProviderOpenAI)
	compatible := cacheCapabilityFor(config.LLMProviderOpenAICompatible)

	tests := []struct {
		name       string
		policy     config.CacheControl
		capability cacheCapability
		profile    cllm.CallProfile
		wantSend   bool
		wantTTL    string
	}{
		{
			name:       "auto asks on an agent turn",
			policy:     config.CacheControl{Mode: config.CacheModeAuto},
			capability: anthropic, profile: cllm.ProfileAgentTurn, wantSend: true,
		},
		{
			name:       "auto stays quiet on a title",
			policy:     config.CacheControl{Mode: config.CacheModeAuto},
			capability: anthropic, profile: cllm.ProfileTitle,
		},
		{
			name:       "auto stays quiet on a compaction",
			policy:     config.CacheControl{Mode: config.CacheModeAuto},
			capability: anthropic, profile: cllm.ProfileCompaction,
		},
		{
			name:       "auto stays quiet on a probe",
			policy:     config.CacheControl{Mode: config.CacheModeAuto},
			capability: anthropic, profile: cllm.ProfileProbe,
		},
		{
			// An empty policy is what an entry that configured nothing produces.
			name:       "an unset mode is auto",
			policy:     config.CacheControl{},
			capability: anthropic, profile: cllm.ProfileAgentTurn, wantSend: true,
		},
		{
			// A caller this build does not know about has not established that
			// anything will read its prefix back.
			name:       "auto stays quiet on an unknown profile",
			policy:     config.CacheControl{Mode: config.CacheModeAuto},
			capability: anthropic, profile: cllm.CallProfile("something_new"),
		},
		{
			name:       "off stays quiet on an agent turn",
			policy:     config.CacheControl{Mode: config.CacheModeOff},
			capability: anthropic, profile: cllm.ProfileAgentTurn,
		},
		{
			name:       "force asks on a title",
			policy:     config.CacheControl{Mode: config.CacheModeForce},
			capability: anthropic, profile: cllm.ProfileTitle, wantSend: true,
		},
		{
			name:       "an explicit ttl reaches the request",
			policy:     config.CacheControl{Mode: config.CacheModeAuto, TTL: config.CacheTTL1h},
			capability: anthropic, profile: cllm.ProfileAgentTurn, wantSend: true, wantTTL: config.CacheTTL1h,
		},
		{
			// The provider default is left to the provider rather than pinned
			// to whatever this build believes it to be.
			name:       "provider_default sends no ttl",
			policy:     config.CacheControl{Mode: config.CacheModeAuto, TTL: config.CacheTTLProviderDefault},
			capability: anthropic, profile: cllm.ProfileAgentTurn, wantSend: true,
		},
		{
			// Responses caches on its own; there is nothing to put in a request.
			name:       "a protocol without request controls sends nothing",
			policy:     config.CacheControl{Mode: config.CacheModeAuto},
			capability: responses, profile: cllm.ProfileAgentTurn,
		},
		{
			name:       "an untested compatible endpoint sends nothing",
			policy:     config.CacheControl{Mode: config.CacheModeAuto},
			capability: compatible, profile: cllm.ProfileAgentTurn,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveCacheDecision(tc.policy, tc.capability, tc.profile)
			if got.send != tc.wantSend {
				t.Errorf("send = %v, want %v", got.send, tc.wantSend)
			}
			if got.ttl != tc.wantTTL {
				t.Errorf("ttl = %q, want %q", got.ttl, tc.wantTTL)
			}
			if !got.send && got.strategy == cacheStrategyAnthropicStatic {
				t.Error("a request that carries no controls reported a strategy that means it did")
			}
		})
	}
}

// An implicit cache is not the same fact as no cache, and reporting either as
// the other misleads a reader trying to work out why a call cost what it did.
func TestCacheCapabilityReporting(t *testing.T) {
	tests := map[string]struct {
		wantReported string
		wantControls bool
	}{
		config.LLMProviderAnthropic:        {cacheCapabilitySupported, true},
		config.LLMProviderOpenAI:           {cacheCapabilityImplicitOnly, false},
		config.LLMProviderOpenAICompatible: {cacheCapabilityUnsupported, false},
		config.LLMProviderOllama:           {cacheCapabilityUnsupported, false},
	}
	for provider, want := range tests {
		t.Run(provider, func(t *testing.T) {
			got := cacheCapabilityFor(provider)
			if got.reported != want.wantReported {
				t.Errorf("reported = %q, want %q", got.reported, want.wantReported)
			}
			if got.requestControls != want.wantControls {
				t.Errorf("requestControls = %v, want %v", got.requestControls, want.wantControls)
			}
		})
	}
}

// Serving force as no caching at all would answer a question nobody asked, so a
// target that takes no cache instructions refuses the policy instead. Auto is
// not refused: most targets are like this, and erroring on them would make the
// default mode unusable.
func TestForceIsRefusedWhereItCannotBeHonoured(t *testing.T) {
	for _, provider := range []string{config.LLMProviderOpenAICompatible, config.LLMProviderOpenAI, config.LLMProviderOllama} {
		t.Run(provider, func(t *testing.T) {
			_, err := NewClient(Config{
				Provider: provider, APIKey: "k", BaseURL: "http://localhost:1", Model: "m",
				CacheControl: config.CacheControl{Mode: config.CacheModeForce},
			})
			if err == nil {
				t.Fatal("force was accepted on a target that takes no cache instructions")
			}
			if _, err := NewClient(Config{
				Provider: provider, APIKey: "k", BaseURL: "http://localhost:1", Model: "m",
				CacheControl: config.CacheControl{Mode: config.CacheModeAuto},
			}); err != nil {
				t.Errorf("auto should be accepted everywhere: %v", err)
			}
		})
	}
}

// A retention the protocol does not document is refused rather than sent and
// silently served at some other length.
func TestUnsupportedTTLIsRefused(t *testing.T) {
	_, err := NewClient(Config{
		Provider: config.LLMProviderAnthropic, APIKey: "k", Model: "m",
		CacheControl: config.CacheControl{Mode: config.CacheModeAuto, TTL: config.CacheTTL24h},
	})
	if err == nil {
		t.Fatal("expected 24h to be refused on a protocol that documents 5m and 1h")
	}
	for _, supported := range []string{config.CacheTTL5m, config.CacheTTL1h} {
		if _, err := NewClient(Config{
			Provider: config.LLMProviderAnthropic, APIKey: "k", Model: "m",
			CacheControl: config.CacheControl{Mode: config.CacheModeAuto, TTL: supported},
		}); err != nil {
			t.Errorf("ttl %q should be accepted on anthropic: %v", supported, err)
		}
	}
}

// An off policy that also names a retention is inert, not wrong. Refusing it
// would fail a configuration whose author had already said they want no
// caching, over a field that will never be sent.
func TestOffToleratesAnUnsupportedTTL(t *testing.T) {
	if _, err := NewClient(Config{
		Provider: config.LLMProviderOpenAICompatible, APIKey: "k", BaseURL: "http://localhost:1", Model: "m",
		CacheControl: config.CacheControl{Mode: config.CacheModeOff, TTL: config.CacheTTL1h},
	}); err != nil {
		t.Errorf("off with an inert ttl should be accepted: %v", err)
	}
}
