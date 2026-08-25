package llm

import (
	"strings"
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
	anthropic := cacheCapabilityFor(cllm.ProviderAnthropic)
	responses := cacheCapabilityFor(cllm.ProviderOpenAI)
	compatible := cacheCapabilityFor(cllm.ProviderOpenAICompatible)

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
			// Responses caches on its own, but still takes a scoped key that
			// says which bucket the prefix belongs in.
			name:       "responses scopes an agent turn",
			policy:     config.CacheControl{Mode: config.CacheModeAuto},
			capability: responses, profile: cllm.ProfileAgentTurn, wantSend: true,
		},
		{
			name:       "responses leaves a title unscoped",
			policy:     config.CacheControl{Mode: config.CacheModeAuto},
			capability: responses, profile: cllm.ProfileTitle,
		},
		{
			name:       "responses takes its own extended retention",
			policy:     config.CacheControl{Mode: config.CacheModeAuto, TTL: config.CacheTTL24h},
			capability: responses, profile: cllm.ProfileAgentTurn, wantSend: true, wantTTL: config.CacheTTL24h,
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
		cllm.ProviderAnthropic:        {cacheCapabilitySupported, true},
		cllm.ProviderOpenAI:           {cacheCapabilitySupported, true},
		cllm.ProviderOpenAICompatible: {cacheCapabilityUnsupported, false},
		cllm.ProviderOllama:           {cacheCapabilityUnsupported, false},
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
	for _, provider := range []string{cllm.ProviderOpenAICompatible, cllm.ProviderOllama} {
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
		Provider: cllm.ProviderAnthropic, APIKey: "k", Model: "m",
		CacheControl: config.CacheControl{Mode: config.CacheModeAuto, TTL: config.CacheTTL24h},
	})
	if err == nil {
		t.Fatal("expected 24h to be refused on a protocol that documents 5m and 1h")
	}
	for _, supported := range []string{config.CacheTTL5m, config.CacheTTL1h} {
		if _, err := NewClient(Config{
			Provider: cllm.ProviderAnthropic, APIKey: "k", Model: "m",
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
		Provider: cllm.ProviderOpenAICompatible, APIKey: "k", BaseURL: "http://localhost:1", Model: "m",
		CacheControl: config.CacheControl{Mode: config.CacheModeOff, TTL: config.CacheTTL1h},
	}); err != nil {
		t.Errorf("off with an inert ttl should be accepted: %v", err)
	}
}

// Retention vocabulary is per protocol, not global. Anthropic documents 5m and
// 1h; the Responses API documents 24h. Sending one protocol's window to the
// other would ask for a retention it has never heard of, so each refuses the
// other's rather than passing it through to be ignored.
func TestRetentionIsRefusedOutsideItsOwnProtocol(t *testing.T) {
	tests := []struct {
		provider  string
		supported []string
		refused   []string
	}{
		{
			provider:  cllm.ProviderAnthropic,
			supported: []string{config.CacheTTL5m, config.CacheTTL1h},
			refused:   []string{config.CacheTTL24h},
		},
		{
			provider:  cllm.ProviderOpenAI,
			supported: []string{config.CacheTTL24h},
			refused:   []string{config.CacheTTL5m, config.CacheTTL1h},
		},
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			for _, ttl := range tc.supported {
				if _, err := NewClient(Config{
					Provider: tc.provider, APIKey: "k", BaseURL: "http://localhost:1", Model: "m",
					CacheControl: config.CacheControl{Mode: config.CacheModeAuto, TTL: ttl},
				}); err != nil {
					t.Errorf("ttl %q should be accepted: %v", ttl, err)
				}
			}
			for _, ttl := range tc.refused {
				if _, err := NewClient(Config{
					Provider: tc.provider, APIKey: "k", BaseURL: "http://localhost:1", Model: "m",
					CacheControl: config.CacheControl{Mode: config.CacheModeAuto, TTL: ttl},
				}); err == nil {
					t.Errorf("ttl %q should be refused on %s", ttl, tc.provider)
				}
			}
		})
	}
}

// The acceptance criterion from docs/design/prompt-cache-control.md section 9,
// phase 4, made executable: no endpoint is described as cache-capable until it
// passes the qualification suite.
//
// The registry is empty on evidence. OpenRouter has been through
// `./make cache-qualify` on all three of its endpoints: every upstream that
// caches implicitly reports its reads over Chat Completions, Anthropic reports
// nothing there because it caches nothing implicitly, and the same model
// qualifies outright over the Messages endpoint. One `openrouter` entry would
// have to speak for both halves of that.
//
// This test is what stands between "we could add openrouter here" and doing it.
// Adding a profile means a qualification result covering everything the entry
// would speak for, and this test updated to name it.
func TestNoCompatibleGatewayIsQualifiedYet(t *testing.T) {
	if len(compatibleProfiles) != 0 {
		t.Errorf("compatibleProfiles has %d entries; each one needs a `%s` run behind it, "+
			"and this test updated to name the gateways that passed",
			len(compatibleProfiles), "./make cache-qualify")
	}
}

// An integration nobody has qualified is refused at construction rather than
// quietly downgraded. An operator who named a gateway expects its behaviour;
// handing them the unqualified default would leave them believing they had
// opted into something.
func TestUnknownIntegrationIsRefused(t *testing.T) {
	_, err := NewClient(Config{
		Provider: cllm.ProviderOpenAICompatible, APIKey: "k", BaseURL: "http://localhost:1",
		Model: "m", Integration: "openrouter",
	})
	if err == nil {
		t.Fatal("an unqualified gateway integration was accepted")
	}
	if !strings.Contains(err.Error(), "openrouter") {
		t.Errorf("error %q does not name the integration that was wrong", err)
	}
}

// An integration on a protocol that declares its own cache behaviour is a
// mistake, not an override: Anthropic and Responses are not gateways whose
// quirks need naming, and accepting one would suggest it changed something.
func TestIntegrationIsRefusedOnANativeProtocol(t *testing.T) {
	for _, provider := range []string{cllm.ProviderAnthropic, cllm.ProviderOpenAI} {
		t.Run(provider, func(t *testing.T) {
			if _, err := NewClient(Config{
				Provider: provider, APIKey: "k", BaseURL: "http://localhost:1",
				Model: "m", Integration: "somegateway",
			}); err == nil {
				t.Error("an integration was accepted on a protocol that declares its own behaviour")
			}
		})
	}
}

// An entry that names no integration is the normal case and must keep working.
func TestNoIntegrationIsTheNormalCase(t *testing.T) {
	for _, provider := range []string{
		cllm.ProviderOpenAICompatible, cllm.ProviderAnthropic,
		cllm.ProviderOpenAI, cllm.ProviderOllama,
	} {
		t.Run(provider, func(t *testing.T) {
			if _, err := NewClient(Config{
				Provider: provider, APIKey: "k", BaseURL: "http://localhost:1", Model: "m",
			}); err != nil {
				t.Errorf("an entry with no integration was refused: %v", err)
			}
		})
	}
}
