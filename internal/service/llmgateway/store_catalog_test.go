package llmgateway_test

import (
	"context"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

func storedModel(mutate func(*model.LLMModel)) model.LLMModel {
	m := model.LLMModel{
		ID:           "lm_1",
		Name:         "fast",
		ProviderType: llmgateway.ProviderAnthropic,
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

// A catalog row written before the structured policy existed still has to mean
// something, and the two legacy values do not mean the same thing.
//
// True was an explicit request for caching, so it becomes the mode that asks on
// every call. False is the column's own default — nobody had to choose it — so
// it is left unset and takes the default policy. Reading that default as an
// opt-out would leave every existing managed target uncached forever, which no
// operator asked for either.
func TestStoreCatalogResolvesTheLegacyPromptCacheColumn(t *testing.T) {
	tests := []struct {
		name     string
		stored   model.LLMModel
		wantMode string
		wantTTL  string
	}{
		{
			name:     "an unset row takes the default policy",
			stored:   storedModel(nil),
			wantMode: "",
		},
		{
			name:     "the legacy true becomes force",
			stored:   storedModel(func(m *model.LLMModel) { m.PromptCache = true }),
			wantMode: "force",
		},
		{
			name: "a structured policy wins over the shorthand",
			stored: storedModel(func(m *model.LLMModel) {
				m.PromptCache = true
				m.CacheMode = "off"
			}),
			wantMode: "off",
		},
		{
			name: "retention travels with the mode",
			stored: storedModel(func(m *model.LLMModel) {
				m.CacheMode = "auto"
				m.CacheTTL = "1h"
			}),
			wantMode: "auto", wantTTL: "1h",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			catalog := &llmgateway.StoreCatalog{Models: &mock.MockLLMModelStore{Models: []model.LLMModel{tc.stored}}}
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
