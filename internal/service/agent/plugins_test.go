package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/agent"
)

// stubSelection stands in for the plugin service. It records what it was asked
// and answers with what the caller set, which is all this package needs to know
// about activation.
type stubSelection struct {
	gotTeam  string
	gotNames []string
	gotActor string
	err      error
}

func (s *stubSelection) ResolveSelection(_ context.Context, teamID string, names []string, actorID string) ([]model.PluginActivation, error) {
	s.gotTeam, s.gotNames, s.gotActor = teamID, names, actorID
	if s.err != nil {
		return nil, s.err
	}
	out := make([]model.PluginActivation, 0, len(names))
	for _, n := range names {
		out = append(out, model.PluginActivation{TeamID: teamID, PluginName: n, Version: "1.0.0", Enabled: true})
	}
	return out, nil
}

func newAgentService(sel agent.PluginSelection) (*agent.Service, *mock.MockAgentStore) {
	store := &mock.MockAgentStore{}
	return &agent.Service{Agents: store, Plugins: sel}, store
}

func TestCreateAgentStoresANormalizedSelection(t *testing.T) {
	sel := &stubSelection{}
	svc, _ := newAgentService(sel)

	created, err := svc.CreateAgent(context.Background(), agent.CreateCmd{
		TeamID: "tm_1", UserID: "u_1", Name: "Reviewer",
		Plugins: []string{" code-review ", "audit", "code-review", "  "},
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	want := []string{"audit", "code-review"}
	if len(created.Plugins) != 2 || created.Plugins[0] != want[0] || created.Plugins[1] != want[1] {
		t.Errorf("stored plugins = %v, want %v trimmed, deduplicated, and sorted", created.Plugins, want)
	}
	if sel.gotTeam != "tm_1" || sel.gotActor != "u_1" {
		t.Errorf("resolution asked for team %q actor %q, want tm_1/u_1", sel.gotTeam, sel.gotActor)
	}
	if len(sel.gotNames) != 2 {
		t.Errorf("resolution saw %v, want the normalized set", sel.gotNames)
	}
}

// An agent naming a plugin its team cannot use is refused while somebody is
// watching, rather than saved and failed at the run.
func TestCreateAgentRefusesAnUnresolvableSelection(t *testing.T) {
	refusal := errors.New("this team has not activated this plugin")
	svc, store := newAgentService(&stubSelection{err: refusal})

	_, err := svc.CreateAgent(context.Background(), agent.CreateCmd{
		TeamID: "tm_1", UserID: "u_1", Name: "Reviewer", Plugins: []string{"code-review"},
	})
	if !errors.Is(err, refusal) {
		t.Fatalf("err = %v, want the refusal", err)
	}
	if len(store.Agents) != 0 {
		t.Error("a refused selection still created the agent")
	}
}

// A deployment with no Marketplace cannot resolve a plugin, so storing a
// selection it cannot honour would be a definition that silently does less.
func TestNamingAPluginWithoutTheServiceIsRefused(t *testing.T) {
	svc, _ := newAgentService(nil)

	_, err := svc.CreateAgent(context.Background(), agent.CreateCmd{
		TeamID: "tm_1", UserID: "u_1", Name: "Reviewer", Plugins: []string{"code-review"},
	})
	if !errors.Is(err, agent.ErrPluginsNotConfigured) {
		t.Fatalf("err = %v, want ErrPluginsNotConfigured", err)
	}
}

// Naming nothing is the common case and must not need a plugin service.
func TestAnAgentNamingNoPluginNeedsNoService(t *testing.T) {
	svc, _ := newAgentService(nil)

	created, err := svc.CreateAgent(context.Background(), agent.CreateCmd{
		TeamID: "tm_1", UserID: "u_1", Name: "Reviewer",
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if len(created.Plugins) != 0 {
		t.Errorf("plugins = %v, want none — nothing is inherited", created.Plugins)
	}
}

// The selection versions with the rest of the definition, and an update can
// clear it.
func TestUpdateRecordsAndClearsTheSelection(t *testing.T) {
	svc, _ := newAgentService(&stubSelection{})
	ctx := context.Background()
	created, err := svc.CreateAgent(ctx, agent.CreateCmd{
		TeamID: "tm_1", UserID: "u_1", Name: "Reviewer", Plugins: []string{"code-review"},
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	cleared, err := svc.UpdateAgent(ctx, agent.UpdateCmd{
		TeamID: "tm_1", UserID: "u_1", AgentID: created.ID, Name: "Reviewer",
	})
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if len(cleared.Plugins) != 0 {
		t.Errorf("plugins = %v, want cleared", cleared.Plugins)
	}

	revs, _, err := svc.ListRevisions(ctx, "tm_1", created.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revs) != 2 {
		t.Fatalf("got %d revisions, want 2", len(revs))
	}
	// Newest first: the cleared one, then the one that named the plugin.
	if len(revs[0].Plugins) != 0 {
		t.Errorf("newest revision plugins = %v, want none", revs[0].Plugins)
	}
	if len(revs[1].Plugins) != 1 || revs[1].Plugins[0] != "code-review" {
		t.Errorf("older revision plugins = %v; an old revision must still answer what that agent named", revs[1].Plugins)
	}
}

// Reordering the same set is not an edit, so it must not append a revision.
func TestReorderingTheSameSelectionIsNotAnEdit(t *testing.T) {
	svc, _ := newAgentService(&stubSelection{})
	ctx := context.Background()
	created, err := svc.CreateAgent(ctx, agent.CreateCmd{
		TeamID: "tm_1", UserID: "u_1", Name: "Reviewer", Plugins: []string{"audit", "code-review"},
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if _, err := svc.UpdateAgent(ctx, agent.UpdateCmd{
		TeamID: "tm_1", UserID: "u_1", AgentID: created.ID, Name: "Reviewer",
		Plugins: []string{"code-review", "audit"},
	}); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	_, total, err := svc.ListRevisions(ctx, "tm_1", created.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if total != 1 {
		t.Errorf("revisions = %d, want 1: a reshuffle of the same set is not a change", total)
	}
}

// Restoring an old revision restores what it named, not today's selection.
func TestRestoreBringsBackTheSelection(t *testing.T) {
	svc, _ := newAgentService(&stubSelection{})
	ctx := context.Background()
	created, err := svc.CreateAgent(ctx, agent.CreateCmd{
		TeamID: "tm_1", UserID: "u_1", Name: "Reviewer", Plugins: []string{"code-review"},
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if _, err := svc.UpdateAgent(ctx, agent.UpdateCmd{
		TeamID: "tm_1", UserID: "u_1", AgentID: created.ID, Name: "Reviewer",
	}); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}

	restored, err := svc.RestoreRevision(ctx, agent.RestoreRevisionCmd{
		TeamID: "tm_1", UserID: "u_1", AgentID: created.ID, Revision: 1,
	})
	if err != nil {
		t.Fatalf("RestoreRevision: %v", err)
	}
	if len(restored.Plugins) != 1 || restored.Plugins[0] != "code-review" {
		t.Errorf("restored plugins = %v, want code-review", restored.Plugins)
	}
}
