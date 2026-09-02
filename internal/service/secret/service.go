// Package secret is the Team Secret lifecycle service: it validates a Secret's
// items, seals them through a Sealer, and stores metadata plus sealed bytes. It
// never returns an item value -- there is no reveal path -- and it owns the
// rule that an item name is an identifier. Authorization (owner-only) is the
// handler's job; team ownership is checked here. See docs/design/team-secrets.md.
package secret

import (
	"context"
	"errors"
	"maps"
	"sort"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coresecret "github.com/gougoujiang/buildmax/internal/core/secret"
)

var (
	ErrNameRequired = apierr.New(apierr.KindInvalid, "secret name required")
	ErrNoItems      = apierr.New(apierr.KindInvalid, "a secret needs at least one item")
	ErrInvalidItem  = apierr.New(apierr.KindInvalid, "an item name must be an identifier")
	ErrUnknownState = apierr.New(apierr.KindInvalid, "unknown secret state")
	ErrNotFound     = apierr.New(apierr.KindNotFound, "secret not found")
	ErrDestroyed    = apierr.New(apierr.KindConflict, "secret is destroyed")
	ErrItemNotFound = apierr.New(apierr.KindInvalid, "no such item to remove")
)

// Service owns Secret lifecycle. Store persists metadata and sealed bytes,
// Sealer does the cryptography, and DeploymentID is bound into the associated
// data so a ciphertext cannot be opened under another deployment.
type Service struct {
	Store        coresecret.Store
	Sealer       coresecret.Sealer
	DeploymentID string
}

// CreateCmd creates a Secret with its first items.
type CreateCmd struct {
	TeamID      string
	CreatedBy   string
	Name        string
	Description string
	Items       map[string]string
}

// Create validates and seals the items, then stores the Secret.
func (s *Service) Create(ctx context.Context, cmd CreateCmd) (*coresecret.Secret, error) {
	if strings.TrimSpace(cmd.Name) == "" {
		return nil, ErrNameRequired
	}
	names, err := validateItems(cmd.Items)
	if err != nil {
		return nil, err
	}
	sealed, err := s.Sealer.Seal(coresecret.Items(cmd.Items), coresecret.AAD(s.DeploymentID, cmd.TeamID))
	if err != nil {
		return nil, err
	}
	return s.Store.CreateSecret(ctx, coresecret.CreateInput{
		TeamID:      cmd.TeamID,
		Name:        cmd.Name,
		Description: cmd.Description,
		Provider:    coresecret.ProviderEmbedded,
		CreatedBy:   cmd.CreatedBy,
		ItemNames:   names,
		Sealed:      sealed,
	})
}

// List returns a team's Secrets, metadata only.
func (s *Service) List(ctx context.Context, teamID string) ([]coresecret.Secret, error) {
	return s.Store.ListSecretsByTeam(ctx, teamID)
}

// Get returns one Secret the team owns. A Secret in another team reads as
// not-found, so the answer does not confirm one exists elsewhere.
func (s *Service) Get(ctx context.Context, teamID, id string) (*coresecret.Secret, error) {
	sec, err := s.scoped(ctx, teamID, id)
	if err != nil {
		return nil, err
	}
	return sec, nil
}

// ReplaceItems sets the whole item map, the shape a raw-JSON editor sends.
func (s *Service) ReplaceItems(ctx context.Context, teamID, id string, items map[string]string) (*coresecret.Secret, error) {
	sec, err := s.scoped(ctx, teamID, id)
	if err != nil {
		return nil, err
	}
	if sec.State == coresecret.StateDestroyed {
		return nil, ErrDestroyed
	}
	names, err := validateItems(items)
	if err != nil {
		return nil, err
	}
	return s.seal(ctx, teamID, id, items, names)
}

// PatchItems sets some item keys and removes others, the shape a row editor
// sends. It decrypts the current items, applies the delta, and re-seals the
// whole map -- one atomic row rewrite, so a rotation of several items is
// consistent.
func (s *Service) PatchItems(ctx context.Context, teamID, id string, set map[string]string, remove []string) (*coresecret.Secret, error) {
	sec, sealed, err := s.scopedSealed(ctx, teamID, id)
	if err != nil {
		return nil, err
	}
	if sec.State == coresecret.StateDestroyed {
		return nil, ErrDestroyed
	}
	items, err := s.Sealer.Open(*sealed, coresecret.AAD(s.DeploymentID, sec.TeamID))
	if err != nil {
		return nil, err
	}
	merged := map[string]string(items)
	if merged == nil {
		merged = map[string]string{}
	}
	for _, k := range remove {
		if _, ok := merged[k]; !ok {
			return nil, ErrItemNotFound
		}
		delete(merged, k)
	}
	maps.Copy(merged, set)
	names, err := validateItems(merged)
	if err != nil {
		return nil, err
	}
	return s.seal(ctx, teamID, id, merged, names)
}

// SetState disables, re-enables, or destroys a Secret.
func (s *Service) SetState(ctx context.Context, teamID, id string, state coresecret.State) (*coresecret.Secret, error) {
	switch state {
	case coresecret.StateActive, coresecret.StateDisabled, coresecret.StateDestroyed:
	default:
		return nil, ErrUnknownState
	}
	if _, err := s.scoped(ctx, teamID, id); err != nil {
		return nil, err
	}
	return s.Store.SetState(ctx, id, state)
}

// seal re-seals items and stores them. Item names are pre-validated.
func (s *Service) seal(ctx context.Context, teamID, id string, items map[string]string, names []string) (*coresecret.Secret, error) {
	sealed, err := s.Sealer.Seal(coresecret.Items(items), coresecret.AAD(s.DeploymentID, teamID))
	if err != nil {
		return nil, err
	}
	return s.Store.UpdateItems(ctx, coresecret.UpdateItemsInput{ID: id, ItemNames: names, Sealed: sealed})
}

// scoped fetches a Secret and refuses one that is not the team's.
func (s *Service) scoped(ctx context.Context, teamID, id string) (*coresecret.Secret, error) {
	sec, err := s.Store.GetSecret(ctx, id)
	if err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if sec.TeamID != teamID {
		return nil, ErrNotFound
	}
	return sec, nil
}

func (s *Service) scopedSealed(ctx context.Context, teamID, id string) (*coresecret.Secret, *coresecret.Sealed, error) {
	sec, sealed, err := s.Store.GetSealed(ctx, id)
	if err != nil {
		if errors.Is(err, apierr.ErrNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	if sec.TeamID != teamID {
		return nil, nil, ErrNotFound
	}
	return sec, sealed, nil
}

// validateItems checks the map is non-empty and every name is an identifier,
// returning the sorted name set to store. Item names must be identifiers so a
// whole group can be injected as environment variables (§5.1, §6.2).
func validateItems(items map[string]string) ([]string, error) {
	if len(items) == 0 {
		return nil, ErrNoItems
	}
	names := make([]string, 0, len(items))
	for name := range items {
		if !coresecret.IsItemName(name) {
			return nil, ErrInvalidItem
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
