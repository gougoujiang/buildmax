package db

import (
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/core/model"
	pluginsvc "github.com/gougoujiang/buildmax/internal/service/plugin"
)

// The store is the only implementation of the activation contract, so a
// mismatch should fail here rather than when the routes are wired.
var _ pluginsvc.ActivationStore = (*Store)(nil)

func TestToPluginActivationCarriesThePin(t *testing.T) {
	updatedBy := "gsyt7at6cjfr33d73mtb"
	got := toPluginActivation(&pluginActivationReadRow{
		Row: pluginActivationRow{
			PublicID:   "gsyt7at6cjfr33d73mta",
			PluginName: "code-review",
			Version:    "1.2.0",
			Digest:     "sha256:abc",
			Enabled:    true,
			Origin:     string(model.PluginActivationAutomatic),
		},
		TeamPublicID:        "gsyt7at6cjfr33d73mtc",
		ActivatedByPublicID: "gsyt7at6cjfr33d73mtd",
		UpdatedByPublicID:   &updatedBy,
	})
	if got.Version != "1.2.0" || got.Digest != "sha256:abc" {
		t.Errorf("the pin did not survive conversion: %+v", got)
	}
	if got.Origin != model.PluginActivationAutomatic {
		t.Errorf("origin = %q, want automatic", got.Origin)
	}
	if got.TeamID == "" || got.ActivatedBy == "" || got.UpdatedBy != updatedBy {
		t.Errorf("a handle was dropped: %+v", got)
	}
}

// An activation that has never been changed has no updated_by handle to read.
func TestToPluginActivationToleratesNoUpdater(t *testing.T) {
	got := toPluginActivation(&pluginActivationReadRow{
		Row:                 pluginActivationRow{PluginName: "code-review", Version: "1.0.0"},
		ActivatedByPublicID: "gsyt7at6cjfr33d73mtd",
	})
	if got.UpdatedBy != "" {
		t.Errorf("UpdatedBy = %q, want empty", got.UpdatedBy)
	}
}

func TestPluginActivationLifecycle(t *testing.T) {
	s, ctx := newTestStore(t)
	owner := newTestUser(t, s, "activation-owner")
	team, err := s.CreateTeam(ctx, "activation team", owner, "")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	activated, err := s.ActivatePlugin(ctx, model.ActivatePluginInput{
		TeamID:     team.ID,
		PluginName: "code-review",
		Version:    "1.0.0",
		Digest:     "sha256:one",
		Origin:     model.PluginActivationCurated,
		ActorID:    owner,
	})
	if err != nil {
		t.Fatalf("ActivatePlugin: %v", err)
	}
	if activated.ID == "" || !activated.Enabled {
		t.Fatalf("unexpected activation: %+v", activated)
	}

	// One row per team and plugin: a second activation is a pin move.
	if _, err := s.ActivatePlugin(ctx, model.ActivatePluginInput{
		TeamID: team.ID, PluginName: "code-review", Version: "2.0.0",
		Digest: "sha256:two", Origin: model.PluginActivationCurated, ActorID: owner,
	}); !errors.Is(err, model.ErrPluginAlreadyActivated) {
		t.Fatalf("second activation err = %v, want ErrPluginAlreadyActivated", err)
	}

	moved, err := s.MovePluginActivationPin(ctx, model.MovePluginActivationPinInput{
		TeamID: team.ID, PluginName: "code-review", Version: "2.0.0",
		Digest: "sha256:two", ActorID: owner,
	})
	if err != nil {
		t.Fatalf("MovePluginActivationPin: %v", err)
	}
	if moved.Version != "2.0.0" || moved.Digest != "sha256:two" {
		t.Errorf("pin did not move: %+v", moved)
	}

	// Suspension keeps the pin; that is why it is a flag and not a delete.
	suspended, err := s.SetPluginActivationEnabled(ctx, team.ID, "code-review", false, owner)
	if err != nil {
		t.Fatalf("SetPluginActivationEnabled: %v", err)
	}
	if suspended.Enabled || suspended.Version != "2.0.0" {
		t.Errorf("suspension lost the pin: %+v", suspended)
	}

	// Suspending an already suspended activation is not "not found".
	if _, err := s.SetPluginActivationEnabled(ctx, team.ID, "code-review", false, owner); err != nil {
		t.Fatalf("re-suspend: %v", err)
	}

	listed, err := s.ListPluginActivations(ctx, team.ID)
	if err != nil {
		t.Fatalf("ListPluginActivations: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("got %d activations, want 1 — a suspended one still explains a failed run", len(listed))
	}

	if _, err := s.MovePluginActivationPin(ctx, model.MovePluginActivationPinInput{
		TeamID: team.ID, PluginName: "absent", Version: "1.0.0", Digest: "sha256:x", ActorID: owner,
	}); !errors.Is(err, apierr.ErrNotFound) {
		t.Fatalf("moving an absent activation err = %v, want ErrNotFound", err)
	}
}

func TestSetTeamPluginCurationRoundTrips(t *testing.T) {
	s, ctx := newTestStore(t)
	owner := newTestUser(t, s, "curation-owner")
	team, err := s.CreateTeam(ctx, "curation team", owner, "")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if team.PluginCuration != model.PluginCurationOpen {
		t.Errorf("a new team's mode = %q, want open by default", team.PluginCuration)
	}

	if err := s.SetTeamPluginCuration(ctx, team.ID, model.PluginCurationCurated); err != nil {
		t.Fatalf("SetTeamPluginCuration: %v", err)
	}
	got, err := s.GetTeam(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if got.PluginCuration != model.PluginCurationCurated {
		t.Errorf("mode = %q, want curated", got.PluginCuration)
	}
}
