package task

import (
	"context"
	"errors"
	"testing"
	"time"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/mock"
)

func TestCreateRun_PersistsProvenance(t *testing.T) {
	taskStore := &mock.MockTaskStore{
		List: []coretask.Task{{
			ID:     "t_1",
			TeamID: "tm_1",
			Status: "SUCCEEDED",
		}},
	}
	runStore := &mock.MockTaskRunStore{}
	svc := &Service{
		Tasks:    taskStore,
		TaskRuns: runStore,
	}

	run, err := svc.CreateRun(context.Background(), CreateRunCmd{
		UserID:        "u1",
		TaskID:        "t_1",
		Input:         "try again",
		CreatedByType: coretask.RunCreatedByTypeUser,
		TriggerSource: coretask.RunTriggerSourcePortalConversation,
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run == nil {
		t.Fatal("CreateRun returned nil run")
	}
	if len(runStore.Runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runStore.Runs))
	}
	got := runStore.Runs[0]
	if got.CreatedBy != "u1" {
		t.Fatalf("created_by = %q, want %q", got.CreatedBy, "u1")
	}
	if got.CreatedByType != coretask.RunCreatedByTypeUser {
		t.Fatalf("created_by_type = %q, want %q", got.CreatedByType, coretask.RunCreatedByTypeUser)
	}
	if got.TriggerSource != coretask.RunTriggerSourcePortalConversation {
		t.Fatalf("trigger_source = %q, want %q", got.TriggerSource, coretask.RunTriggerSourcePortalConversation)
	}
}

// A deleted agent is invisible to every path that would start new work with
// it (agentdef.Agent.DeletedAt's own contract). Direct admission is one of
// those paths, and the only thing that keeps it that way is CreateTask
// resolving the agent through the same GetAgent a browsing caller uses, so
// this pins that resolution rather than a separate rule.
func TestCreateTaskRefusesADeletedAgent(t *testing.T) {
	deletedAt := time.Now().UTC()
	agentID := "a_deleted"
	agents := &mock.MockAgentStore{Agents: []agentdef.Agent{{
		ID: agentID, TeamID: "tm_1", Name: "gone", DeletedAt: &deletedAt,
	}}}
	svc := &Service{
		Tasks:  &mock.MockTaskStore{},
		Agents: agents,
	}

	_, err := svc.CreateTask(context.Background(), CreateTaskCmd{
		UserID: "u1", TeamID: "tm_1", Input: "do the thing", AgentID: &agentID,
	})
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("CreateTask err = %v, want %v", err, ErrAgentNotFound)
	}
}

// Continue names no agent of its own -- it runs the task's. Deleting that
// agent between a task's first run and its Continue must refuse the same way
// admission would, not fall back to whatever the task remembers.
func TestCreateRunRefusesWhenTheTasksAgentWasDeleted(t *testing.T) {
	deletedAt := time.Now().UTC()
	agentID := "a_deleted"
	agents := &mock.MockAgentStore{Agents: []agentdef.Agent{{
		ID: agentID, TeamID: "tm_1", Name: "gone", DeletedAt: &deletedAt,
	}}}
	taskStore := &mock.MockTaskStore{List: []coretask.Task{{
		ID: "t_1", TeamID: "tm_1", Status: "SUCCEEDED", AgentID: &agentID,
	}}}
	svc := &Service{
		Tasks:    taskStore,
		TaskRuns: &mock.MockTaskRunStore{},
		Agents:   agents,
	}

	_, err := svc.CreateRun(context.Background(), CreateRunCmd{
		UserID: "u1", TaskID: "t_1", Input: "keep going",
		CreatedByType: coretask.RunCreatedByTypeUser,
		TriggerSource: coretask.RunTriggerSourcePortalTaskRerun,
	})
	if !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("CreateRun err = %v, want %v", err, ErrAgentNotFound)
	}
}

// quotaStub answers Check with whatever it was given.
type quotaStub struct {
	allowed bool
	reason  string
	err     error
}

func (q quotaStub) Check(context.Context, string, int, int) (bool, string, error) {
	return q.allowed, q.reason, q.err
}

// A limit that cannot be read is not a limit that passed. Admitting the run
// would spend a team's allowance without metering it, and the caller would see
// no difference from a team that had room.
func TestAdmitsRefusesWhenQuotaCannotBeRead(t *testing.T) {
	boom := errors.New("quota store unreachable")
	svc := &Service{QuotaChecker: quotaStub{err: boom}}

	err := svc.Admits(context.Background(), "tm_1")
	if !errors.Is(err, boom) {
		t.Fatalf("Admits err = %v, want %v", err, boom)
	}
	// A read failure is a 500, not the 429 an over-quota team gets: the team
	// is not over anything, the deployment cannot see.
	if kind, _ := apierr.KindOf(err); kind == apierr.KindQuotaExceeded {
		t.Error("an unreadable quota was reported to the caller as an exceeded one")
	}
}

func TestAdmitsRefusesAnOverQuotaTeamWithTheQuotaKind(t *testing.T) {
	svc := &Service{QuotaChecker: quotaStub{allowed: false, reason: "quota exceeded: run limit"}}

	err := svc.Admits(context.Background(), "tm_1")
	if kind, _ := apierr.KindOf(err); kind != apierr.KindQuotaExceeded {
		t.Fatalf("Admits err kind = %q, want %q", kind, apierr.KindQuotaExceeded)
	}
}
