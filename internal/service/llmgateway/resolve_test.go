package llmgateway_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

// failingCatalog reports a store-level failure, distinct from a missing target.
type failingCatalog struct{ err error }

func (c failingCatalog) Target(context.Context, string) (llmgateway.Target, error) {
	return llmgateway.Target{}, c.err
}

func (c failingCatalog) TargetByName(context.Context, string) (llmgateway.Target, error) {
	return llmgateway.Target{}, c.err
}

func (c failingCatalog) List(context.Context) ([]llmgateway.Target, error) {
	return nil, c.err
}

// testCatalog holds Fast (full capabilities), Deep (text chat only), and
// Retired (disabled). NewStaticCatalog orders by ID, so Deep sorts first.
func testCatalog(t *testing.T) llmgateway.Catalog {
	t.Helper()

	deep := validTarget()
	deep.ID = "mt_deep"
	deep.Name = "Deep"
	deep.UpstreamModel = "vendor/deep-1"
	deep.Capabilities = llmgateway.NewCapabilitySet(llmgateway.CapabilityTextChat)

	retired := validTarget()
	retired.ID = "mt_retired"
	retired.Name = "Retired"
	retired.Enabled = false

	catalog, err := llmgateway.NewStaticCatalog([]llmgateway.Target{validTarget(), deep, retired})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	return catalog
}

func testResolver(t *testing.T) *llmgateway.Resolver {
	t.Helper()
	return &llmgateway.Resolver{Catalog: testCatalog(t), DefaultModel: "Fast"}
}

