package db

import (
	"errors"
	"sync"
	"testing"

	agentdef "github.com/icloudbb/buildmax/internal/core/agentdef"
	coretask "github.com/icloudbb/buildmax/internal/core/task"
)

// admissionSpace seeds a space and an agent and returns the ids a workflow-node
// admission needs. Each test gets its own space so an admission key is unique to
// it even though the key text repeats across tests.
func admissionSpace(t *testing.T, s *Store, label string) (spaceID, userID, agentID string) {
	t.Helper()
	userID = newTestUser(t, s, label)
	spaceID = newTestSpace(t, s, userID)
	agent, err := s.CreateAgentInSpace(t.Context(), agentdef.CreateInput{
		SpaceID: spaceID, UserID: userID, Def: agentdef.Definition{Name: label + "-agent"},
	})
	if err != nil {
		t.Fatalf("CreateAgentInSpace: %v", err)
	}
	return spaceID, userID, agent.ID
}

// admissionInput is the CreateInput a Workflow step dispatch hands AdmitTask:
// an agent task with workflow-step provenance under a stable key.
func admissionInput(spaceID, userID, agentID, key, input string) *coretask.CreateInput {
	return &coretask.CreateInput{
		SpaceID:                 spaceID,
		AgentID:                 &agentID,
		Input:                   input,
		Title:                   "admitted",
		CreatedBy:               userID,
		InitialRunCreatedBy:     userID,
		InitialRunCreatedByType: coretask.RunCreatedByTypeUser,
		InitialRunTriggerSource: coretask.RunTriggerSourceWorkflowStep,
		AdmissionKey:            key,
	}
}

func cleanupTask(t *testing.T, s *Store, taskID string) {
	t.Helper()
	t.Cleanup(func() {
		_ = s.db.Delete(&taskRunRow{}, "task_id = ?", canonicalTaskKey(t, s, taskID)).Error
		_ = s.db.Delete(&taskRow{}, "public_id = ?", canonicalPublicID(taskID)).Error
	})
}

func canonicalTaskKey(t *testing.T, s *Store, taskID string) uint64 {
	t.Helper()
	var row taskRow
	if err := s.db.Select("id").Where("public_id = ?", canonicalPublicID(taskID)).Take(&row).Error; err != nil {
		return 0
	}
	return row.ID
}

// The first admission creates one task and its first run; a replay under the
// same key and payload returns that same task, both while its first run is still
// active and after it has finished. That replay is what recovery of the crash
// window between admitting a Workflow node's task and linking it depends on.
func TestWorkflowTaskAdmissionCreatesOnceAndReplays(t *testing.T) {
	s, ctx := newTestStore(t)
	spaceID, userID, agentID := admissionSpace(t, s, "admit-replay")
	key := "workflow/wr_" + testPublicID(t) + "/node/research"

	first, err := s.AdmitTask(ctx, admissionInput(spaceID, userID, agentID, key, "do the research"))
	if err != nil {
		t.Fatalf("AdmitTask (first): %v", err)
	}
	cleanupTask(t, s, first.ID)
	if first.LastRunID == nil {
		t.Fatal("AdmitTask did not create the first run")
	}
	firstRunID := *first.LastRunID

	// Replay while the first run is still active: same task and same run.
	replay, err := s.AdmitTask(ctx, admissionInput(spaceID, userID, agentID, key, "do the research"))
	if err != nil {
		t.Fatalf("AdmitTask (replay, active): %v", err)
	}
	if replay.ID != first.ID {
		t.Errorf("replay returned task %q, want the original %q", replay.ID, first.ID)
	}
	if replay.LastRunID == nil || *replay.LastRunID != firstRunID {
		t.Errorf("replay last_run_id = %v, want the original %q", replay.LastRunID, firstRunID)
	}

	// Replay after the first run has finished: still the same task, not a second.
	startTaskRunForTest(t, s, ctx, firstRunID)
	if updated, err := s.TransitionTaskRun(ctx, coretask.TransitionRunInput{
		TaskRunID: firstRunID, ExpectedStatus: coretask.RunStatusRunning, NewStatus: coretask.RunStatusSucceeded,
	}); err != nil || !updated {
		t.Fatalf("TransitionTaskRun to SUCCEEDED: updated=%v err=%v", updated, err)
	}
	afterFinish, err := s.AdmitTask(ctx, admissionInput(spaceID, userID, agentID, key, "do the research"))
	if err != nil {
		t.Fatalf("AdmitTask (replay, finished): %v", err)
	}
	if afterFinish.ID != first.ID {
		t.Errorf("replay after finish returned task %q, want the original %q", afterFinish.ID, first.ID)
	}

	// Exactly one task and one run exist for the key.
	assertOneTaskForKey(t, s, spaceID, key)
	var runCount int64
	if err := s.db.Model(&taskRunRow{}).Where("task_id = ?", canonicalTaskKey(t, s, first.ID)).Count(&runCount).Error; err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runCount != 1 {
		t.Errorf("admitted task has %d runs, want exactly 1", runCount)
	}
}

