package task

import (
	"context"
	"testing"

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
