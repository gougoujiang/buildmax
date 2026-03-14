package adapter

import (
	"context"
	"errors"
	"testing"

	"buildmax/internal/conversation"
	"buildmax/internal/storage/entity"
)

// recordingChatRunStore records CreateChatRun arguments and returns a configurable run or error.
type recordingChatRunStore struct {
	createChatRun func(ctx context.Context, chatID, input, createdBy string) (*entity.ChatRun, error)
	calls         []struct{ chatID, input, createdBy string }
}

func (r *recordingChatRunStore) CreateChatRun(ctx context.Context, chatID, input, createdBy string) (*entity.ChatRun, error) {
	r.calls = append(r.calls, struct{ chatID, input, createdBy string }{chatID, input, createdBy})
	if r.createChatRun != nil {
		return r.createChatRun(ctx, chatID, input, createdBy)
	}
	return &entity.ChatRun{ChatRunID: "r_abc", ChatID: chatID, Input: input}, nil
}

func (r *recordingChatRunStore) GetNextPendingChatRun(context.Context) (*entity.ChatRun, error) { return nil, nil }
func (r *recordingChatRunStore) GetChatRun(context.Context, string) (*entity.ChatRun, error)   { return nil, nil }
func (r *recordingChatRunStore) GetChatRunWithChat(context.Context, string) (*entity.ChatRun, *entity.Chat, error) {
	return nil, nil, nil
}
func (r *recordingChatRunStore) UpdateChatRunStatusIf(context.Context, string, string, string, *int64, *int64, *string, *string, *string) (bool, error) {
	return false, nil
}
func (r *recordingChatRunStore) UpdateChatRunStatus(context.Context, string, string, *int64, *int64, *string, *string, *string, *int, *int) error {
	return nil
}
func (r *recordingChatRunStore) UpdateChatRunWorkerInfo(context.Context, string, string, *string, *int64) error {
	return nil
}
func (r *recordingChatRunStore) OnRunComplete(context.Context, string, []string) error { return nil }
func (r *recordingChatRunStore) SyncChatFromRun(context.Context, string) error         { return nil }

func TestPassThroughEngine_Process(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		rec := &recordingChatRunStore{}
		engine := NewPassThroughEngine(rec)
		turn := conversation.ConversationTurn{
			WorkspaceID:    "w_1",
			ConversationID: "c_1",
			UserID:         "u_1",
			Message:        "do something",
		}
		result, err := engine.Process(ctx, "w_1", "c_1", turn)
		if err != nil {
			t.Fatalf("Process: %v", err)
		}
		if len(rec.calls) != 1 {
			t.Fatalf("CreateChatRun called %d times, want 1", len(rec.calls))
		}
		if rec.calls[0].chatID != "c_1" {
			t.Errorf("CreateChatRun chatID = %q, want c_1", rec.calls[0].chatID)
		}
		if rec.calls[0].input != "do something" {
			t.Errorf("CreateChatRun input = %q, want do something", rec.calls[0].input)
		}
		if rec.calls[0].createdBy != "u_1" {
			t.Errorf("CreateChatRun createdBy = %q, want u_1", rec.calls[0].createdBy)
		}
		if len(result.TaskIDs) != 1 || result.TaskIDs[0] != "r_abc" {
			t.Errorf("TaskIDs = %v, want [r_abc]", result.TaskIDs)
		}
	})

	t.Run("ErrRunInProgress", func(t *testing.T) {
		rec := &recordingChatRunStore{
			createChatRun: func(context.Context, string, string, string) (*entity.ChatRun, error) {
				return nil, entity.ErrRunInProgress
			},
		}
		engine := NewPassThroughEngine(rec)
		turn := conversation.ConversationTurn{UserID: "u_1", Message: "x"}
		result, err := engine.Process(ctx, "w_1", "c_1", turn)
		if !errors.Is(err, entity.ErrRunInProgress) {
			t.Errorf("err = %v, want ErrRunInProgress", err)
		}
		if len(result.TaskIDs) != 0 {
			t.Errorf("TaskIDs = %v, want empty", result.TaskIDs)
		}
	})
}
