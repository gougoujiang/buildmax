package llmgateway_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

// teamPolicies is a PolicySource with per-team policies, so tests can express
// a team that has no policy at all.
type teamPolicies map[string]llmgateway.TeamPolicy

func (p teamPolicies) PolicyForTeam(_ context.Context, teamID string) (llmgateway.TeamPolicy, error) {
	if teamID == "" {
		return llmgateway.TeamPolicy{}, llmgateway.ErrTeamRequired
	}
	return p[teamID], nil
}

// failingCatalog reports a store-level failure, distinct from a missing target.
type failingCatalog struct{ err error }

func (c failingCatalog) Target(context.Context, string) (llmgateway.Target, error) {
	return llmgateway.Target{}, c.err
}

func testResolver(t *testing.T) *llmgateway.Resolver {
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

	return &llmgateway.Resolver{
		Catalog: catalog,
		Policies: teamPolicies{
			"tm_one": {
				DefaultAlias: "default",
				Aliases: map[string]string{
					"default":  "mt_fast",
					"deep":     "mt_deep",
					"retired":  "mt_retired",
					"dangling": "mt_gone",
				},
			},
			"tm_no_default": {
				Aliases: map[string]string{"fast": "mt_fast"},
			},
		},
	}
}

func TestResolveAppliesTeamDefault(t *testing.T) {
	resolver := testResolver(t)

	got, err := resolver.Resolve(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Alias != "default" {
		t.Errorf("Alias = %q, want %q", got.Alias, "default")
	}
	if got.Target.ID != "mt_fast" {
		t.Errorf("Target.ID = %q, want %q", got.Target.ID, "mt_fast")
	}
	if got.Target.UpstreamModel != "vendor/fast-1" {
		t.Errorf("Target.UpstreamModel = %q, want %q", got.Target.UpstreamModel, "vendor/fast-1")
	}
}

func TestResolveNamedAlias(t *testing.T) {
	resolver := testResolver(t)

	got, err := resolver.Resolve(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one", Alias: "deep"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Target.ID != "mt_deep" {
		t.Errorf("Target.ID = %q, want %q", got.Target.ID, "mt_deep")
	}
}

func TestResolveRejectsProviderModelIdentifier(t *testing.T) {
	// Aliases are the only model identifiers a managed client may submit. A
	// provider's own identifier must not resolve just because the catalog uses
	// it upstream.
	resolver := testResolver(t)

	for _, alias := range []string{"vendor/fast-1", "mt_fast"} {
		_, err := resolver.Resolve(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one", Alias: alias})
		if !errors.Is(err, llmgateway.ErrUnknownAlias) {
			t.Errorf("Resolve(%q): want ErrUnknownAlias, got %v", alias, err)
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
			name: "no team",
			req:  llmgateway.ResolveRequest{Alias: "deep"},
			want: llmgateway.ErrTeamRequired,
		},
		{
			name: "team without policy",
			req:  llmgateway.ResolveRequest{TeamID: "tm_unknown", Alias: "deep"},
			want: llmgateway.ErrTeamNotAuthorized,
		},
		{
			name: "unknown alias",
			req:  llmgateway.ResolveRequest{TeamID: "tm_one", Alias: "reasoning"},
			want: llmgateway.ErrUnknownAlias,
		},
		{
			name: "alias points at a missing target",
			req:  llmgateway.ResolveRequest{TeamID: "tm_one", Alias: "dangling"},
			want: llmgateway.ErrTargetNotFound,
		},
		{
			name: "disabled target",
			req:  llmgateway.ResolveRequest{TeamID: "tm_one", Alias: "retired"},
			want: llmgateway.ErrTargetDisabled,
		},
		{
			name: "no default alias to fall back on",
			req:  llmgateway.ResolveRequest{TeamID: "tm_no_default"},
			want: llmgateway.ErrNoDefaultAlias,
		},
		{
			name: "unsupported capability",
			req: llmgateway.ResolveRequest{
				TeamID:   "tm_one",
				Alias:    "deep",
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

func TestResolveCapabilityErrorCarriesDetail(t *testing.T) {
	resolver := testResolver(t)

	_, err := resolver.Resolve(context.Background(), llmgateway.ResolveRequest{
		TeamID: "tm_one",
		Alias:  "deep",
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
	if capErr.Alias != "deep" {
		t.Errorf("Alias = %q, want %q", capErr.Alias, "deep")
	}
	want := []llmgateway.Capability{llmgateway.CapabilityToolCalls, llmgateway.CapabilityStreamingText}
	if !slices.Equal(capErr.Missing, want) {
		t.Errorf("Missing = %v, want %v", capErr.Missing, want)
	}
	if !errors.Is(err, llmgateway.ErrCapabilityUnsupported) {
		t.Error("CapabilityError must classify as ErrCapabilityUnsupported")
	}
}

func TestResolveNotConfigured(t *testing.T) {
	var nilResolver *llmgateway.Resolver
	if _, err := nilResolver.Resolve(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"}); !errors.Is(err, llmgateway.ErrCatalogNotConfigured) {
		t.Errorf("nil resolver: want ErrCatalogNotConfigured, got %v", err)
	}

	noCatalog := &llmgateway.Resolver{Policies: teamPolicies{}}
	if _, err := noCatalog.Resolve(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"}); !errors.Is(err, llmgateway.ErrCatalogNotConfigured) {
		t.Errorf("no catalog: want ErrCatalogNotConfigured, got %v", err)
	}

	catalog, err := llmgateway.NewStaticCatalog([]llmgateway.Target{validTarget()})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	noPolicies := &llmgateway.Resolver{Catalog: catalog}
	if _, err := noPolicies.Resolve(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"}); !errors.Is(err, llmgateway.ErrPolicyNotConfigured) {
		t.Errorf("no policies: want ErrPolicyNotConfigured, got %v", err)
	}
	if _, err := noPolicies.Available(context.Background(), "tm_one"); !errors.Is(err, llmgateway.ErrPolicyNotConfigured) {
		t.Errorf("Available with no policies: want ErrPolicyNotConfigured, got %v", err)
	}
}

func TestAvailableListsOnlyUsableAliases(t *testing.T) {
	resolver := testResolver(t)

	models, err := resolver.Available(context.Background(), "tm_one")
	if err != nil {
		t.Fatalf("Available: %v", err)
	}

	var aliases []string
	for _, model := range models {
		aliases = append(aliases, model.Alias)
	}
	// "retired" is disabled and "dangling" has no catalog entry; both are
	// skipped so a partly retired catalog still lists.
	if want := []string{"deep", "default"}; !slices.Equal(aliases, want) {
		t.Fatalf("aliases = %v, want %v", aliases, want)
	}

	for _, model := range models {
		if model.Alias == "default" && !model.Default {
			t.Error("the team default alias is not marked as default")
		}
		if model.Alias == "deep" && model.Default {
			t.Error("a non-default alias is marked as default")
		}
	}
	if models[1].Name != "Fast" {
		t.Errorf("Name = %q, want %q", models[1].Name, "Fast")
	}
	if want := llmgateway.BaselineCapabilities(); len(models[1].Capabilities) != len(want) {
		t.Errorf("Capabilities = %v, want %d entries", models[1].Capabilities, len(want))
	}
}

func TestAvailableErrors(t *testing.T) {
	resolver := testResolver(t)

	if _, err := resolver.Available(context.Background(), ""); !errors.Is(err, llmgateway.ErrTeamRequired) {
		t.Errorf("want ErrTeamRequired, got %v", err)
	}
	if _, err := resolver.Available(context.Background(), "tm_unknown"); !errors.Is(err, llmgateway.ErrTeamNotAuthorized) {
		t.Errorf("want ErrTeamNotAuthorized, got %v", err)
	}

	// A catalog failure is not a missing target and must not be swallowed.
	boom := errors.New("catalog unavailable")
	failing := &llmgateway.Resolver{
		Catalog:  failingCatalog{err: boom},
		Policies: teamPolicies{"tm_one": {DefaultAlias: "default", Aliases: map[string]string{"default": "mt_fast"}}},
	}
	if _, err := failing.Available(context.Background(), "tm_one"); !errors.Is(err, boom) {
		t.Errorf("Available: want the catalog error, got %v", err)
	}
	if _, err := failing.Resolve(context.Background(), llmgateway.ResolveRequest{TeamID: "tm_one"}); !errors.Is(err, boom) {
		t.Errorf("Resolve: want the catalog error, got %v", err)
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
	disabled.Enabled = false

	catalog, err := llmgateway.NewStaticCatalog([]llmgateway.Target{target, disabled})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	resolver := &llmgateway.Resolver{
		Catalog: catalog,
		Policies: teamPolicies{
			"tm_one": {
				DefaultAlias: "default",
				Aliases: map[string]string{
					"default":  "mt_fast",
					"disabled": "mt_disabled",
					"dangling": "mt_gone",
				},
			},
		},
	}

	requests := []llmgateway.ResolveRequest{
		{Alias: "default"},
		{TeamID: "tm_unknown", Alias: "default"},
		{TeamID: "tm_one", Alias: "reasoning"},
		{TeamID: "tm_one", Alias: "dangling"},
		{TeamID: "tm_one", Alias: "disabled"},
		{TeamID: "tm_one", Alias: "default", Requires: []llmgateway.Capability{llmgateway.CapabilityToolCalls}},
	}
	for _, req := range requests {
		_, err := resolver.Resolve(context.Background(), req)
		if err == nil {
			t.Fatalf("Resolve(%+v) unexpectedly succeeded", req)
		}
		assertNoSentinel(t, fmt.Sprintf("error for alias %q", req.Alias), err.Error(), sentinels)
	}

	// Invalid-configuration errors are read by operators but land in startup
	// logs, so they carry an ID and never a credential.
	_, err = llmgateway.NewStaticCatalog([]llmgateway.Target{{
		ID:            "mt_broken",
		ProviderType:  llmgateway.ProviderOpenAICompatible,
		Endpoint:      secretEndpoint,
		CredentialRef: secretCredential,
	}})
	if err == nil {
		t.Fatal("an invalid catalog was accepted")
	}
	assertNoSentinel(t, "catalog validation error", err.Error(), sentinels)

	// A listing is the one place a client sees model data at all.
	models, err := resolver.Available(context.Background(), "tm_one")
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
