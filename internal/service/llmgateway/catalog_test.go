package llmgateway_test

import (
	"context"
	"errors"
	"slices"
	"strings"
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
			name: "negative max tokens",
			targets: []llmgateway.Target{func() llmgateway.Target {
				target := validTarget()
				target.MaxTokens = -1
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
	second.Name = "Deep"
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

// A name is how a client addresses a model, so a duplicate would make the
// second target unreachable rather than merely redundant.
func TestNewStaticCatalogRejectsDuplicateNames(t *testing.T) {
	second := validTarget()
	second.ID = "mt_deep"

	_, err := llmgateway.NewStaticCatalog([]llmgateway.Target{validTarget(), second})
	if !errors.Is(err, llmgateway.ErrInvalidCatalog) {
		t.Fatalf("want ErrInvalidCatalog, got %v", err)
	}
	if !strings.Contains(err.Error(), "Fast") {
		t.Errorf("the error does not name the duplicate: %v", err)
	}
}

func TestStaticCatalogTargetByName(t *testing.T) {
	catalog, err := llmgateway.NewStaticCatalog([]llmgateway.Target{validTarget()})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}

	got, err := catalog.TargetByName(context.Background(), "Fast")
	if err != nil {
		t.Fatalf("TargetByName: %v", err)
	}
	if got.ID != "mt_fast" {
		t.Errorf("ID = %q, want %q", got.ID, "mt_fast")
	}
	if _, err := catalog.TargetByName(context.Background(), "Gone"); !errors.Is(err, llmgateway.ErrTargetNotFound) {
		t.Errorf("want ErrTargetNotFound, got %v", err)
	}
}

func TestNewStaticCatalogAcceptsEmpty(t *testing.T) {
	// An empty catalog is a valid deployment state: the gateway is simply not
	// offering managed models yet. A resolve against it fails with
	// ErrCatalogEmpty rather than the catalog refusing to exist.
	catalog, err := llmgateway.NewStaticCatalog(nil)
	if err != nil {
		t.Fatalf("NewStaticCatalog(nil): %v", err)
	}
	if ids := catalog.IDs(); ids != nil {
		t.Errorf("IDs() = %v, want nil", ids)
	}
}

// TestProviderNeedsCredential pins the one exemption the catalog has. Every
// hosted protocol authenticates, and a target missing its key must fail at
// selection rather than send an unauthenticated request upstream; a local
// runtime has no key to miss.
func TestProviderNeedsCredential(t *testing.T) {
	for _, provider := range []string{llmgateway.ProviderOpenAICompatible, llmgateway.ProviderOpenAI, llmgateway.ProviderAnthropic} {
		if !llmgateway.ProviderNeedsCredential(provider) {
			t.Errorf("provider %q authenticates and must need a credential", provider)
		}
	}
	if llmgateway.ProviderNeedsCredential(llmgateway.ProviderOllama) {
		t.Errorf("provider %q is a local runtime with no credential to hold", llmgateway.ProviderOllama)
	}
	// An unknown name is not an exemption: it reaches KnownProvider first, and
	// answering "needs none" here would be the wrong default if that changed.
	if !llmgateway.ProviderNeedsCredential("bedrock") {
		t.Error("an unrecognized provider should not be exempt")
	}
}

func TestProvidersAndKnownProviderAgree(t *testing.T) {
	for _, provider := range llmgateway.Providers() {
		if !llmgateway.KnownProvider(provider) {
			t.Errorf("Providers lists %q, which KnownProvider rejects", provider)
		}
	}
	if llmgateway.KnownProvider("") {
		t.Error("an empty provider is not known: a target must state its protocol")
	}
}
