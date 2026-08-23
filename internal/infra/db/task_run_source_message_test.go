package db

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
)

// A run's input is what Tier 1 decided to send a worker. Without the message it
// came from there is nothing to compare that against, so the link has to survive
// the round trip on both the first run and every continuation.
func TestCreateTaskRunPersistsSourceMessage(t *testing.T) {
	s, ctx := newTestStore(t)
	user := newTestUser(t, s, "source-message")

	conv, err := s.CreateConversation(ctx, user, "portal", user)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	asked, err := s.AppendMessage(ctx, model.AppendMessageInput{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "look into the flaky test, but do not touch the CI config",
	})
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	task, err := s.CreateTask(ctx, &model.CreateTaskInput{
		ConversationID:            conv.ID,
		Input:                     "investigate the flaky test",
		CreatedBy:                 user,
		InitialRunSourceMessageID: &asked.ID,
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

	first, err := s.GetTaskRun(ctx, *task.LastRunID)
	if err != nil {
		t.Fatalf("GetTaskRun: %v", err)
	}
	if first.SourceMessageID == nil || *first.SourceMessageID != asked.ID {
		t.Fatalf("first run source_message_id = %v, want %s", first.SourceMessageID, asked.ID)
	}

	endedAt := int64(1_800_000_000)
	if err := s.UpdateRun(ctx, model.UpdateTaskRunInput{
		TaskRunID: first.ID,
		Status:    model.RunStatusSucceeded,
		EndedAt:   &endedAt,
	}); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}

	// A continuation is a different request and records the message that made it.
	followUp, err := s.AppendMessage(ctx, model.AppendMessageInput{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "now check whether it fails on Windows too",
	})
	if err != nil {
		t.Fatalf("AppendMessage follow-up: %v", err)
	}
	second, err := s.CreateTaskRun(ctx, model.CreateTaskRunInput{
		TaskID:          task.ID,
		Input:           "check the Windows runner",
		CreatedBy:       user,
		SourceMessageID: &followUp.ID,
	})
	if err != nil {
		t.Fatalf("CreateTaskRun: %v", err)
	}
	if second.SourceMessageID == nil || *second.SourceMessageID != followUp.ID {
		t.Fatalf("returned run source_message_id = %v, want %s", second.SourceMessageID, followUp.ID)
	}
	stored, err := s.GetTaskRun(ctx, second.ID)
	if err != nil {
		t.Fatalf("GetTaskRun second: %v", err)
	}
	if stored.SourceMessageID == nil || *stored.SourceMessageID != followUp.ID {
		t.Errorf("stored source_message_id = %v, want %s", stored.SourceMessageID, followUp.ID)
	}
	if first.SourceMessageID == nil || *first.SourceMessageID == *stored.SourceMessageID {
		t.Error("each run should record its own message, not the task's first one")
	}
}

// Provenance that cannot be resolved must not cost the work. A run with no
// message behind it — a workflow step, an issue agent run, a retry — is normal,
// and a handle that resolves to nothing is treated the same way.
func TestCreateTaskRunWithoutASourceMessage(t *testing.T) {
	s, ctx := newTestStore(t)
	user := newTestUser(t, s, "source-message-absent")

	conv, err := s.CreateConversation(ctx, user, "portal", user)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	unknown, err := util.NewPublicID()
	if err != nil {
		t.Fatalf("NewPublicID: %v", err)
	}
	task, err := s.CreateTask(ctx, &model.CreateTaskInput{
		ConversationID:            conv.ID,
		Input:                     "run the nightly sweep",
		CreatedBy:                 user,
		InitialRunSourceMessageID: &unknown,
	})
	if err != nil {
		t.Fatalf("CreateTask with an unresolvable message: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.WithContext(ctx).Delete(&taskRunRow{}, "task_id = ?", task.ID)
		_ = s.db.WithContext(ctx).Delete(&taskRow{}, "task_id = ?", task.ID)
		_ = s.db.WithContext(ctx).Delete(&conversationRow{}, "conversation_id = ?", conv.ID)
	})
	run, err := s.GetTaskRun(ctx, *task.LastRunID)
	if err != nil {
		t.Fatalf("GetTaskRun: %v", err)
	}
	if run.SourceMessageID != nil {
		t.Errorf("source_message_id = %v, want nil", run.SourceMessageID)
	}
}
