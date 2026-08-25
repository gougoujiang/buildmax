package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/audit"
)

const (
	testTeam  = "tm_1"
	testAdmin = "u_admin"
	testDev   = "u_dev"
)

// newActivationService builds the team half of the service over in-memory
// stores. The team starts in whichever curation mode the caller names, because
// that is the setting every path here branches on.
func newActivationService(t *testing.T, curation coreplugin.Curation) (*Service, *mock.MockPluginStore, *mock.MockPluginActivationStore, *fakeAudit) {
	t.Helper()
	catalog := mock.NewMockPluginStore()
	activations := mock.NewMockPluginActivationStore()
	events := &fakeAudit{}
	teams := &mock.MockTeamStore{Teams: []model.Team{{ID: testTeam, Name: "Team", PluginCuration: curation}}}
	return &Service{
		Catalog:     catalog,
		Activations: activations,
		Teams:       teams,
		Packages:    mock.NewMockPluginPackageStorage(),
		KeyPrefix:   "bm",
		Audit:       audit.NewRecorder(events),
	}, catalog, activations, events
}

// publishInto seeds the catalog directly. The publication path has its own
// tests; these need releases to exist, not to be uploaded.
func publishInto(t *testing.T, catalog *mock.MockPluginStore, name, version string, in coreplugin.Inspection) {
	t.Helper()
	ctx := context.Background()
	if entry, err := catalog.GetPlugin(ctx, name); err != nil {
		t.Fatalf("GetPlugin: %v", err)
	} else if entry == nil {
		if _, err := catalog.CreatePlugin(ctx, coreplugin.CreateInput{Name: name, CreatedBy: testAdmin}); err != nil {
			t.Fatalf("CreatePlugin: %v", err)
		}
	}
	if _, err := catalog.CreatePluginRelease(ctx, coreplugin.CreateReleaseInput{
		PluginName:  name,
		Version:     version,
		Digest:      "sha256:" + name + version,
		ObjectKey:   "bm/" + name + "/" + version,
		Inspection:  in,
		PublishedBy: testAdmin,
	}); err != nil {
		t.Fatalf("CreatePluginRelease: %v", err)
	}
}

func skillOnly() coreplugin.Inspection {
	return coreplugin.Inspection{Skills: []string{"review"}}
}

func withHook() coreplugin.Inspection {
	return coreplugin.Inspection{
		Skills: []string{"review"},
		Hooks:  []coreplugin.Hook{{Event: "pre_tool_use"}},
	}
}

func TestActivatePinsTheNewestRelease(t *testing.T) {
	s, catalog, _, events := newActivationService(t, coreplugin.CurationCurated)
	ctx := context.Background()
	publishInto(t, catalog, "code-review", "1.0.0", skillOnly())
	publishInto(t, catalog, "code-review", "1.2.0", skillOnly())

	got, err := s.Activate(ctx, ActivateInput{TeamID: testTeam, PluginName: "code-review", ActorID: testAdmin})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if got.Version != "1.2.0" {
		t.Errorf("pinned version = %q, want the newest 1.2.0", got.Version)
	}
	if got.Digest == "" {
		t.Error("an activation without a digest is not a pin")
	}
	if got.Origin != coreplugin.ActivationCurated {
		t.Errorf("origin = %q, want curated", got.Origin)
	}
	if !events.has(model.AuditPluginActivated) {
		t.Error("activation is the record that answers why a run had a capability; it must be audited")
	}
}

// The pin is the point: publishing after an activation must not move it.
func TestAPublishAfterActivationDoesNotMoveThePin(t *testing.T) {
	s, catalog, _, _ := newActivationService(t, coreplugin.CurationCurated)
	ctx := context.Background()
	publishInto(t, catalog, "code-review", "1.0.0", skillOnly())

	activated, err := s.Activate(ctx, ActivateInput{TeamID: testTeam, PluginName: "code-review", ActorID: testAdmin})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	publishInto(t, catalog, "code-review", "2.0.0", skillOnly())

	got, err := s.ListActivations(ctx, testTeam)
	if err != nil {
		t.Fatalf("ListActivations: %v", err)
	}
	if len(got) != 1 || got[0].Version != activated.Version {
		t.Errorf("pin moved to %+v after a publish; it must stay at %q", got, activated.Version)
	}
}

func TestActivateRefusesExecutableContent(t *testing.T) {
	s, catalog, _, _ := newActivationService(t, coreplugin.CurationCurated)
	ctx := context.Background()
	publishInto(t, catalog, "guard", "1.0.0", withHook())

	_, err := s.Activate(ctx, ActivateInput{TeamID: testTeam, PluginName: "guard", ActorID: testAdmin})
	if !errors.Is(err, ErrExecutableContent) {
		t.Fatalf("err = %v, want ErrExecutableContent", err)
	}
}

