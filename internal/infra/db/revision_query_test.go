package db

import (
	"context"
	"os"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/util"
)

// listRevisions and getRevision resolve their table from a type parameter, so
// they compile against any struct and only a real database proves GORM found
// the right one. Agents and workflows both go through them.
func revisionTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, ctx
}

func TestAgentRevisionsPageNewestFirst(t *testing.T) {
	s, ctx := revisionTestStore(t)
	teamID := util.NewPrefixedID(util.PrefixTeam)
	userID := util.NewPrefixedID(util.PrefixUser)

	agent, err := s.CreateAgentInTeam(ctx, teamID, userID, "first", "d", "i")
	if err != nil {
		t.Fatalf("CreateAgentInTeam: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.WithContext(ctx).Delete(&agentRevisionRow{}, "agent_id = ?", agent.AgentID).Error
		_ = s.db.WithContext(ctx).Delete(&agentRow{}, "agent_id = ?", agent.AgentID).Error
	})
	for _, name := range []string{"second", "third"} {
		if _, err := s.UpdateAgentInTeam(ctx, agent.AgentID, teamID, userID, name, "d", "i"); err != nil {
			t.Fatalf("UpdateAgentInTeam %s: %v", name, err)
		}
	}

	all, total, err := s.ListAgentRevisions(ctx, agent.AgentID, 0, 0)
	if err != nil {
		t.Fatalf("ListAgentRevisions: %v", err)
	}
	if total != 3 || len(all) != 3 {
		t.Fatalf("total=%d len=%d, want 3 and 3", total, len(all))
	}
	if all[0].Name != "third" {
		t.Errorf("first row = %q, want the newest revision", all[0].Name)
	}

	page, total, err := s.ListAgentRevisions(ctx, agent.AgentID, 1, 1)
	if err != nil {
		t.Fatalf("ListAgentRevisions paged: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want the count before paging", total)
	}
	if len(page) != 1 || page[0].Name != "second" {
		t.Errorf("page = %+v, want one row holding the second revision", page)
	}
}

func TestGetAgentRevisionReportsMissingAsNil(t *testing.T) {
	s, ctx := revisionTestStore(t)
	teamID := util.NewPrefixedID(util.PrefixTeam)
	userID := util.NewPrefixedID(util.PrefixUser)

	agent, err := s.CreateAgentInTeam(ctx, teamID, userID, "only", "d", "i")
	if err != nil {
		t.Fatalf("CreateAgentInTeam: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.WithContext(ctx).Delete(&agentRevisionRow{}, "agent_id = ?", agent.AgentID).Error
		_ = s.db.WithContext(ctx).Delete(&agentRow{}, "agent_id = ?", agent.AgentID).Error
	})

	got, err := s.GetAgentRevision(ctx, agent.AgentID, agent.Revision)
	if err != nil || got == nil {
		t.Fatalf("GetAgentRevision(existing) = %v, %v", got, err)
	}
	if got.Name != "only" {
		t.Errorf("Name = %q", got.Name)
	}

	missing, err := s.GetAgentRevision(ctx, agent.AgentID, 999)
	if err != nil {
		t.Fatalf("GetAgentRevision(missing) returned an error: %v", err)
	}
	if missing != nil {
		t.Errorf("a missing revision must read as nil, got %+v", missing)
	}
}

// The same two helpers, a different table and owner column.
func TestWorkflowRevisionsUseTheSameQueryShape(t *testing.T) {
	s, ctx := revisionTestStore(t)
	teamID := util.NewPrefixedID(util.PrefixTeam)
	userID := util.NewPrefixedID(util.PrefixUser)

	wf, err := s.CreateWorkflow(ctx, teamID, userID, "wf", "d", `{"steps":[]}`)
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.WithContext(ctx).Delete(&workflowRevisionRow{}, "workflow_id = ?", wf.WorkflowID).Error
		_ = s.db.WithContext(ctx).Delete(&workflowRow{}, "workflow_id = ?", wf.WorkflowID).Error
	})

	list, total, err := s.ListWorkflowRevisions(ctx, wf.WorkflowID, 0, 0)
	if err != nil {
		t.Fatalf("ListWorkflowRevisions: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("total=%d len=%d, want 1 and 1", total, len(list))
	}

	got, err := s.GetWorkflowRevision(ctx, wf.WorkflowID, wf.Revision)
	if err != nil || got == nil {
		t.Fatalf("GetWorkflowRevision = %v, %v", got, err)
	}
	if got.Name != "wf" {
		t.Errorf("Name = %q", got.Name)
	}
	if missing, err := s.GetWorkflowRevision(ctx, wf.WorkflowID, 999); err != nil || missing != nil {
		t.Errorf("missing revision = %v, %v; want nil, nil", missing, err)
	}
}
