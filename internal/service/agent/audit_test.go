package agent_test

import (
	"context"
	"testing"

	coreaudit "github.com/icloudbb/buildmax/internal/core/audit"
	"github.com/icloudbb/buildmax/internal/mock"
	"github.com/icloudbb/buildmax/internal/service/agent"
	"github.com/icloudbb/buildmax/internal/service/audit"
)

// fakeAudit collects the events a service records so a test can assert what the
// trail would hold.
type fakeAudit struct{ events []coreaudit.Event }

func (f *fakeAudit) RecordAuditEvent(_ context.Context, e coreaudit.Event) error {
	f.events = append(f.events, e)
	return nil
}

func (f *fakeAudit) find(action string) (coreaudit.Event, bool) {
	for _, e := range f.events {
		if e.Action == action {
			return e, true
		}
	}
	return coreaudit.Event{}, false
}

func newAuditedService() (*agent.Service, *fakeAudit) {
	events := &fakeAudit{}
	return &agent.Service{Agents: &mock.MockAgentStore{}, Audit: audit.NewRecorder(events)}, events
}

// An agent definition is instructions plus a tool and model selection that later
// runs execute, so its whole lifecycle is recorded — with the actor, the space,
// and the agent as the target, and never the prompt.
func TestAgentLifecycleIsAudited(t *testing.T) {
	s, events := newAuditedService()
	ctx := context.Background()

	created, err := s.CreateAgent(ctx, agent.CreateCmd{SpaceID: "tm_1", UserID: "u_1", Name: "reviewer", Instructions: "i"})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	ev, ok := events.find(coreaudit.AgentCreated)
	if !ok {
		t.Fatalf("no %s recorded; got %v", coreaudit.AgentCreated, events.events)
	}
	if ev.ActorID != "u_1" || ev.SpaceID != "tm_1" || ev.TargetID != created.ID || ev.TargetType != "agent" {
		t.Errorf("create event = %+v", ev)
	}
	if ev.Detail != "reviewer" {
		t.Errorf("create detail = %q, want the agent name", ev.Detail)
	}

	if _, err := s.UpdateAgent(ctx, agent.UpdateCmd{SpaceID: "tm_1", UserID: "u_2", AgentID: created.ID, Name: "auditor", Instructions: "i"}); err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if ev, ok := events.find(coreaudit.AgentUpdated); !ok || ev.ActorID != "u_2" || ev.TargetID != created.ID {
		t.Errorf("update event = %+v ok=%v", ev, ok)
	}

	if err := s.DeleteAgent(ctx, "tm_1", created.ID, "u_3"); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	if ev, ok := events.find(coreaudit.AgentDeleted); !ok || ev.ActorID != "u_3" || ev.TargetID != created.ID {
		t.Errorf("delete event = %+v ok=%v", ev, ok)
	}
}

// A refused delete — a published workflow still names the agent — must not leave
// a deletion in the trail.
func TestBlockedDeleteRecordsNothing(t *testing.T) {
	s, events := newAuditedService()
	ctx := context.Background()
	created, err := s.CreateAgent(ctx, agent.CreateCmd{SpaceID: "tm_1", UserID: "u_1", Name: "reviewer", Instructions: "i"})
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
	s.Workflows = usedBy{{ID: "w_1", Name: "nightly"}}

	if err := s.DeleteAgent(ctx, "tm_1", created.ID, "u_1"); err == nil {
		t.Fatal("expected the delete to be refused")
	}
	if _, ok := events.find(coreaudit.AgentDeleted); ok {
		t.Error("a refused delete was recorded as a deletion")
	}
}
