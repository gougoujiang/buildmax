package llmgateway_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

func validPolicy() llmgateway.TeamPolicy {
	return llmgateway.TeamPolicy{
		DefaultAlias: "default",
		Aliases: map[string]string{
			"default": "mt_fast",
			"fast":    "mt_fast",
			"deep":    "mt_deep",
		},
	}
}

func knownIDs() []string { return []string{"mt_fast", "mt_deep"} }

func TestValidatePolicyRejects(t *testing.T) {
	tests := []struct {
		name   string
		policy llmgateway.TeamPolicy
	}{
		{
			name:   "no aliases",
			policy: llmgateway.TeamPolicy{DefaultAlias: "default"},
		},
		{
			name: "alias maps to nothing",
			policy: llmgateway.TeamPolicy{
				DefaultAlias: "default",
				Aliases:      map[string]string{"default": ""},
			},
		},
		{
			name: "alias maps to unknown target",
			policy: llmgateway.TeamPolicy{
				DefaultAlias: "default",
				Aliases:      map[string]string{"default": "mt_gone"},
			},
		},
		{
			name: "empty alias name",
			policy: llmgateway.TeamPolicy{
				DefaultAlias: "default",
				Aliases:      map[string]string{"default": "mt_fast", "": "mt_fast"},
			},
		},
		{
			name: "no default alias",
			policy: llmgateway.TeamPolicy{
				Aliases: map[string]string{"fast": "mt_fast"},
			},
		},
		{
			name: "default alias is not granted",
			policy: llmgateway.TeamPolicy{
				DefaultAlias: "reasoning",
				Aliases:      map[string]string{"fast": "mt_fast"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := llmgateway.ValidatePolicy(tc.policy, knownIDs()); !errors.Is(err, llmgateway.ErrInvalidPolicy) {
				t.Fatalf("want ErrInvalidPolicy, got %v", err)
			}
			if _, err := llmgateway.NewStaticPolicySource(tc.policy, knownIDs()); !errors.Is(err, llmgateway.ErrInvalidPolicy) {
				t.Fatalf("NewStaticPolicySource: want ErrInvalidPolicy, got %v", err)
			}
		})
	}
}

func TestValidatePolicyAcceptsValid(t *testing.T) {
	if err := llmgateway.ValidatePolicy(validPolicy(), knownIDs()); err != nil {
		t.Fatalf("ValidatePolicy: %v", err)
	}
}

func TestStaticPolicySourceAppliesToEveryTeam(t *testing.T) {
	source, err := llmgateway.NewStaticPolicySource(validPolicy(), knownIDs())
	if err != nil {
		t.Fatalf("NewStaticPolicySource: %v", err)
	}

	for _, teamID := range []string{"tm_one", "tm_two"} {
		policy, err := source.PolicyForTeam(context.Background(), teamID)
		if err != nil {
			t.Fatalf("PolicyForTeam(%q): %v", teamID, err)
		}
		if policy.DefaultAlias != "default" {
			t.Errorf("DefaultAlias = %q, want %q", policy.DefaultAlias, "default")
		}
		if want := []string{"deep", "default", "fast"}; !slices.Equal(policy.AliasNames(), want) {
			t.Errorf("AliasNames() = %v, want %v", policy.AliasNames(), want)
		}
	}
}

func TestStaticPolicySourceRequiresTeam(t *testing.T) {
	source, err := llmgateway.NewStaticPolicySource(validPolicy(), knownIDs())
	if err != nil {
		t.Fatalf("NewStaticPolicySource: %v", err)
	}
	if _, err := source.PolicyForTeam(context.Background(), ""); !errors.Is(err, llmgateway.ErrTeamRequired) {
		t.Errorf("want ErrTeamRequired, got %v", err)
	}

	var nilSource *llmgateway.StaticPolicySource
	if _, err := nilSource.PolicyForTeam(context.Background(), "tm_one"); !errors.Is(err, llmgateway.ErrPolicyNotConfigured) {
		t.Errorf("want ErrPolicyNotConfigured, got %v", err)
	}
}

func TestStaticPolicySourceCopiesAliases(t *testing.T) {
	policy := validPolicy()
	source, err := llmgateway.NewStaticPolicySource(policy, knownIDs())
	if err != nil {
		t.Fatalf("NewStaticPolicySource: %v", err)
	}

	// Mutating the caller's map must not grant a team a new alias.
	policy.Aliases["backdoor"] = "mt_deep"

	stored, err := source.PolicyForTeam(context.Background(), "tm_one")
	if err != nil {
		t.Fatalf("PolicyForTeam: %v", err)
	}
	if _, ok := stored.TargetID("backdoor"); ok {
		t.Error("policy source aliased the caller's map")
	}
}

func TestTeamPolicyTargetID(t *testing.T) {
	policy := validPolicy()

	if id, ok := policy.TargetID("deep"); !ok || id != "mt_deep" {
		t.Errorf("TargetID(deep) = %q, %v", id, ok)
	}
	if _, ok := policy.TargetID("reasoning"); ok {
		t.Error("unknown alias resolved")
	}
	if _, ok := policy.TargetID(""); ok {
		t.Error("empty alias resolved")
	}
	if _, ok := (llmgateway.TeamPolicy{}).TargetID("fast"); ok {
		t.Error("the zero policy granted an alias")
	}
	if !(llmgateway.TeamPolicy{}).IsEmpty() {
		t.Error("the zero policy must be empty")
	}
}
