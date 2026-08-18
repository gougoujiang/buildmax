package db

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

const cancelTestUser = "cancel-store-user"

// TestTaskRunCancelQueries covers the three queries a cancel depends on. They
// are guarded by status rather than by the caller, because the caller is an
// HTTP handler racing a scheduler and a worker for the same row.
func TestTaskRunCancelQueries(t *testing.T) {
	s, ctx := newTestStore(t)

	conv, err := s.CreateConversation(ctx, cancelTestUser, "portal", cancelTestUser)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	task, err := s.CreateTask(ctx, &model.CreateTaskInput{
		ConversationID: conv.ConversationID,
		Input:          "input",
		CreatedBy:      cancelTestUser,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.WithContext(ctx).Delete(&taskRunRow{}, "task_id = ?", task.TaskID)
		_ = s.db.WithContext(ctx).Delete(&taskRow{}, "task_id = ?", task.TaskID)
		_ = s.db.WithContext(ctx).Delete(&conversationRow{}, "conversation_id = ?", conv.ConversationID)
	})
	if task.LastRunID == nil {
		t.Fatal("CreateTask should create the first run")
	}
	runID := *task.LastRunID

	active, err := s.GetActiveTaskRunByTask(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("GetActiveTaskRunByTask: %v", err)
	}
	if active == nil || active.TaskRunID != runID {
		t.Fatalf("active run = %+v, want the task's pending run %s", active, runID)
	}

	requested, err := s.RequestTaskRunCancel(ctx, runID, cancelTestUser, 1_800_000_000)
	if err != nil {
		t.Fatalf("RequestTaskRunCancel: %v", err)
	}
	if !requested {
		t.Fatal("the first cancel request was not recorded")
	}
	// A second request must not overwrite the first: the stored name is whoever
	// asked, and the stored time is what the backstop measures against.
	again, err := s.RequestTaskRunCancel(ctx, runID, "someone-else", 1_800_009_999)
	if err != nil {
		t.Fatalf("RequestTaskRunCancel again: %v", err)
	}
	if again {
		t.Error("a second cancel request overwrote the first")
	}
	stored, err := s.GetTaskRun(ctx, runID)
	if err != nil || stored == nil {
		t.Fatalf("GetTaskRun: %v", err)
	}
	if stored.CancelRequestedAt == nil || *stored.CancelRequestedAt != 1_800_000_000 {
		t.Errorf("cancel_requested_at = %v, want the first request's time", stored.CancelRequestedAt)
	}
	if stored.CancelRequestedBy == nil || *stored.CancelRequestedBy != cancelTestUser {
		t.Errorf("cancel_requested_by = %v, want %q", stored.CancelRequestedBy, cancelTestUser)
	}
	if stored.Status != string(model.RunStatusPending) {
		t.Errorf("status = %q, want the request to leave it alone", stored.Status)
	}

	// The backstop only sees requests older than the cutoff, so a run asked to
	// stop a moment ago is still its worker's to end.
	early, err := s.ListCancelRequestedTaskRuns(ctx, 1_799_999_999, 10)
	if err != nil {
		t.Fatalf("ListCancelRequestedTaskRuns: %v", err)
	}
	for _, r := range early {
		if r.TaskRunID == runID {
			t.Error("a cancel request newer than the cutoff was swept")
		}
	}
	due, err := s.ListCancelRequestedTaskRuns(ctx, 1_800_000_000, 10)
	if err != nil {
		t.Fatalf("ListCancelRequestedTaskRuns: %v", err)
	}
	if !containsRun(due, runID) {
		t.Error("a cancel request older than the cutoff was not swept")
	}

	// Once the run is terminal nothing may cancel it again, and the backstop
	// must stop seeing it.
	endedAt := int64(1_800_000_100)
	if err := s.UpdateRun(ctx, model.UpdateTaskRunInput{
		TaskRunID: runID, Status: model.RunStatusCanceled, EndedAt: &endedAt,
	}); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}
	if got, err := s.RequestTaskRunCancel(ctx, runID, cancelTestUser, 1_800_000_200); err != nil || got {
		t.Errorf("RequestTaskRunCancel on a finished run = %v, %v; want false, nil", got, err)
	}
	after, err := s.ListCancelRequestedTaskRuns(ctx, 1_800_000_300, 10)
	if err != nil {
		t.Fatalf("ListCancelRequestedTaskRuns: %v", err)
	}
	if containsRun(after, runID) {
		t.Error("a finished run is still listed as awaiting its cancel")
	}
	active, err = s.GetActiveTaskRunByTask(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("GetActiveTaskRunByTask: %v", err)
	}
	if active != nil {
		t.Errorf("active run = %+v, want none once the run is canceled", active)
	}
}

func containsRun(runs []model.TaskRun, taskRunID string) bool {
	for _, r := range runs {
		if r.TaskRunID == taskRunID {
			return true
		}
	}
	return false
}
