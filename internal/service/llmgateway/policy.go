package llmgateway

import (
	"context"
	"fmt"
	"maps"
	"slices"
)

// TeamPolicy is the set of model aliases available to one team.
//
// An alias is the only model identifier a managed client may submit. Mapping
// aliases to catalog IDs here keeps upstream endpoints, credentials, and
// provider model identifiers inside the team authorization boundary.
type TeamPolicy struct {
	// DefaultAlias is used when a request names no alias.
	DefaultAlias string
	// Aliases maps a stable alias such as "fast" to a catalog target ID.
	Aliases map[string]string
}

// TargetID returns the catalog ID an alias maps to.
func (p TeamPolicy) TargetID(alias string) (string, bool) {
	if alias == "" || len(p.Aliases) == 0 {
		return "", false
	}
	id, ok := p.Aliases[alias]
	return id, ok
}

// AliasNames returns the team's aliases in a stable order.
func (p TeamPolicy) AliasNames() []string {
	if len(p.Aliases) == 0 {
		return nil
	}
	out := make([]string, 0, len(p.Aliases))
	for alias := range p.Aliases {
		out = append(out, alias)
	}
	slices.Sort(out)
	return out
}

// IsEmpty reports whether the policy grants access to nothing.
func (p TeamPolicy) IsEmpty() bool { return len(p.Aliases) == 0 }

// PolicySource provides the model policy for a team. Implementations return an
// empty policy, not an error, when a team has no policy: that is an
// authorization outcome, not a failure.
type PolicySource interface {
	PolicyForTeam(ctx context.Context, teamID string) (TeamPolicy, error)
}

// StaticPolicySource applies one operator-defined policy to every team. It is
// the deployment-wide default that lets a private deployment expose an
// approved catalog before per-team policy is stored in the database.
type StaticPolicySource struct {
	policy TeamPolicy
}

// NewStaticPolicySource validates the policy against the known catalog IDs and
// builds the source. Passing the catalog IDs is what turns a dangling alias
// into a startup error instead of a failed user call.
func NewStaticPolicySource(policy TeamPolicy, knownTargetIDs []string) (*StaticPolicySource, error) {
	if err := ValidatePolicy(policy, knownTargetIDs); err != nil {
		return nil, err
	}
	aliases := make(map[string]string, len(policy.Aliases))
	maps.Copy(aliases, policy.Aliases)
	return &StaticPolicySource{policy: TeamPolicy{DefaultAlias: policy.DefaultAlias, Aliases: aliases}}, nil
}

// PolicyForTeam returns the same policy for every team.
func (s *StaticPolicySource) PolicyForTeam(_ context.Context, teamID string) (TeamPolicy, error) {
	if s == nil {
		return TeamPolicy{}, ErrPolicyNotConfigured
	}
	if teamID == "" {
		return TeamPolicy{}, ErrTeamRequired
	}
	return s.policy, nil
}

// ValidatePolicy reports whether every alias points at a known target and the
// default alias is one the policy actually grants.
func ValidatePolicy(policy TeamPolicy, knownTargetIDs []string) error {
	if policy.IsEmpty() {
		return fmt.Errorf("%w: no aliases", ErrInvalidPolicy)
	}
	if _, blank := policy.Aliases[""]; blank {
		return fmt.Errorf("%w: an alias name is empty", ErrInvalidPolicy)
	}
	known := make(map[string]struct{}, len(knownTargetIDs))
	for _, id := range knownTargetIDs {
		known[id] = struct{}{}
	}
	for _, alias := range policy.AliasNames() {
		id := policy.Aliases[alias]
		if id == "" {
			return fmt.Errorf("%w: alias %q maps to no target", ErrInvalidPolicy, alias)
		}
		if _, ok := known[id]; !ok {
			return fmt.Errorf("%w: alias %q maps to unknown target %q", ErrInvalidPolicy, alias, id)
		}
	}
	if policy.DefaultAlias == "" {
		return fmt.Errorf("%w: no default alias", ErrInvalidPolicy)
	}
	if _, ok := policy.Aliases[policy.DefaultAlias]; !ok {
		return fmt.Errorf("%w: default alias %q is not granted", ErrInvalidPolicy, policy.DefaultAlias)
	}
	return nil
}
