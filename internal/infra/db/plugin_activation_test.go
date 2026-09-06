package db

import (
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
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
			Origin:     string(coreplugin.ActivationAutomatic),
		},
		SpacePublicID:       "gsyt7at6cjfr33d73mtc",
		ActivatedByPublicID: "gsyt7at6cjfr33d73mtd",
		UpdatedByPublicID:   &updatedBy,
	})
	if got.Version != "1.2.0" || got.Digest != "sha256:abc" {
		t.Errorf("the pin did not survive conversion: %+v", got)
	}
	if got.Origin != coreplugin.ActivationAutomatic {
		t.Errorf("origin = %q, want automatic", got.Origin)
	}
	if got.SpaceID == "" || got.ActivatedBy == "" || got.UpdatedBy != updatedBy {
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
	space, err := s.CreateSpace(ctx, "activation space", owner, "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	activated, err := s.ActivatePlugin(ctx, coreplugin.ActivateInput{
		SpaceID:    space.ID,
		PluginName: "code-review",
		Version:    "1.0.0",
		Digest:     "sha256:one",
		Origin:     coreplugin.ActivationCurated,
		ActorID:    owner,
	})
	if err != nil {
		t.Fatalf("ActivatePlugin: %v", err)
	}
	if activated.ID == "" || !activated.Enabled {
		t.Fatalf("unexpected activation: %+v", activated)
	}

	// One row per space and plugin: a second activation is a pin move.
	if _, err := s.ActivatePlugin(ctx, coreplugin.ActivateInput{
		SpaceID: space.ID, PluginName: "code-review", Version: "2.0.0",
		Digest: "sha256:two", Origin: coreplugin.ActivationCurated, ActorID: owner,
	}); !errors.Is(err, coreplugin.ErrAlreadyActivated) {
		t.Fatalf("second activation err = %v, want ErrPluginAlreadyActivated", err)
	}

	moved, err := s.MovePluginActivationPin(ctx, coreplugin.MovePinInput{
		SpaceID: space.ID, PluginName: "code-review", Version: "2.0.0",
		Digest: "sha256:two", ActorID: owner,
	})
	if err != nil {
		t.Fatalf("MovePluginActivationPin: %v", err)
	}
	if moved.Version != "2.0.0" || moved.Digest != "sha256:two" {
		t.Errorf("pin did not move: %+v", moved)
	}

	// Suspension keeps the pin; that is why it is a flag and not a delete.
	suspended, err := s.SetPluginActivationEnabled(ctx, space.ID, "code-review", false, owner)
	if err != nil {
		t.Fatalf("SetPluginActivationEnabled: %v", err)
	}
	if suspended.Enabled || suspended.Version != "2.0.0" {
		t.Errorf("suspension lost the pin: %+v", suspended)
	}

	// Suspending an already suspended activation is not "not found".
	if _, err := s.SetPluginActivationEnabled(ctx, space.ID, "code-review", false, owner); err != nil {
		t.Fatalf("re-suspend: %v", err)
	}

	listed, err := s.ListPluginActivations(ctx, space.ID)
	if err != nil {
		t.Fatalf("ListPluginActivations: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("got %d activations, want 1 — a suspended one still explains a failed run", len(listed))
	}

	if _, err := s.MovePluginActivationPin(ctx, coreplugin.MovePinInput{
		SpaceID: space.ID, PluginName: "absent", Version: "1.0.0", Digest: "sha256:x", ActorID: owner,
	}); !errors.Is(err, apierr.ErrNotFound) {
		t.Fatalf("moving an absent activation err = %v, want ErrNotFound", err)
	}
}

func TestSetSpacePluginCurationRoundTrips(t *testing.T) {
	s, ctx := newTestStore(t)
	owner := newTestUser(t, s, "curation-owner")
	space, err := s.CreateSpace(ctx, "curation space", owner, "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	if space.PluginCuration != coreplugin.CurationOpen {
		t.Errorf("a new space's mode = %q, want open by default", space.PluginCuration)
	}

	if err := s.SetSpacePluginCuration(ctx, space.ID, coreplugin.CurationCurated); err != nil {
		t.Fatalf("SetSpacePluginCuration: %v", err)
	}
	got, err := s.GetSpace(ctx, space.ID)
	if err != nil {
		t.Fatalf("GetSpace: %v", err)
	}
	if got.PluginCuration != coreplugin.CurationCurated {
		t.Errorf("mode = %q, want curated", got.PluginCuration)
	}
}

func TestSetSpaceSandboxDefaultsRoundTrips(t *testing.T) {
	s, ctx := newTestStore(t)
	owner := newTestUser(t, s, "sandbox-defaults-owner")
	space, err := s.CreateSpace(ctx, "sandbox defaults space", owner, "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	if space.DefaultSandboxNetworkTier != "" || space.DefaultSandboxFilesystemTier != "" {
		t.Errorf("a new space's defaults = %q/%q, want empty", space.DefaultSandboxNetworkTier, space.DefaultSandboxFilesystemTier)
	}

	if err := s.SetSpaceSandboxDefaults(ctx, space.ID, "registries", "workspace_plus_shared_read"); err != nil {
		t.Fatalf("SetSpaceSandboxDefaults: %v", err)
	}
	got, err := s.GetSpace(ctx, space.ID)
	if err != nil {
		t.Fatalf("GetSpace: %v", err)
	}
	if got.DefaultSandboxNetworkTier != "registries" || got.DefaultSandboxFilesystemTier != "workspace_plus_shared_read" {
		t.Errorf("defaults = %q/%q, want registries/workspace_plus_shared_read", got.DefaultSandboxNetworkTier, got.DefaultSandboxFilesystemTier)
	}
}

func TestSetSpaceAgentInstructionsAdvancesOnlyOnChange(t *testing.T) {
	s, ctx := newTestStore(t)
	owner := newTestUser(t, s, "agent-instructions-owner")
	space, err := s.CreateSpace(ctx, "agent instructions space", owner, "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	if err := s.SetSpaceAgentInstructions(ctx, space.ID, "Use British English."); err != nil {
		t.Fatalf("SetSpaceAgentInstructions: %v", err)
	}
	got, err := s.GetSpace(ctx, space.ID)
	if err != nil {
		t.Fatalf("GetSpace: %v", err)
	}
	if got.AgentInstructions != "Use British English." || got.AgentInstructionsRevision != 1 {
		t.Fatalf("instructions = %q at revision %d, want first revision", got.AgentInstructions, got.AgentInstructionsRevision)
	}

	if err := s.SetSpaceAgentInstructions(ctx, space.ID, "Use British English."); err != nil {
		t.Fatalf("repeat SetSpaceAgentInstructions: %v", err)
	}
	got, err = s.GetSpace(ctx, space.ID)
	if err != nil {
		t.Fatalf("GetSpace after repeat: %v", err)
	}
	if got.AgentInstructionsRevision != 1 {
		t.Errorf("unchanged text advanced revision to %d, want 1", got.AgentInstructionsRevision)
	}
}