// A replay whose payload differs from the admitted one is a caller mistake, not
// a silent reuse of unrelated work: it is refused with a conflict and the
// original task is left exactly as it was.
func TestWorkflowTaskAdmissionRejectsConflictingPayload(t *testing.T) {
	s, ctx := newTestStore(t)
	spaceID, userID, agentID := admissionSpace(t, s, "admit-conflict")
	key := "workflow/wr_" + testPublicID(t) + "/node/write"

	first, err := s.AdmitTask(ctx, admissionInput(spaceID, userID, agentID, key, "write the report"))
	if err != nil {
		t.Fatalf("AdmitTask (first): %v", err)
	}
	cleanupTask(t, s, first.ID)

	conflict, err := s.AdmitTask(ctx, admissionInput(spaceID, userID, agentID, key, "write a different report"))
	if !errors.Is(err, coretask.ErrTaskAdmissionConflict) {
		t.Fatalf("AdmitTask with a changed payload: err = %v, want ErrTaskAdmissionConflict", err)
	}
	if conflict != nil {
		t.Errorf("a conflicting admission returned a task %+v, want nil", conflict)
	}

	// The original is untouched.
	stored, err := s.GetTask(ctx, first.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored == nil || stored.Input != "write the report" {
		t.Errorf("original task input = %v, want it unchanged by the rejected admission", stored)
	}
	assertOneTaskForKey(t, s, spaceID, key)
}

// The key is scoped to its space: the same key text in two spaces is two
// different admissions, each with its own task.
func TestWorkflowTaskAdmissionIsScopedBySpace(t *testing.T) {
	s, ctx := newTestStore(t)
	spaceA, userA, agentA := admissionSpace(t, s, "admit-space-a")
	spaceB, userB, agentB := admissionSpace(t, s, "admit-space-b")
	key := "workflow/wr_shared/node/step"

	a, err := s.AdmitTask(ctx, admissionInput(spaceA, userA, agentA, key, "space a work"))
	if err != nil {
		t.Fatalf("AdmitTask (space a): %v", err)
	}
	cleanupTask(t, s, a.ID)
	b, err := s.AdmitTask(ctx, admissionInput(spaceB, userB, agentB, key, "space b work"))
	if err != nil {
		t.Fatalf("AdmitTask (space b): %v", err)
	}
	cleanupTask(t, s, b.ID)

	if a.ID == b.ID {
		t.Errorf("the same key text in two spaces returned one task %q, want two distinct tasks", a.ID)
	}
}

// AdmitTask refuses an empty key: idempotent admission is meaningless without
// one, so it is a caller error rather than a silent plain create.
func TestWorkflowTaskAdmissionRequiresAKey(t *testing.T) {
	s, ctx := newTestStore(t)
	spaceID, userID, agentID := admissionSpace(t, s, "admit-nokey")
	if _, err := s.AdmitTask(ctx, admissionInput(spaceID, userID, agentID, "", "work")); err == nil {
		t.Fatal("AdmitTask with an empty key returned no error, want one")
	}
}

// Concurrent identical admissions -- a retried dispatch racing itself, or two
// reconcilers observing the same due node -- must resolve to one task, never
// two executions of the same Workflow node. The unique index on
// (space_id, admission_key) is what enforces it: remove that index and this
// test fails with several tasks created at once.
func TestWorkflowTaskAdmissionHasOneWinnerUnderContention(t *testing.T) {
	s, ctx := newTestStore(t)
	spaceID, userID, agentID := admissionSpace(t, s, "admit-race")
	key := "workflow/wr_" + testPublicID(t) + "/node/parallel"

	var mu sync.Mutex
	seen := map[string]struct{}{}
	race(t, func(int) (bool, error) {
		task, err := s.AdmitTask(ctx, admissionInput(spaceID, userID, agentID, key, "contended work"))
		if err != nil {
			return false, err
		}
		mu.Lock()
		seen[task.ID] = struct{}{}
		mu.Unlock()
		return true, nil
	})
	if len(seen) != 1 {
		t.Errorf("concurrent admissions resolved to %d distinct tasks, want exactly 1: %v", len(seen), seen)
	}
	for id := range seen {
		cleanupTask(t, s, id)
	}
	assertOneTaskForKey(t, s, spaceID, key)
}

// assertOneTaskForKey fails unless exactly one task row is bound to the key in
// the space. It reads the row directly because the admission key is not part of
// the public projection.
func assertOneTaskForKey(t *testing.T, s *Store, spaceID, key string) {
	t.Helper()
	spaceKey, err := lookupKey(t.Context(), s.db, "space", spaceID)
	if err != nil {
		t.Fatalf("lookup space: %v", err)
	}
	var count int64
	if err := s.db.Model(&taskRow{}).Where("space_id = ? AND admission_key = ?", spaceKey, key).Count(&count).Error; err != nil {
		t.Fatalf("count tasks for key: %v", err)
	}
	if count != 1 {
		t.Errorf("%d tasks bound to admission key %q, want exactly 1", count, key)
	}
}
