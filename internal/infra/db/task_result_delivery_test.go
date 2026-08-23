package db

import (
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// deliveryFixture creates a conversation, a task, and its first run.
func deliveryFixture(t *testing.T, s *Store, label string) (conversationID, taskRunID string) {
	t.Helper()
	ctx := t.Context()
	user := newTestUser(t, s, label)
	conv, err := s.CreateConversation(ctx, user, "portal", user)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	task, err := s.CreateTask(ctx, &model.CreateTaskInput{
		ConversationID: conv.ID, Input: "input", CreatedBy: user,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.Delete(&taskResultDeliveryRow{}, "task_run_id IN (SELECT id FROM task_run WHERE task_id IN (SELECT id FROM task WHERE public_id = ?))", task.ID)
		_ = s.db.Delete(&taskRunRow{}, "task_id = ?", task.ID)
		_ = s.db.Delete(&taskRow{}, "task_id = ?", task.ID)
		_ = s.db.Delete(&conversationRow{}, "conversation_id = ?", conv.ID)
	})
	return conv.ID, *task.LastRunID
}

// One run owes one report. A run whose outcome is announced twice — by its
// worker and then by the reaper that gave up on it — must not queue two.
func TestEnqueueTaskResultDeliveryIsIdempotent(t *testing.T) {
	s, ctx := newTestStore(t)
	convID, runID := deliveryFixture(t, s, "delivery-enqueue")

	if err := s.EnqueueTaskResultDelivery(ctx, runID, convID, time.Unix(100, 0).UTC()); err != nil {
		t.Fatalf("EnqueueTaskResultDelivery: %v", err)
	}
	if err := s.EnqueueTaskResultDelivery(ctx, runID, convID, time.Unix(200, 0).UTC()); err != nil {
		t.Fatalf("second EnqueueTaskResultDelivery: %v", err)
	}

	due, err := s.ListDueTaskResultDeliveries(ctx, time.Unix(300, 0).UTC(), 10)
	if err != nil {
		t.Fatalf("ListDueTaskResultDeliveries: %v", err)
	}
	found := 0
	for _, d := range due {
		if d.TaskRunID == runID {
			found++
			if !d.NextAttemptAt.Equal(time.Unix(100, 0).UTC()) {
				t.Errorf("next_attempt_at = %v, want the first enqueue's, not the second's", d.NextAttemptAt)
			}
		}
	}
	if found != 1 {
		t.Errorf("the run owes %d reports, want 1", found)
	}
}

// The claim is the whole defence against reporting one run twice: two sweepers
// see the same due row, and only one of them may proceed.
func TestClaimTaskResultDeliveryTakesItOnce(t *testing.T) {
	s, ctx := newTestStore(t)
	convID, runID := deliveryFixture(t, s, "delivery-claim")
	if err := s.EnqueueTaskResultDelivery(ctx, runID, convID, time.Unix(100, 0).UTC()); err != nil {
		t.Fatal(err)
	}

	first, err := s.ClaimTaskResultDelivery(ctx, runID, time.Unix(200, 0).UTC(), time.Unix(900, 0).UTC())
	if err != nil {
		t.Fatalf("ClaimTaskResultDelivery: %v", err)
	}
	if first == nil {
		t.Fatal("the first claim took nothing")
	}
	if first.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", first.Attempts)
	}

	// The same moment again: the claim pushed the next attempt to 900.
	second, err := s.ClaimTaskResultDelivery(ctx, runID, time.Unix(200, 0).UTC(), time.Unix(900, 0).UTC())
	if err != nil {
		t.Fatalf("second ClaimTaskResultDelivery: %v", err)
	}
	if second != nil {
		t.Errorf("a claimed delivery was claimed again: %+v", second)
	}
}

// A failed attempt keeps the report owed, says why, and brings the retry
// forward from the lease the claim set.
func TestRecordTaskResultDeliveryFailureKeepsItOwed(t *testing.T) {
	s, ctx := newTestStore(t)
	convID, runID := deliveryFixture(t, s, "delivery-failure")
	if err := s.EnqueueTaskResultDelivery(ctx, runID, convID, time.Unix(100, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimTaskResultDelivery(ctx, runID, time.Unix(200, 0).UTC(), time.Unix(900, 0).UTC()); err != nil {
		t.Fatal(err)
	}

	if err := s.RecordTaskResultDeliveryFailure(ctx, runID, "the model refused", time.Unix(260, 0).UTC()); err != nil {
		t.Fatalf("RecordTaskResultDeliveryFailure: %v", err)
	}

	due, err := s.ListDueTaskResultDeliveries(ctx, time.Unix(260, 0).UTC(), 10)
	if err != nil {
		t.Fatal(err)
	}
	var got *model.TaskResultDelivery
	for i := range due {
		if due[i].TaskRunID == runID {
			got = &due[i]
		}
	}
	if got == nil {
		t.Fatal("the failed report is no longer due")
	}
	if got.LastError == nil || *got.LastError != "the model refused" {
		t.Errorf("last_error = %v, want the reason", got.LastError)
	}
}

// A finished delivery is finished. Nothing reopens one, so a sweep after it
// cannot report the same run again.
func TestFinishTaskResultDeliveryClosesIt(t *testing.T) {
	s, ctx := newTestStore(t)
	convID, runID := deliveryFixture(t, s, "delivery-finish")
	if err := s.EnqueueTaskResultDelivery(ctx, runID, convID, time.Unix(100, 0).UTC()); err != nil {
		t.Fatal(err)
	}

	if err := s.FinishTaskResultDelivery(ctx, runID, model.DeliveryDelivered, nil); err != nil {
		t.Fatalf("FinishTaskResultDelivery: %v", err)
	}

	due, err := s.ListDueTaskResultDeliveries(ctx, time.Unix(10_000, 0).UTC(), 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range due {
		if d.TaskRunID == runID {
			t.Fatalf("a delivered report is still due: %+v", d)
		}
	}
	claimed, err := s.ClaimTaskResultDelivery(ctx, runID, time.Unix(10_000, 0).UTC(), time.Unix(20_000, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if claimed != nil {
		t.Errorf("a delivered report was claimed again: %+v", claimed)
	}
}
