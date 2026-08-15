package llmgateway_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

func validTarget() llmgateway.Target {
	return llmgateway.Target{
		ID:            "mt_fast",
		Name:          "Fast",
		ProviderType:  llmgateway.ProviderOpenAICompatible,
		Endpoint:      "https://upstream.example.com/v1",
		CredentialRef: "upstream_key",
		UpstreamModel: "vendor/fast-1",
		ContextWindow: 128000,
		CallTimeout:   300 * time.Second,
		Capabilities:  llmgateway.NewCapabilitySet(llmgateway.BaselineCapabilities()...),
		Enabled:       true,
	}
}

func TestNewStaticCatalogRejectsInvalidTargets(t *testing.T) {
	tests := []struct {
		name    string
		targets []llmgateway.Target
	}{
		{
			name: "missing id",
			targets: []llmgateway.Target{func() llmgateway.Target {
				target := validTarget()
				target.ID = ""
				return target
			}()},
		},
		{
			name: "missing provider type",
			targets: []llmgateway.Target{func() llmgateway.Target {
				target := validTarget()
				target.ProviderType = ""
				return target
			}()},
		},
		{
			name: "missing endpoint",
			targets: []llmgateway.Target{func() llmgateway.Target {
				target := validTarget()
				target.Endpoint = ""
				return target
			}()},
		},
		{
			name: "missing upstream model",
			targets: []llmgateway.Target{func() llmgateway.Target {
				target := validTarget()
				target.UpstreamModel = ""
				return target
			}()},
		},
		{
			name: "no declared capabilities",
			targets: []llmgateway.Target{func() llmgateway.Target {
				target := validTarget()
				target.Capabilities = nil
				return target
			}()},
		},
		{
			name: "negative context window",
			targets: []llmgateway.Target{func() llmgateway.Target {
				target := validTarget()
				target.ContextWindow = -1
				return target
			}()},
		},
		{
			name: "negative call timeout",
			targets: []llmgateway.Target{func() llmgateway.Target {
				target := validTarget()
				target.CallTimeout = -time.Second
				return target
			}()},
		},
		{
			name:    "duplicate id",
			targets: []llmgateway.Target{validTarget(), validTarget()},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			catalog, err := llmgateway.NewStaticCatalog(tc.targets)
			if !errors.Is(err, llmgateway.ErrInvalidCatalog) {
				t.Fatalf("want ErrInvalidCatalog, got %v", err)
			}
			if catalog != nil {
				t.Error("a rejected catalog must not be returned")
			}
		})
	}
}

func TestStaticCatalogTarget(t *testing.T) {
	catalog, err := llmgateway.NewStaticCatalog([]llmgateway.Target{validTarget()})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}

	got, err := catalog.Target(context.Background(), "mt_fast")
	if err != nil {
		t.Fatalf("Target: %v", err)
	}
	if got.UpstreamModel != "vendor/fast-1" {
		t.Errorf("UpstreamModel = %q, want %q", got.UpstreamModel, "vendor/fast-1")
	}

	if _, err := catalog.Target(context.Background(), "mt_missing"); !errors.Is(err, llmgateway.ErrTargetNotFound) {
		t.Errorf("want ErrTargetNotFound, got %v", err)
	}
}

func TestStaticCatalogNilIsNotConfigured(t *testing.T) {
	var catalog *llmgateway.StaticCatalog
	if _, err := catalog.Target(context.Background(), "mt_fast"); !errors.Is(err, llmgateway.ErrCatalogNotConfigured) {
		t.Errorf("want ErrCatalogNotConfigured, got %v", err)
	}
	if ids := catalog.IDs(); ids != nil {
		t.Errorf("IDs() = %v, want nil", ids)
	}
}

func TestStaticCatalogIDsAreStableAndCopied(t *testing.T) {
	second := validTarget()
	second.ID = "mt_deep"
	catalog, err := llmgateway.NewStaticCatalog([]llmgateway.Target{validTarget(), second})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}

	want := []string{"mt_deep", "mt_fast"}
	ids := catalog.IDs()
	if !slices.Equal(ids, want) {
		t.Fatalf("IDs() = %v, want %v", ids, want)
	}

	ids[0] = "mutated"
	if again := catalog.IDs(); !slices.Equal(again, want) {
		t.Errorf("IDs() returned an aliased slice: %v", again)
	}
}

func TestNewStaticCatalogAcceptsEmpty(t *testing.T) {
	// An empty catalog is a valid deployment state: the gateway is simply not
	// offering managed models yet. Policy validation is what rejects an alias
	// with nothing behind it.
	catalog, err := llmgateway.NewStaticCatalog(nil)
	if err != nil {
		t.Fatalf("NewStaticCatalog(nil): %v", err)
	}
	if ids := catalog.IDs(); ids != nil {
		t.Errorf("IDs() = %v, want nil", ids)
	}
}