// A plugin whose next version adds a hook stops at the version before it: the
// pin move goes through the same check a first activation does.
func TestMovePinRefusesAReleaseThatAddsExecutableContent(t *testing.T) {
	s, catalog, _, _ := newActivationService(t, coreplugin.CurationCurated)
	ctx := context.Background()
	publishInto(t, catalog, "code-review", "1.0.0", skillOnly())
	if _, err := s.Activate(ctx, ActivateInput{TeamID: testTeam, PluginName: "code-review", ActorID: testAdmin}); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	publishInto(t, catalog, "code-review", "1.1.0", withHook())

	_, err := s.MovePin(ctx, ActivateInput{TeamID: testTeam, PluginName: "code-review", Version: "1.1.0", ActorID: testAdmin})
	if !errors.Is(err, ErrExecutableContent) {
		t.Fatalf("err = %v, want ErrExecutableContent", err)
	}
	got, err := s.Activations.GetPluginActivation(ctx, testTeam, "code-review")
	if err != nil {
		t.Fatalf("GetPluginActivation: %v", err)
	}
	if got.Version != "1.0.0" {
		t.Errorf("a refused move changed the pin to %q", got.Version)
	}
}

func TestActivateSkipsYankedAndPrereleases(t *testing.T) {
	s, catalog, _, _ := newActivationService(t, coreplugin.CurationCurated)
	ctx := context.Background()
	publishInto(t, catalog, "code-review", "1.0.0", skillOnly())
	publishInto(t, catalog, "code-review", "1.1.0-rc.1", skillOnly())
	publishInto(t, catalog, "code-review", "1.2.0", skillOnly())
	if err := catalog.YankPluginRelease(ctx, "code-review", "1.2.0", testAdmin, "bad"); err != nil {
		t.Fatalf("YankPluginRelease: %v", err)
	}

	got, err := s.Activate(ctx, ActivateInput{TeamID: testTeam, PluginName: "code-review", ActorID: testAdmin})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if got.Version != "1.0.0" {
		t.Errorf("pinned %q; a yanked release and a prerelease are both skipped, leaving 1.0.0", got.Version)
	}
}

// Open mode: naming the plugin is what activates it, and the record says who.
func TestOpenModeActivatesOnFirstNaming(t *testing.T) {
	s, catalog, _, events := newActivationService(t, coreplugin.CurationOpen)
	ctx := context.Background()
	publishInto(t, catalog, "code-review", "1.2.0", skillOnly())

	got, err := s.ResolveSelection(ctx, testTeam, []string{"code-review"}, testDev)
	if err != nil {
		t.Fatalf("ResolveSelection: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d activations, want 1", len(got))
	}
	if got[0].Origin != coreplugin.ActivationAutomatic {
		t.Errorf("origin = %q; an open-mode activation must read as automatic", got[0].Origin)
	}
	if got[0].ActivatedBy != testDev {
		t.Errorf("activated_by = %q, want the person who saved the agent (%q)", got[0].ActivatedBy, testDev)
	}
	if got[0].Version != "1.2.0" || got[0].Digest == "" {
		t.Errorf("an automatic activation is still a pin, got %+v", got[0])
	}
	if !events.has(model.AuditPluginActivated) {
		t.Error("an automatic activation is audited like any other")
	}
}

// Curated mode refuses the same name at the write rather than activating it.
func TestCuratedModeRefusesAnUnactivatedName(t *testing.T) {
	s, catalog, _, _ := newActivationService(t, coreplugin.CurationCurated)
	ctx := context.Background()
	publishInto(t, catalog, "code-review", "1.2.0", skillOnly())

	_, err := s.ResolveSelection(ctx, testTeam, []string{"code-review"}, testDev)
	if !errors.Is(err, ErrNotActivated) {
		t.Fatalf("err = %v, want ErrNotActivated", err)
	}
}

// Open mode relaxes the team's housekeeping, never the operator's gate.
func TestOpenModeStillRefusesExecutableContent(t *testing.T) {
	s, catalog, _, _ := newActivationService(t, coreplugin.CurationOpen)
	ctx := context.Background()
	publishInto(t, catalog, "guard", "1.0.0", withHook())

	_, err := s.ResolveSelection(ctx, testTeam, []string{"guard"}, testDev)
	if !errors.Is(err, ErrExecutableContent) {
		t.Fatalf("err = %v, want ErrExecutableContent", err)
	}
}

// An automatic pin behaves like a curated one: a later release does not move it.
func TestAnAutomaticPinDoesNotAdvance(t *testing.T) {
	s, catalog, _, _ := newActivationService(t, coreplugin.CurationOpen)
	ctx := context.Background()
	publishInto(t, catalog, "code-review", "1.0.0", skillOnly())
	if _, err := s.ResolveSelection(ctx, testTeam, []string{"code-review"}, testDev); err != nil {
		t.Fatalf("ResolveSelection: %v", err)
	}
	publishInto(t, catalog, "code-review", "2.0.0", skillOnly())

	got, err := s.ResolveSelection(ctx, testTeam, []string{"code-review"}, testDev)
	if err != nil {
		t.Fatalf("ResolveSelection: %v", err)
	}
	if got[0].Version != "1.0.0" {
		t.Errorf("automatic pin advanced to %q; a pin moves only when somebody moves it", got[0].Version)
	}
}

