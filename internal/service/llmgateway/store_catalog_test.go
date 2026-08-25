package llmgateway_test

import (
	"context"
	"testing"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	coregw "github.com/gougoujiang/buildmax/internal/core/llmgateway"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

func storedModel(mutate func(*coregw.Model)) coregw.Model {
	m := coregw.Model{
		ID:           "lm_1",
		Name:         "fast",
		ProviderType: cllm.ProviderAnthropic,
		APIURL:       "https://api.anthropic.com",
		Model:        "claude-sonnet-5",
		Capabilities: []string{string(llmgateway.CapabilityTextChat)},
		Enabled:      true,
	}
	if mutate != nil {
		mutate(&m)
	}
	return m
}

// A row that names no policy takes the default rather than reading as an
// opt-out, and one that names a mode gets exactly that mode. There is no
// shorthand to fold in: BuildMax is pre-release, so `cache_mode` is the only
// way a catalog row states this.
func TestStoreCatalogResolvesTheCachePolicy(t *testing.T) {
	tests := []struct {
		name     string
		stored   coregw.Model
		wantMode string
		wantTTL  string
	}{
		{
			name:     "an unset row takes the default policy",
			stored:   storedModel(nil),
			wantMode: "",
		},
		{
			name:     "an explicit opt-out survives",
			stored:   storedModel(func(m *coregw.Model) { m.CacheMode = "off" }),
			wantMode: "off",
		},
		{
			name:     "force is carried through",
			stored:   storedModel(func(m *coregw.Model) { m.CacheMode = "force" }),
			wantMode: "force",
		},
		{
			name: "retention travels with the mode",
			stored: storedModel(func(m *coregw.Model) {
				m.CacheMode = "auto"
				m.CacheTTL = "1h"
			}),
			wantMode: "auto", wantTTL: "1h",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			catalog := &llmgateway.StoreCatalog{Models: &mock.MockLLMModelStore{Models: []coregw.Model{tc.stored}}}
			target, err := catalog.Target(context.Background(), "lm_1")
			if err != nil {
				t.Fatalf("Target: %v", err)
			}
			if target.CacheMode != tc.wantMode {
				t.Errorf("CacheMode = %q, want %q", target.CacheMode, tc.wantMode)
			}
			if target.CacheTTL != tc.wantTTL {
				t.Errorf("CacheTTL = %q, want %q", target.CacheTTL, tc.wantTTL)
			}
		})
	}
}
