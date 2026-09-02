package db

import (
	"context"
	"testing"
	"time"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
)

func startTaskRunForTest(t testing.TB, s *Store, ctx context.Context, taskRunID string) {
	t.Helper()
	for _, step := range []coretask.TransitionRunInput{
		{TaskRunID: taskRunID, ExpectedStatus: coretask.RunStatusPending, NewStatus: coretask.RunStatusScheduled},
		{TaskRunID: taskRunID, ExpectedStatus: coretask.RunStatusScheduled, NewStatus: coretask.RunStatusRunning},
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
	task, err := s.CreateTask(ctx, &coretask.CreateInput{
		TeamID:         conversation.TeamID,
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

	transition := func(from, to coretask.RunStatus, fields func(*coretask.TransitionRunInput)) {
		t.Helper()
		in := coretask.TransitionRunInput{TaskRunID: runID, ExpectedStatus: from, NewStatus: to}
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
	transition(coretask.RunStatusPending, coretask.RunStatusScheduled, nil)
	transition(coretask.RunStatusScheduled, coretask.RunStatusRunning, func(in *coretask.TransitionRunInput) {
		in.StartedAt = &startedAt
	})
	transition(coretask.RunStatusRunning, coretask.RunStatusSucceeded, func(in *coretask.TransitionRunInput) {
		in.EndedAt = &endedAt
		in.Output = &output
		in.ArtifactRelativePaths = []string{"result.md"}
	})

	staleMessage := "stale reaper outcome"
	updated, err := s.TransitionTaskRun(ctx, coretask.TransitionRunInput{
		TaskRunID:      runID,
		ExpectedStatus: coretask.RunStatusRunning,
		NewStatus:      coretask.RunStatusFailed,
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
	if storedRun == nil || storedRun.Status != string(coretask.RunStatusSucceeded) || storedRun.Output == nil || *storedRun.Output != output {
		t.Fatalf("stored run = %+v, want the worker's successful outcome", storedRun)
	}
	storedTask, err := s.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if storedTask == nil || storedTask.Status != string(coretask.RunStatusSucceeded) || storedTask.Output == nil || *storedTask.Output != output {
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
