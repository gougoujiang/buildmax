package llmgateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Resolution outcomes. Handlers map these to stable BuildMax error codes, so
// the wording here stays free of upstream endpoints, credentials, and provider
// model identifiers.
var (
	ErrCatalogNotConfigured  = errors.New("model catalog is not configured")
	ErrPolicyNotConfigured   = errors.New("team model policy is not configured")
	ErrTeamRequired          = errors.New("team is required")
	ErrTeamNotAuthorized     = errors.New("team has no model policy")
	ErrNoDefaultAlias        = errors.New("team model policy has no default alias")
	ErrUnknownAlias          = errors.New("model alias is not available to this team")
	ErrTargetNotFound        = errors.New("model target not found in catalog")
	ErrTargetDisabled        = errors.New("model target is disabled")
	ErrCapabilityUnsupported = errors.New("model target does not support a required capability")
	ErrInvalidCatalog        = errors.New("invalid model catalog")
	ErrInvalidPolicy         = errors.New("invalid team model policy")
)

// CapabilityError reports which required capabilities a target does not
// declare. It satisfies errors.Is(err, ErrCapabilityUnsupported) so callers can
// classify the failure without inspecting the detail.
type CapabilityError struct {
	Alias   string
	Missing []Capability
}

func (e *CapabilityError) Error() string {
	names := make([]string, 0, len(e.Missing))
	for _, capability := range e.Missing {
		names = append(names, string(capability))
	}
	return fmt.Sprintf("model alias %q does not support: %s", e.Alias, strings.Join(names, ", "))
}

// Is reports whether the error matches the ErrCapabilityUnsupported sentinel.
func (e *CapabilityError) Is(target error) bool { return target == ErrCapabilityUnsupported }

// ResolveRequest names the model a caller wants, in the only terms a managed
// client may use.
type ResolveRequest struct {
	// TeamID is derived from authentication, never from the request body.
	TeamID string
	// Alias is a stable team alias. Empty selects the team's default.
	Alias string
	// Requires is the capability set the call needs.
	Requires []Capability
}

// Resolution is a successful alias resolution.
type Resolution struct {
	// Alias is the alias actually used, with the default already applied.
	Alias string
	// Target is the operator-approved upstream to call.
	Target Target
}

// AvailableModel is one alias a team may use. It deliberately omits the
// endpoint, credential reference, provider type, and upstream model identifier:
// listing models must not disclose how the deployment reaches a provider.
type AvailableModel struct {
	Alias        string
	Name         string
	Capabilities []Capability
	Default      bool
}

// Resolver maps (team, alias) to an operator-approved target.
type Resolver struct {
	Catalog  Catalog
	Policies PolicySource
}

// Resolve returns the target for a request, applying the team default and
// rejecting anything the team is not permitted to use.
func (r *Resolver) Resolve(ctx context.Context, req ResolveRequest) (Resolution, error) {
	if r == nil || r.Catalog == nil {
		return Resolution{}, ErrCatalogNotConfigured
	}
	if r.Policies == nil {
		return Resolution{}, ErrPolicyNotConfigured
	}
	if req.TeamID == "" {
		return Resolution{}, ErrTeamRequired
	}

	policy, err := r.Policies.PolicyForTeam(ctx, req.TeamID)
	if err != nil {
		return Resolution{}, err
	}
	if policy.IsEmpty() {
		return Resolution{}, ErrTeamNotAuthorized
	}

	alias := req.Alias
	if alias == "" {
		alias = policy.DefaultAlias
	}
	if alias == "" {
		return Resolution{}, ErrNoDefaultAlias
	}

	targetID, ok := policy.TargetID(alias)
	if !ok {
		return Resolution{}, ErrUnknownAlias
	}

	target, err := r.Catalog.Target(ctx, targetID)
	if err != nil {
		return Resolution{}, err
	}
	if !target.Enabled {
		return Resolution{}, ErrTargetDisabled
	}
	if missing := target.Capabilities.Missing(req.Requires); len(missing) > 0 {
		return Resolution{}, &CapabilityError{Alias: alias, Missing: missing}
	}
	return Resolution{Alias: alias, Target: target}, nil
}

// Available lists the aliases a team may use, in a stable order.
//
// An alias whose target is missing or disabled is skipped rather than
// reported: a listing stays usable when part of the catalog is retired, while
// Resolve still fails loudly for a caller that asks for that alias by name.
func (r *Resolver) Available(ctx context.Context, teamID string) ([]AvailableModel, error) {
	if r == nil || r.Catalog == nil {
		return nil, ErrCatalogNotConfigured
	}
	if r.Policies == nil {
		return nil, ErrPolicyNotConfigured
	}
	if teamID == "" {
		return nil, ErrTeamRequired
	}

	policy, err := r.Policies.PolicyForTeam(ctx, teamID)
	if err != nil {
		return nil, err
	}
	if policy.IsEmpty() {
		return nil, ErrTeamNotAuthorized
	}

	var models []AvailableModel
	for _, alias := range policy.AliasNames() {
		targetID := policy.Aliases[alias]
		target, err := r.Catalog.Target(ctx, targetID)
		if errors.Is(err, ErrTargetNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !target.Enabled {
			continue
		}
		models = append(models, AvailableModel{
			Alias:        alias,
			Name:         target.Name,
			Capabilities: target.Capabilities.List(),
			Default:      alias == policy.DefaultAlias,
		})
	}
	return models, nil
}