func TestResolveAppliesConfiguredDefault(t *testing.T) {
	resolver := testResolver(t)

	got, err := resolver.Resolve(context.Background(), llmgateway.ResolveRequest{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Name != "Fast" {
		t.Errorf("Name = %q, want %q", got.Name, "Fast")
	}
	if got.Target.ID != "mt_fast" {
		t.Errorf("Target.ID = %q, want %q", got.Target.ID, "mt_fast")
	}
	if got.Target.UpstreamModel != "vendor/fast-1" {
		t.Errorf("Target.UpstreamModel = %q, want %q", got.Target.UpstreamModel, "vendor/fast-1")
	}
}

// A deployment that configures no default is the single-model case, and it must
// work without configuration.
func TestResolveFallsBackToFirstEnabledModel(t *testing.T) {
	resolver := &llmgateway.Resolver{Catalog: testCatalog(t)}

	got, err := resolver.Resolve(context.Background(), llmgateway.ResolveRequest{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	// Ordered by ID, so mt_deep comes before mt_fast.
	if got.Name != "Deep" {
		t.Errorf("Name = %q, want %q", got.Name, "Deep")
	}
}

// The fallback skips a disabled row rather than picking one that cannot serve.
func TestResolveFallbackSkipsDisabled(t *testing.T) {
	retired := validTarget()
	retired.ID = "mt_aaa_retired"
	retired.Name = "Retired"
	retired.Enabled = false

	catalog, err := llmgateway.NewStaticCatalog([]llmgateway.Target{retired, validTarget()})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	resolver := &llmgateway.Resolver{Catalog: catalog}

	got, err := resolver.Resolve(context.Background(), llmgateway.ResolveRequest{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Name != "Fast" {
		t.Errorf("Name = %q, want the first enabled model %q", got.Name, "Fast")
	}
}

func TestResolveNamedModel(t *testing.T) {
	resolver := testResolver(t)

	got, err := resolver.Resolve(context.Background(), llmgateway.ResolveRequest{Name: "Deep"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Target.ID != "mt_deep" {
		t.Errorf("Target.ID = %q, want %q", got.Target.ID, "mt_deep")
	}
}

// The catalog name is the only model identifier a client may submit. Neither
// the provider's own identifier nor the catalog ID resolves, so a client cannot
// address a model by something the operator did not publish.
func TestResolveRejectsProviderModelIdentifierAndCatalogID(t *testing.T) {
	resolver := testResolver(t)

	for _, name := range []string{"vendor/fast-1", "mt_fast"} {
		_, err := resolver.Resolve(context.Background(), llmgateway.ResolveRequest{Name: name})
		if !errors.Is(err, llmgateway.ErrTargetNotFound) {
			t.Errorf("Resolve(%q): want ErrTargetNotFound, got %v", name, err)
		}
	}
}

func TestResolveErrors(t *testing.T) {
	resolver := testResolver(t)

	tests := []struct {
		name string
		req  llmgateway.ResolveRequest
		want error
	}{
		{
			name: "unknown model",
			req:  llmgateway.ResolveRequest{Name: "Reasoning"},
			want: llmgateway.ErrTargetNotFound,
		},
		{
			name: "disabled model",
			req:  llmgateway.ResolveRequest{Name: "Retired"},
			want: llmgateway.ErrTargetDisabled,
		},
		{
			name: "unsupported capability",
			req: llmgateway.ResolveRequest{
				Name:     "Deep",
				Requires: []llmgateway.Capability{llmgateway.CapabilityToolCalls},
			},
			want: llmgateway.ErrCapabilityUnsupported,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolver.Resolve(context.Background(), tc.req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Resolve: want %v, got %v", tc.want, err)
			}
		})
	}
}

// A configured default that names nothing fails rather than quietly serving a
// different model: the operator stated which one this deployment answers with.
func TestResolveConfiguredDefaultMustExist(t *testing.T) {
	resolver := &llmgateway.Resolver{Catalog: testCatalog(t), DefaultModel: "Gone"}

	_, err := resolver.Resolve(context.Background(), llmgateway.ResolveRequest{})
	if !errors.Is(err, llmgateway.ErrTargetNotFound) {
		t.Errorf("want ErrTargetNotFound, got %v", err)
	}
}

func TestResolveEmptyCatalog(t *testing.T) {
	retired := validTarget()
	retired.Enabled = false
	catalog, err := llmgateway.NewStaticCatalog([]llmgateway.Target{retired})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	resolver := &llmgateway.Resolver{Catalog: catalog}

	if _, err := resolver.Resolve(context.Background(), llmgateway.ResolveRequest{}); !errors.Is(err, llmgateway.ErrCatalogEmpty) {
		t.Errorf("want ErrCatalogEmpty, got %v", err)
	}
}

func TestResolveCapabilityErrorCarriesDetail(t *testing.T) {
	resolver := testResolver(t)

	_, err := resolver.Resolve(context.Background(), llmgateway.ResolveRequest{
		Name: "Deep",
		Requires: []llmgateway.Capability{
			llmgateway.CapabilityTextChat,
			llmgateway.CapabilityToolCalls,
			llmgateway.CapabilityStreamingText,
		},
	})

	var capErr *llmgateway.CapabilityError
	if !errors.As(err, &capErr) {
		t.Fatalf("want *CapabilityError, got %T: %v", err, err)
	}
	if capErr.Model != "Deep" {
		t.Errorf("Model = %q, want %q", capErr.Model, "Deep")
	}
	want := []llmgateway.Capability{llmgateway.CapabilityToolCalls, llmgateway.CapabilityStreamingText}
	if !slices.Equal(capErr.Missing, want) {
		t.Errorf("Missing = %v, want %v", capErr.Missing, want)
	}
	if !errors.Is(err, llmgateway.ErrCapabilityUnsupported) {
		t.Error("CapabilityError must classify as ErrCapabilityUnsupported")
	}
}

// Server-owned inference names a catalog entry by ID, which is not addressing a
// client has.
func TestResolveTargetByID(t *testing.T) {
	resolver := testResolver(t)

	target, err := resolver.ResolveTargetByID(context.Background(), "mt_deep", []llmgateway.Capability{llmgateway.CapabilityTextChat})
	if err != nil {
		t.Fatalf("ResolveTargetByID: %v", err)
	}
	if target.UpstreamModel != "vendor/deep-1" {
		t.Errorf("UpstreamModel = %q, want %q", target.UpstreamModel, "vendor/deep-1")
	}
}

func TestResolveTargetByIDErrors(t *testing.T) {
	resolver := testResolver(t)

	tests := []struct {
		name     string
		targetID string
		requires []llmgateway.Capability
		want     error
	}{
		{name: "no id", want: llmgateway.ErrTargetNotFound},
		{name: "unknown id", targetID: "mt_gone", want: llmgateway.ErrTargetNotFound},
		{name: "disabled", targetID: "mt_retired", want: llmgateway.ErrTargetDisabled},
		{
			name:     "unsupported capability",
			targetID: "mt_deep",
			requires: []llmgateway.Capability{llmgateway.CapabilityToolCalls},
			want:     llmgateway.ErrCapabilityUnsupported,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := resolver.ResolveTargetByID(context.Background(), tc.targetID, tc.requires); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}

	var nilResolver *llmgateway.Resolver
	if _, err := nilResolver.ResolveTargetByID(context.Background(), "mt_fast", nil); !errors.Is(err, llmgateway.ErrCatalogNotConfigured) {
		t.Errorf("nil resolver: want ErrCatalogNotConfigured, got %v", err)
	}
}

func TestResolveNotConfigured(t *testing.T) {
	var nilResolver *llmgateway.Resolver
	if _, err := nilResolver.Resolve(context.Background(), llmgateway.ResolveRequest{}); !errors.Is(err, llmgateway.ErrCatalogNotConfigured) {
		t.Errorf("nil resolver: want ErrCatalogNotConfigured, got %v", err)
	}
	if _, err := nilResolver.Available(context.Background()); !errors.Is(err, llmgateway.ErrCatalogNotConfigured) {
		t.Errorf("nil resolver Available: want ErrCatalogNotConfigured, got %v", err)
	}

	noCatalog := &llmgateway.Resolver{}
	if _, err := noCatalog.Resolve(context.Background(), llmgateway.ResolveRequest{}); !errors.Is(err, llmgateway.ErrCatalogNotConfigured) {
		t.Errorf("no catalog: want ErrCatalogNotConfigured, got %v", err)
	}
}

func TestAvailableListsOnlyUsableModels(t *testing.T) {
	resolver := testResolver(t)

	models, err := resolver.Available(context.Background())
	if err != nil {
		t.Fatalf("Available: %v", err)
	}

	var names []string
	for _, model := range models {
		names = append(names, model.Name)
	}
	// "Retired" is disabled, so it is skipped and a partly retired catalog
	// still lists. Ordering follows the catalog's own, by ID.
	if want := []string{"Deep", "Fast"}; !slices.Equal(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}

	for _, model := range models {
		if model.Name == "Fast" && !model.Default {
			t.Error("the configured default is not marked as default")
		}
		if model.Name == "Deep" && model.Default {
			t.Error("a non-default model is marked as default")
		}
	}
	if want := llmgateway.BaselineCapabilities(); len(models[1].Capabilities) != len(want) {
		t.Errorf("Capabilities = %v, want %d entries", models[1].Capabilities, len(want))
	}
}

// With no configured default the listing marks the same model Resolve would
// pick, so a client reading the flag and a client sending no name agree.
func TestAvailableMarksTheFallbackDefault(t *testing.T) {
	resolver := &llmgateway.Resolver{Catalog: testCatalog(t)}

	models, err := resolver.Available(context.Background())
	if err != nil {
		t.Fatalf("Available: %v", err)
	}
	if len(models) == 0 || !models[0].Default || models[0].Name != "Deep" {
		t.Fatalf("models = %+v, want the first enabled model marked default", models)
	}
	for _, model := range models[1:] {
		if model.Default {
			t.Errorf("%q is also marked default", model.Name)
		}
	}
}

func TestAvailableErrors(t *testing.T) {
	// A catalog failure is not a missing target and must not be swallowed.
	boom := errors.New("catalog unavailable")
	failing := &llmgateway.Resolver{Catalog: failingCatalog{err: boom}}

	if _, err := failing.Available(context.Background()); !errors.Is(err, boom) {
		t.Errorf("Available: want the catalog error, got %v", err)
	}
	if _, err := failing.Resolve(context.Background(), llmgateway.ResolveRequest{Name: "Fast"}); !errors.Is(err, boom) {
		t.Errorf("Resolve: want the catalog error, got %v", err)
	}
	if _, err := failing.Resolve(context.Background(), llmgateway.ResolveRequest{}); !errors.Is(err, boom) {
		t.Errorf("Resolve with no name: want the catalog error, got %v", err)
	}
}

// TestNoUpstreamDetailEscapes is the guard for the credential-handling rule in
// docs/design/llm-gateway.md: resolution outcomes reach users and logs, so they
// must never carry the endpoint, the credential reference, or the provider's
// own model identifier.
func TestNoUpstreamDetailEscapes(t *testing.T) {
	const (
		secretEndpoint   = "https://SECRET-ENDPOINT.internal/v1"
		secretCredential = "SECRET-CREDENTIAL-REF"
		secretModel      = "SECRET-UPSTREAM-MODEL"
	)
	sentinels := []string{secretEndpoint, secretCredential, secretModel}

	target := validTarget()
	target.Endpoint = secretEndpoint
	target.CredentialRef = secretCredential
	target.UpstreamModel = secretModel
	target.Capabilities = llmgateway.NewCapabilitySet(llmgateway.CapabilityTextChat)

	disabled := target
	disabled.ID = "mt_disabled"
	disabled.Name = "Disabled"
	disabled.Enabled = false

	catalog, err := llmgateway.NewStaticCatalog([]llmgateway.Target{target, disabled})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	resolver := &llmgateway.Resolver{Catalog: catalog, DefaultModel: "Fast"}

	requests := []llmgateway.ResolveRequest{
		{Name: "Reasoning"},
		{Name: "Disabled"},
		{Name: secretModel},
		{Name: "Fast", Requires: []llmgateway.Capability{llmgateway.CapabilityToolCalls}},
	}
	for _, req := range requests {
		_, err := resolver.Resolve(context.Background(), req)
		if err == nil {
			t.Fatalf("Resolve(%+v) unexpectedly succeeded", req)
		}
		assertNoSentinel(t, fmt.Sprintf("error for model %q", req.Name), err.Error(), sentinels)
	}

	// Invalid-configuration errors are read by operators but land in startup
	// logs, so they carry an ID and never a credential.
	_, err = llmgateway.NewStaticCatalog([]llmgateway.Target{{
		ID:            "mt_broken",
		ProviderType:  cllm.ProviderOpenAICompatible,
		Endpoint:      secretEndpoint,
		CredentialRef: secretCredential,
	}})
	if err == nil {
		t.Fatal("an invalid catalog was accepted")
	}
	assertNoSentinel(t, "catalog validation error", err.Error(), sentinels)

	// A listing is the one place a client sees model data at all.
	models, err := resolver.Available(context.Background())
	if err != nil {
		t.Fatalf("Available: %v", err)
	}
	assertNoSentinel(t, "available models", fmt.Sprintf("%+v", models), sentinels)
}

func assertNoSentinel(t *testing.T, what, got string, sentinels []string) {
	t.Helper()
	for _, sentinel := range sentinels {
		if strings.Contains(got, sentinel) {
			t.Errorf("%s leaked upstream detail %q: %s", what, sentinel, got)
		}
	}
}