// Resolving a selection must not fail because the plugin is suspended: a run is
// where that fails, and refusing here would block the edit that removes it.
func TestResolveSelectionReturnsASuspendedActivation(t *testing.T) {
	s, catalog, _, _ := newActivationService(t, coreplugin.CurationOpen)
	ctx := context.Background()
	publishInto(t, catalog, "code-review", "1.0.0", skillOnly())
	if _, err := s.ResolveSelection(ctx, testTeam, []string{"code-review"}, testDev); err != nil {
		t.Fatalf("ResolveSelection: %v", err)
	}
	if _, err := s.SetActivationEnabled(ctx, testTeam, "code-review", false, testAdmin); err != nil {
		t.Fatalf("SetActivationEnabled: %v", err)
	}

	got, err := s.ResolveSelection(ctx, testTeam, []string{"code-review"}, testDev)
	if err != nil {
		t.Fatalf("ResolveSelection: %v", err)
	}
	if len(got) != 1 || got[0].Enabled {
		t.Errorf("want the suspended activation returned, got %+v", got)
	}
}

func TestSuspendKeepsThePinAndIsAudited(t *testing.T) {
	s, catalog, _, events := newActivationService(t, coreplugin.CurationCurated)
	ctx := context.Background()
	publishInto(t, catalog, "code-review", "1.2.0", skillOnly())
	if _, err := s.Activate(ctx, ActivateInput{TeamID: testTeam, PluginName: "code-review", ActorID: testAdmin}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	suspended, err := s.SetActivationEnabled(ctx, testTeam, "code-review", false, testAdmin)
	if err != nil {
		t.Fatalf("SetActivationEnabled: %v", err)
	}
	if suspended.Enabled {
		t.Error("still enabled after suspension")
	}
	if suspended.Version != "1.2.0" || suspended.Digest == "" {
		t.Errorf("suspension lost the pin: %+v", suspended)
	}
	if !events.has(model.AuditPluginSuspended) {
		t.Error("suspension stops a team's runs; it must be audited")
	}

	resumed, err := s.SetActivationEnabled(ctx, testTeam, "code-review", true, testAdmin)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !resumed.Enabled || !events.has(model.AuditPluginResumed) {
		t.Errorf("resume did not take or was not audited: %+v", resumed)
	}
}

func TestActivateTwiceIsRefused(t *testing.T) {
	s, catalog, _, _ := newActivationService(t, coreplugin.CurationCurated)
	ctx := context.Background()
	publishInto(t, catalog, "code-review", "1.0.0", skillOnly())
	if _, err := s.Activate(ctx, ActivateInput{TeamID: testTeam, PluginName: "code-review", ActorID: testAdmin}); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	_, err := s.Activate(ctx, ActivateInput{TeamID: testTeam, PluginName: "code-review", ActorID: testAdmin})
	if !errors.Is(err, coreplugin.ErrAlreadyActivated) {
		t.Fatalf("err = %v, want ErrPluginAlreadyActivated: a second activation is a pin move", err)
	}
}

func TestSetCurationValidatesAndAudits(t *testing.T) {
	s, _, _, events := newActivationService(t, coreplugin.CurationOpen)
	ctx := context.Background()

	if err := s.SetCuration(ctx, testTeam, coreplugin.CurationCurated, testAdmin); err != nil {
		t.Fatalf("SetCuration: %v", err)
	}
	if !events.has(model.AuditTeamPluginCuration) {
		t.Error("the mode is a decision about a team's runs; it must be audited")
	}
	team, err := s.Teams.GetTeam(ctx, testTeam)
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if team.PluginCuration != coreplugin.CurationCurated {
		t.Errorf("mode = %q, want curated", team.PluginCuration)
	}

	if err := s.SetCuration(ctx, testTeam, coreplugin.Curation("whatever"), testAdmin); !errors.Is(err, ErrInvalidCuration) {
		t.Fatalf("err = %v, want ErrInvalidCuration", err)
	}
}

// An unset mode is open: a team that never chose has not asked to be restricted.
func TestAnUnsetCurationModeReadsAsOpen(t *testing.T) {
	if got := coreplugin.NormalizeCuration(""); got != coreplugin.CurationOpen {
		t.Errorf("empty mode = %q, want open", got)
	}
	if got := coreplugin.NormalizeCuration("curated"); got != coreplugin.CurationCurated {
		t.Errorf("curated mode = %q", got)
	}
}
