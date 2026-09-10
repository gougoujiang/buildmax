package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
)

// workflowRunFixture creates a running workflow run with n pending step runs and
// returns the store, run id, and the step ids in index order. It registers
// cleanup of the rows it writes so it runs before the space fixture's cleanup.
func workflowRunFixture(t *testing.T, n int) (*Store, string, []string) {
	t.Helper()
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	userID, spaceID := secretTestSpace(t, s, "workflow-transition@example.com")

	wf, err := s.CreateWorkflow(ctx, spaceID, userID, "wf", "", `{"steps":[]}`)
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	t.Cleanup(func() {
		s.db.Exec(`DELETE wsr FROM workflow_step_run wsr
			JOIN workflow_run wr ON wr.id = wsr.workflow_run_id
			JOIN workflow w ON w.id = wr.workflow_id WHERE w.public_id = ?`, wf.ID)
		s.db.Exec(`DELETE wr FROM workflow_run wr
			JOIN workflow w ON w.id = wr.workflow_id WHERE w.public_id = ?`, wf.ID)
		s.db.Where("workflow_id IN (SELECT id FROM workflow WHERE public_id = ?)", wf.ID).Delete(&workflowRevisionRow{})
		s.db.Where("public_id = ?", wf.ID).Delete(&workflowRow{})
	})

	now := time.Now().UTC()
	run, err := s.CreateWorkflowRun(ctx, coreworkflow.CreateRunInput{
		WorkflowID: wf.ID,
		Status:     string(coreworkflow.RunStatusRunning),
		CreatedBy:  userID,
		StartedAt:  &now,
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	stepsIn := make([]coreworkflow.CreateStepRunInput, n)
	for i := range stepsIn {
		stepsIn[i] = coreworkflow.CreateStepRunInput{
			StepID:    string(rune('a' + i)),
			StepIndex: i,
			StepType:  coreworkflow.StepTypeAgentTask,
			Prompt:    "do",
			Status:    string(coreworkflow.StepRunStatusPending),
		}
	}
	steps, err := s.CreateWorkflowStepRuns(ctx, run.ID, stepsIn)
	if err != nil {
		t.Fatalf("CreateWorkflowStepRuns: %v", err)
	}
	ids := make([]string, len(steps))
	for i := range steps {
		ids[i] = steps[i].ID
	}
	return s, run.ID, ids
}

func TestWorkflowStepRunTransition_CAS(t *testing.T) {
	s, _, steps := workflowRunFixture(t, 1)
	ctx := context.Background()

	// A valid transition from the expected status applies.
	applied, err := s.TransitionWorkflowStepRun(ctx, coreworkflow.TransitionStepRunInput{
		StepRunID:      steps[0],
		ExpectedStatus: coreworkflow.StepRunStatusPending,
		NewStatus:      coreworkflow.StepRunStatusRunning,
	})
	if err != nil || !applied {
		t.Fatalf("pending->running = %v, %v; want true, nil", applied, err)
	}

	// The same transition again finds the step no longer pending: no write, no
	// error -- another actor won.
	applied, err = s.TransitionWorkflowStepRun(ctx, coreworkflow.TransitionStepRunInput{
		StepRunID:      steps[0],
		ExpectedStatus: coreworkflow.StepRunStatusPending,
		NewStatus:      coreworkflow.StepRunStatusRunning,
	})
	if err != nil || applied {
		t.Fatalf("stale pending->running = %v, %v; want false, nil", applied, err)
	}

	// An illegal transition is a programming error, not a lost race.
	_, err = s.TransitionWorkflowStepRun(ctx, coreworkflow.TransitionStepRunInput{
		StepRunID:      steps[0],
		ExpectedStatus: coreworkflow.StepRunStatusRunning,
		NewStatus:      coreworkflow.StepRunStatusPending,
	})
	if !errors.Is(err, coreworkflow.ErrInvalidStepRunTransition) {
		t.Fatalf("running->pending err = %v, want ErrInvalidStepRunTransition", err)
	}
}

func TestFinalizeFailedWorkflowRun_BlocksLaterSteps(t *testing.T) {
	s, runID, steps := workflowRunFixture(t, 3)
	ctx := context.Background()

	// Start the first step, then fail it: the run fails and every later step
	// still pending is blocked, atomically.
	if _, err := s.TransitionWorkflowStepRun(ctx, coreworkflow.TransitionStepRunInput{
		StepRunID:      steps[0],
		ExpectedStatus: coreworkflow.StepRunStatusPending,
		NewStatus:      coreworkflow.StepRunStatusRunning,
	}); err != nil {
		t.Fatalf("start step 0: %v", err)
	}
	now := time.Now().UTC()
	applied, err := s.FinalizeFailedWorkflowRun(ctx, coreworkflow.FinalizeFailedRunInput{
		WorkflowRunID: runID,
		StepRunID:     steps[0],
		StepIndex:     0,
		StepExpected:  coreworkflow.StepRunStatusRunning,
		StepStatus:    coreworkflow.StepRunStatusFailed,
		RunExpected:   coreworkflow.RunStatusRunning,
		RunStatus:     coreworkflow.RunStatusFailed,
		EndedAt:       &now,
	})
	if err != nil || !applied {
		t.Fatalf("finalize = %v, %v; want true, nil", applied, err)
	}

	got, err := s.ListWorkflowStepRuns(ctx, runID)
	if err != nil {
		t.Fatalf("ListWorkflowStepRuns: %v", err)
	}
	want := []string{
		string(coreworkflow.StepRunStatusFailed),
		string(coreworkflow.StepRunStatusBlocked),
		string(coreworkflow.StepRunStatusBlocked),
	}
	for i := range got {
		if got[i].Status != want[i] {
			t.Errorf("step %d status = %s, want %s", i, got[i].Status, want[i])
		}
	}
	run, err := s.GetWorkflowRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetWorkflowRun: %v", err)
	}
	if run.Status != string(coreworkflow.RunStatusFailed) {
		t.Errorf("run status = %s, want failed", run.Status)
	}
}
