package db

import (
	"context"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

func startTaskRunForTest(t testing.TB, s *Store, ctx context.Context, taskRunID string) {
	t.Helper()
	for _, step := range []model.TransitionTaskRunInput{
		{TaskRunID: taskRunID, ExpectedStatus: model.RunStatusPending, NewStatus: model.RunStatusScheduled},
		{TaskRunID: taskRunID, ExpectedStatus: model.RunStatusScheduled, NewStatus: model.RunStatusRunning},
	} {
		updated, err := s.TransitionTaskRun(ctx, step)
		if err != nil || !updated {
			t.Fatalf("start task run %s (%s -> %s): updated=%v err=%v", taskRunID, step.ExpectedStatus, step.NewStatus, updated, err)
		}
	}
}

func TestTransitionTaskRunDoesNotOverwriteACommittedOutcome(t *testing.T) {
	s, ctx := newTestStore(t)
	userID := newTestUser(t, s, "run-transition")
	conversation, err := s.CreateConversation(ctx, userID, "portal", userID)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	task, err := s.CreateTask(ctx, &model.CreateTaskInput{
		ConversationID: conversation.ID,
		Input:          "input",
		CreatedBy:      userID,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.LastRunID == nil {
		t.Fatal("CreateTask did not create its first run")
	}
	runID := *task.LastRunID

	transition := func(from, to model.RunStatus, fields func(*model.TransitionTaskRunInput)) {
		t.Helper()
		in := model.TransitionTaskRunInput{TaskRunID: runID, ExpectedStatus: from, NewStatus: to}
		if fields != nil {
			fields(&in)
		}
		updated, err := s.TransitionTaskRun(ctx, in)
		if err != nil {
			t.Fatalf("TransitionTaskRun %s -> %s: %v", from, to, err)
		}
		if !updated {
			t.Fatalf("TransitionTaskRun %s -> %s lost unexpectedly", from, to)
		}
	}

	startedAt := time.Unix(1_800_000_000, 0).UTC()
	endedAt := startedAt.Add(time.Minute)
	output := "worker result"
	transition(model.RunStatusPending, model.RunStatusScheduled, nil)
	transition(model.RunStatusScheduled, model.RunStatusRunning, func(in *model.TransitionTaskRunInput) {
		in.StartedAt = &startedAt
	})
	transition(model.RunStatusRunning, model.RunStatusSucceeded, func(in *model.TransitionTaskRunInput) {
		in.EndedAt = &endedAt
		in.Output = &output
		in.ArtifactRelativePaths = []string{"result.md"}
	})

	staleMessage := "stale reaper outcome"
	updated, err := s.TransitionTaskRun(ctx, model.TransitionTaskRunInput{
		TaskRunID:      runID,
		ExpectedStatus: model.RunStatusRunning,
		NewStatus:      model.RunStatusFailed,
		EndedAt:        &endedAt,
		ErrorMessage:   &staleMessage,
	})
	if err != nil {
		t.Fatalf("stale TransitionTaskRun: %v", err)
	}
	if updated {
		t.Fatal("a stale RUNNING -> FAILED transition overwrote SUCCEEDED")
	}

	storedRun, err := s.GetTaskRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetTaskRun: %v", err)
	}
	if storedRun == nil || storedRun.Status != string(model.RunStatusSucceeded) || storedRun.Output == nil || *storedRun.Output != output {
		t.Fatalf("stored run = %+v, want the worker's successful outcome", storedRun)
	}
	storedTask, err := s.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if storedTask == nil || storedTask.Status != string(model.RunStatusSucceeded) || storedTask.Output == nil || *storedTask.Output != output {
		t.Fatalf("stored task = %+v, want the same successful outcome", storedTask)
	}
	artifacts, err := s.GetTaskRunOutputFiles(ctx, runID)
	if err != nil {
		t.Fatalf("GetTaskRunOutputFiles: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].RelativePath != "result.md" {
		t.Fatalf("artifacts = %+v, want result.md", artifacts)
	}
}
