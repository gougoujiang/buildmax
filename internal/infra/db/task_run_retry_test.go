package db

import (
	"testing"
	"time"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
)

// A retry is only distinguishable from any other run by what it stores, so the
// lineage has to survive the round trip: without it, a repeated run is
// indistinguishable from a follow-up that happened to be asked the same thing.
func TestCreateTaskRunPersistsRetryLineage(t *testing.T) {
	s, ctx := newTestStore(t)
	retryTestUser := newTestUser(t, s, "retry-store")

	conv, err := s.CreateConversation(ctx, retryTestUser, "portal", retryTestUser)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	task, err := s.CreateTask(ctx, &coretask.CreateInput{
		TeamID:         conv.TeamID,
		ConversationID: conv.ID,
		Input:          "input",
		CreatedBy:      retryTestUser,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.WithContext(ctx).Delete(&taskRunRow{}, "task_id = ?", task.ID)
		_ = s.db.WithContext(ctx).Delete(&taskRow{}, "task_id = ?", task.ID)
		_ = s.db.WithContext(ctx).Delete(&conversationRow{}, "conversation_id = ?", conv.ID)
	})
	if task.LastRunID == nil {
		t.Fatal("CreateTask should create the first run")
	}
	failed := *task.LastRunID
	endedAt := time.Unix(1_800_000_000, 0).UTC()
	startTaskRunForTest(t, s, ctx, failed)
	if updated, err := s.TransitionTaskRun(ctx, coretask.TransitionRunInput{
		TaskRunID:      failed,
		ExpectedStatus: coretask.RunStatusRunning,
		NewStatus:      coretask.RunStatusFailed,
		EndedAt:        &endedAt,
	}); err != nil || !updated {
		t.Fatalf("TransitionTaskRun to FAILED: updated=%v err=%v", updated, err)
	}

	retry, err := s.CreateTaskRun(ctx, coretask.CreateRunInput{
		TaskID:           task.ID,
		Input:            "input",
		CreatedBy:        retryTestUser,
		CreatedByType:    coretask.RunCreatedByTypeUser,
		TriggerSource:    coretask.RunTriggerSourceTaskRetry,
		RetryOfTaskRunID: &failed,
	})
	if err != nil {
		t.Fatalf("CreateTaskRun: %v", err)
	}

	stored, err := s.GetTaskRun(ctx, retry.ID)
	if err != nil {
		t.Fatalf("GetTaskRun: %v", err)
	}
	if stored == nil {
		t.Fatal("GetTaskRun returned no run")
	}
	if stored.RetryOfTaskRunID == nil || *stored.RetryOfTaskRunID != failed {
		t.Errorf("retry_of_task_run_id = %v, want %s", stored.RetryOfTaskRunID, failed)
	}
	if stored.TriggerSource != coretask.RunTriggerSourceTaskRetry {
		t.Errorf("trigger_source = %q, want %q", stored.TriggerSource, coretask.RunTriggerSourceTaskRetry)
	}

	// The run it repeats is untouched: a retry is a new attempt, not a rewrite
	// of the record that explains why one was needed.
	previous, err := s.GetTaskRun(ctx, failed)
	if err != nil {
		t.Fatalf("GetTaskRun previous: %v", err)
	}
	if previous.Status != string(coretask.RunStatusFailed) {
		t.Errorf("previous run status = %q, want FAILED", previous.Status)
	}
	if previous.RetryOfTaskRunID != nil {
		t.Errorf("previous run retry_of_task_run_id = %v, want nil", previous.RetryOfTaskRunID)
	}
}
