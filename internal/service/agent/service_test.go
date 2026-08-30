package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/agent"
)

func newService(t *testing.T) (*agent.Service, *mock.MockAgentStore, context.Context) {
	t.Helper()
	store := &mock.MockAgentStore{}
	return &agent.Service{Agents: store}, store, context.Background()
}

func create(t *testing.T, s *agent.Service, teamID string) *agentdef.Agent {
	t.Helper()
	a, err := s.CreateAgent(context.Background(), agent.CreateCmd{
		TeamID: teamID, UserID: "u_1", Name: "reviewer", Description: "d", Instructions: "i",
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	return a
}

func TestCreateRequiresAName(t *testing.T) {
	s, _, ctx := newService(t)

	_, err := s.CreateAgent(ctx, agent.CreateCmd{TeamID: "tm_1", UserID: "u_1"})

	if !errors.Is(err, agent.ErrNameRequired) {
		t.Fatalf("err = %v, want ErrNameRequired", err)
	}
	if kind, _ := apierr.KindOf(err); kind != apierr.KindInvalid {
		t.Errorf("kind = %q, want invalid", kind)
	}
}

// TestCreateRejectsUnknownSandboxTier asserts a tier outside
// config.ValidSandboxNetworkTier/ValidSandboxFilesystemTier is refused before
// anything is stored, the same way an empty name is.
func TestCreateRejectsUnknownSandboxTier(t *testing.T) {
	s, _, ctx := newService(t)

	_, err := s.CreateAgent(ctx, agent.CreateCmd{
		TeamID: "tm_1", UserID: "u_1", Name: "reviewer",
		SandboxNetworkTier: "unlimited",
	})

	if !errors.Is(err, agent.ErrInvalidSandboxTier) {
		t.Fatalf("err = %v, want ErrInvalidSandboxTier", err)
	}
	if kind, _ := apierr.KindOf(err); kind != apierr.KindInvalid {
		t.Errorf("kind = %q, want invalid", kind)
	}
}

// TestCreateAndUpdateStoreSandboxTiers asserts a valid declared tier is
// stored on create, versions with an update, and an update that omits it
// resets that axis to the strictest tier rather than leaving it unchanged.
func TestCreateAndUpdateStoreSandboxTiers(t *testing.T) {
	s, _, ctx := newService(t)

	a, err := s.CreateAgent(ctx, agent.CreateCmd{
		TeamID: "tm_1", UserID: "u_1", Name: "builder",
		SandboxNetworkTier: "registries", SandboxFilesystemTier: "workspace",
	})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	if a.SandboxNetworkTier != "registries" {
		t.Errorf("SandboxNetworkTier = %q, want registries", a.SandboxNetworkTier)
	}

	updated, err := s.UpdateAgent(ctx, agent.UpdateCmd{
		TeamID: "tm_1", UserID: "u_1", AgentID: a.ID, Name: "builder",
	})
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if updated.SandboxNetworkTier != "" {
		t.Errorf("SandboxNetworkTier after omitting it = %q, want empty (reset to strictest)", updated.SandboxNetworkTier)
	}
}

// The ownership check was written out separately in four handlers. It belongs
// in one place, and it has to answer not-found rather than forbidden so the
// reply does not confirm that the id exists in another team.
func TestAnotherTeamsAgentReadsAsNotFound(t *testing.T) {
	s, _, ctx := newService(t)
	other := create(t, s, "tm_other")

	_, err := s.GetAgent(ctx, "tm_mine", other.ID)

	if !errors.Is(err, agent.ErrAgentNotFound) {
		t.Fatalf("err = %v, want ErrAgentNotFound", err)
	}
	if kind, _ := apierr.KindOf(err); kind != apierr.KindNotFound {
		t.Errorf("kind = %q, want not_found", kind)
	}
}

func TestRevisionsAndRestoreStayInsideTheTeam(t *testing.T) {
	s, _, ctx := newService(t)
	other := create(t, s, "tm_other")

	if _, _, err := s.ListRevisions(ctx, "tm_mine", other.ID, 10, 0); !errors.Is(err, agent.ErrAgentNotFound) {
		t.Errorf("ListRevisions leaked another team's agent: %v", err)
	}
	_, err := s.RestoreRevision(ctx, agent.RestoreRevisionCmd{TeamID: "tm_mine", UserID: "u_1", AgentID: other.ID, Revision: 1})
	if !errors.Is(err, agent.ErrAgentNotFound) {
		t.Errorf("RestoreRevision leaked another team's agent: %v", err)
	}
}

// Restoring is an edit: it appends a revision rather than rewinding history.
func TestRestoreAppendsRatherThanRewinds(t *testing.T) {
	s, _, ctx := newService(t)
	a := create(t, s, "tm_1")
	if _, err := s.UpdateAgent(ctx, agent.UpdateCmd{
		TeamID: "tm_1", UserID: "u_1", AgentID: a.ID, Name: "renamed", Description: "d2", Instructions: "i2",
	}); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}

	restored, err := s.RestoreRevision(ctx, agent.RestoreRevisionCmd{
		TeamID: "tm_1", UserID: "u_1", AgentID: a.ID, Revision: 1,
	})
	if err != nil {
		t.Fatalf("RestoreRevision: %v", err)
	}
	if restored.Name != "reviewer" {
		t.Errorf("Name = %q, want the first revision's name back", restored.Name)
	}

	_, total, err := s.ListRevisions(ctx, "tm_1", a.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if total != 3 {
		t.Errorf("revisions = %d, want 3: create, rename, restore", total)
	}
}

func TestMissingRevisionIsReported(t *testing.T) {
	s, _, ctx := newService(t)
	a := create(t, s, "tm_1")

	_, err := s.RestoreRevision(ctx, agent.RestoreRevisionCmd{
		TeamID: "tm_1", UserID: "u_1", AgentID: a.ID, Revision: 999,
	})

	if !errors.Is(err, agent.ErrRevisionNotFound) {
		t.Fatalf("err = %v, want ErrRevisionNotFound", err)
	}
}

type usedBy []coreworkflow.Workflow

func (u usedBy) PublishedWorkflowsUsingAgent(context.Context, string, string) ([]coreworkflow.Workflow, error) {
	return u, nil
}

// Deleting an agent a published workflow names would break that workflow at its
// next step, so the refusal names the workflows rather than just saying no.
func TestDeleteNamesTheWorkflowsBlockingIt(t *testing.T) {
	s, _, ctx := newService(t)
	a := create(t, s, "tm_1")
	s.Workflows = usedBy{{ID: "w_1", Name: "nightly"}, {ID: "w_2", Name: "release"}}

	err := s.DeleteAgent(ctx, "tm_1", a.ID)

	if !errors.Is(err, agent.ErrUsedByPublishedFlows) {
		t.Fatalf("err = %v, want ErrUsedByPublishedFlows", err)
	}
	if kind, _ := apierr.KindOf(err); kind != apierr.KindConflict {
		t.Errorf("kind = %q, want conflict", kind)
	}
	for _, want := range []string{"nightly (w_1)", "release (w_2)"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("want %q named in %q", want, err.Error())
		}
	}
}

// A deployment that cannot answer the question does not block the delete, which
// is what the handler did when the store was nil.
func TestDeleteProceedsWithoutAWorkflowSource(t *testing.T) {
	s, _, ctx := newService(t)
	a := create(t, s, "tm_1")

	if err := s.DeleteAgent(ctx, "tm_1", a.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	if _, err := s.GetAgent(ctx, "tm_1", a.ID); !errors.Is(err, agent.ErrAgentNotFound) {
		t.Errorf("a deleted agent should read as not found, got %v", err)
	}
}

func TestDeletingAnotherTeamsAgentIsNotFound(t *testing.T) {
	s, _, ctx := newService(t)
	other := create(t, s, "tm_other")

	if err := s.DeleteAgent(ctx, "tm_mine", other.ID); !errors.Is(err, agent.ErrAgentNotFound) {
		t.Errorf("err = %v, want ErrAgentNotFound", err)
	}
}

// Every method has to say "not configured" rather than panic on a deployment
// with no database.
func TestNoStoreIsReportedNotPanicked(t *testing.T) {
	s := &agent.Service{}
	ctx := context.Background()

	checks := []error{}
	_, err := s.ListAgents(ctx, "tm_1")
	checks = append(checks, err)
	_, err = s.CreateAgent(ctx, agent.CreateCmd{TeamID: "tm_1", Name: "x"})
	checks = append(checks, err)
	_, err = s.GetAgent(ctx, "tm_1", "a_1")
	checks = append(checks, err)
	_, err = s.UpdateAgent(ctx, agent.UpdateCmd{TeamID: "tm_1", AgentID: "a_1", Name: "x"})
	checks = append(checks, err)
	checks = append(checks, s.DeleteAgent(ctx, "tm_1", "a_1"))

	for i, err := range checks {
		if !errors.Is(err, agent.ErrAgentsNotConfigured) {
			t.Errorf("check %d: err = %v, want ErrAgentsNotConfigured", i, err)
		}
	}
}
