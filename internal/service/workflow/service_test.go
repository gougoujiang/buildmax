package workflow

import (
	"context"
	"strings"
	"testing"

	agentdef "github.com/gougoujiang/buildmax/internal/core/agentdef"
	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	coreworkflow "github.com/gougoujiang/buildmax/internal/core/workflow"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/task"
)

func TestCreateWorkflow_ValidateDefinition(t *testing.T) {
	svc := &Service{
		Workflows: &mock.MockWorkflowStore{},
		Agents: &mock.MockAgentStore{
			Agents: []agentdef.Agent{{ID: "a_1", TeamID: "tm_1", Name: "Agent 1"}},
		},
	}
	workflow, err := svc.CreateWorkflow(context.Background(), CreateWorkflowCmd{
		TeamID:     "tm_1",
		UserID:     "u1",
		Name:       "WF",
		Definition: `{"steps":[{"step_id":"collect","type":"agent_task","target_agent_id":"a_1","prompt":"collect data"}]}`,
	})
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if workflow.ID == "" {
		t.Fatal("expected workflow id")
	}
	if workflow.Status != coreworkflow.StatusDraft {
		t.Fatalf("workflow status = %q, want draft", workflow.Status)
	}
}

func TestStartWorkflowRunAndAdvanceOnTerminal(t *testing.T) {
	workflowStore := &mock.MockWorkflowStore{
		Workflows: []coreworkflow.Workflow{{
			ID:          "w_1",
			TeamID:      "tm_1",
			Name:        "WF",
			Definition:  `{"steps":[{"step_id":"collect","type":"agent_task","target_agent_id":"a_1","prompt":"collect data"},{"step_id":"summarize","type":"agent_task","target_agent_id":"a_2","prompt":"summarize"}]}`,
			Description: "desc",
			Status:      coreworkflow.StatusPublished,
		}},
	}
	taskStore := &mock.MockTaskStore{}
	agentStore := &mock.MockAgentStore{
		Agents: []agentdef.Agent{
			{ID: "a_1", TeamID: "tm_1", Name: "Collector", Instructions: "collect"},
			{ID: "a_2", TeamID: "tm_1", Name: "Summarizer", Instructions: "summarize"},
		},
	}
	svc := &Service{
		Workflows: workflowStore,
		Agents:    agentStore,
		TaskService: &task.Service{
			Agents: agentStore,
			Tasks:  taskStore,
		},
	}
	run, steps, err := svc.StartWorkflowRun(context.Background(), StartWorkflowRunCmd{
		TeamID:     "tm_1",
		UserID:     "u1",
		WorkflowID: "w_1",
	})
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}
	if run.Status != coreworkflow.RunStatusRunning {
		t.Fatalf("run status = %q, want running", run.Status)
	}
	if len(steps) != 2 {
		t.Fatalf("steps len = %d, want 2", len(steps))
	}
	if steps[0].Status != coreworkflow.StepRunStatusRunning {
		t.Fatalf("step[0] status = %q, want running", steps[0].Status)
	}
	if steps[0].TaskRunID == nil {
		t.Fatal("expected first step task run id")
	}

	output := "done"
	if err := svc.HandleTaskRunTerminal(context.Background(), coretask.RunTerminalInfo{
		TaskRunID: *steps[0].TaskRunID,
		TaskID:    *steps[0].TaskID,
		UserID:    "u1",
		Status:    string(coretask.RunStatusSucceeded),
		Output:    &output,
	}); err != nil {
		t.Fatalf("HandleTaskRunTerminal first step: %v", err)
	}
	updatedSteps, err := workflowStore.ListWorkflowStepRuns(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ListWorkflowStepRuns: %v", err)
	}
	if updatedSteps[0].Status != coreworkflow.StepRunStatusSucceeded {
		t.Fatalf("step[0] status = %q, want succeeded", updatedSteps[0].Status)
	}
	if updatedSteps[1].Status != coreworkflow.StepRunStatusRunning {
		t.Fatalf("step[1] status = %q, want running", updatedSteps[1].Status)
	}
}

func TestStartWorkflowRun_StepsUseAgentSnapshot(t *testing.T) {
	workflowStore := &mock.MockWorkflowStore{
		Workflows: []coreworkflow.Workflow{{
			ID:         "w_1",
			TeamID:     "tm_1",
			Name:       "WF",
			Definition: `{"steps":[{"step_id":"collect","type":"agent_task","target_agent_id":"a_1","prompt":"collect data"},{"step_id":"summarize","type":"agent_task","target_agent_id":"a_2","prompt":"summarize"}]}`,
			Status:     coreworkflow.StatusPublished,
			Revision:   3,
		}},
	}
	taskStore := &mock.MockTaskStore{}
	agentStore := &mock.MockAgentStore{
		Agents: []agentdef.Agent{
			{ID: "a_1", TeamID: "tm_1", Name: "Collector", Description: "collects", Instructions: "collect carefully", Revision: 1},
			{ID: "a_2", TeamID: "tm_1", Name: "Summarizer", Description: "summarizes", Instructions: "summarize carefully", Revision: 2},
		},
	}
	svc := &Service{
		Workflows: workflowStore,
		Agents:    agentStore,
		TaskService: &task.Service{
			Agents: agentStore,
			Tasks:  taskStore,
		},
	}
	run, steps, err := svc.StartWorkflowRun(context.Background(), StartWorkflowRunCmd{
		TeamID:     "tm_1",
		UserID:     "u1",
		WorkflowID: "w_1",
	})
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}
	if steps[1].AgentName != "Summarizer" || steps[1].AgentInstructions != "summarize carefully" {
		t.Fatalf("step[1] snapshot = %q/%q, want Summarizer/summarize carefully", steps[1].AgentName, steps[1].AgentInstructions)
	}
	if run.WorkflowRevision != 3 {
		t.Fatalf("run workflow revision = %d, want 3", run.WorkflowRevision)
	}
	if steps[1].AgentRevision != 2 {
		t.Fatalf("step[1] agent revision = %d, want 2", steps[1].AgentRevision)
	}

	// Editing the agent after the run started must not change a step still pending.
	for i := range agentStore.Agents {
		if agentStore.Agents[i].ID == "a_2" {
			agentStore.Agents[i].Name = "Renamed"
			agentStore.Agents[i].Instructions = "rewritten mid-run"
		}
	}

	output := "done"
	if err := svc.HandleTaskRunTerminal(context.Background(), coretask.RunTerminalInfo{
		TaskRunID: *steps[0].TaskRunID,
		TaskID:    *steps[0].TaskID,
		UserID:    "u1",
		Status:    string(coretask.RunStatusSucceeded),
		Output:    &output,
	}); err != nil {
		t.Fatalf("HandleTaskRunTerminal: %v", err)
	}

	updated, err := workflowStore.ListWorkflowStepRuns(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ListWorkflowStepRuns: %v", err)
	}
	if updated[1].Status != coreworkflow.StepRunStatusRunning {
		t.Fatalf("step[1] status = %q, want running", updated[1].Status)
	}
	if len(taskStore.List) != 2 {
		t.Fatalf("tasks created = %d, want 2", len(taskStore.List))
	}
	secondInput := taskStore.List[1].Input
	if !strings.Contains(secondInput, "summarize carefully") {
		t.Fatalf("second task input lost the snapshot: %q", secondInput)
	}
	if strings.Contains(secondInput, "rewritten mid-run") {
		t.Fatalf("second task input used the edited agent: %q", secondInput)
	}
}

func TestUpdateWorkflow_RecordsRevisions(t *testing.T) {
	workflowStore := &mock.MockWorkflowStore{}
	agentStore := &mock.MockAgentStore{
		Agents: []agentdef.Agent{{ID: "a_1", TeamID: "tm_1", Name: "Agent 1", Revision: 1}},
	}
	svc := &Service{Workflows: workflowStore, Agents: agentStore}
	first := `{"steps":[{"step_id":"collect","type":"agent_task","target_agent_id":"a_1","prompt":"collect data"}]}`
	created, err := svc.CreateWorkflow(context.Background(), CreateWorkflowCmd{
		TeamID: "tm_1", UserID: "u1", Name: "WF", Definition: first,
	})
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if created.Revision != 1 {
		t.Fatalf("created revision = %d, want 1", created.Revision)
	}

	second := `{"steps":[{"step_id":"collect","type":"agent_task","target_agent_id":"a_1","prompt":"collect more data"}]}`
	updated, err := svc.UpdateWorkflow(context.Background(), UpdateWorkflowCmd{
		TeamID: "tm_1", UserID: "u2", WorkflowID: created.ID, Definition: &second,
	})
	if err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}
	if updated.Revision != 2 {
		t.Fatalf("updated revision = %d, want 2", updated.Revision)
	}

	// Saving the same content again is not a revision.
	if _, err := svc.UpdateWorkflow(context.Background(), UpdateWorkflowCmd{
		TeamID: "tm_1", UserID: "u2", WorkflowID: created.ID, Definition: &second,
	}); err != nil {
		t.Fatalf("UpdateWorkflow no-op: %v", err)
	}

	revisions, total, err := svc.ListWorkflowRevisions(context.Background(), "tm_1", created.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListWorkflowRevisions: %v", err)
	}
	if total != 2 {
		t.Fatalf("revisions = %d, want 2", total)
	}
	if revisions[0].Revision != 2 || revisions[0].CreatedBy != "u2" {
		t.Fatalf("newest revision = %d by %q, want 2 by u2", revisions[0].Revision, revisions[0].CreatedBy)
	}
	if revisions[1].Definition != first {
		t.Fatalf("revision 1 definition = %q, want the original", revisions[1].Definition)
	}
}

func TestRestoreWorkflowRevision_AppendsAndKeepsStatus(t *testing.T) {
	workflowStore := &mock.MockWorkflowStore{}
	agentStore := &mock.MockAgentStore{
		Agents: []agentdef.Agent{{ID: "a_1", TeamID: "tm_1", Name: "Agent 1", Revision: 1}},
	}
	svc := &Service{Workflows: workflowStore, Agents: agentStore}
	first := `{"steps":[{"step_id":"collect","type":"agent_task","target_agent_id":"a_1","prompt":"collect data"}]}`
	created, err := svc.CreateWorkflow(context.Background(), CreateWorkflowCmd{
		TeamID: "tm_1", UserID: "u1", Name: "WF", Definition: first,
	})
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	published := coreworkflow.StatusPublished
	if _, err := svc.UpdateWorkflow(context.Background(), UpdateWorkflowCmd{
		TeamID: "tm_1", UserID: "u1", WorkflowID: created.ID, Status: &published,
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	second := `{"steps":[{"step_id":"collect","type":"agent_task","target_agent_id":"a_1","prompt":"collect more data"}]}`
	if _, err := svc.UpdateWorkflow(context.Background(), UpdateWorkflowCmd{
		TeamID: "tm_1", UserID: "u1", WorkflowID: created.ID, Definition: &second,
	}); err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}

	restored, err := svc.RestoreWorkflowRevision(context.Background(), RestoreWorkflowRevisionCmd{
		TeamID: "tm_1", UserID: "u3", WorkflowID: created.ID, Revision: 1,
	})
	if err != nil {
		t.Fatalf("RestoreWorkflowRevision: %v", err)
	}
	if restored.Definition != first {
		t.Fatalf("restored definition = %q, want the revision 1 definition", restored.Definition)
	}
	if restored.Revision != 4 {
		t.Fatalf("restored revision = %d, want 4 — a restore appends rather than rewinds", restored.Revision)
	}
	// Revision 1 was a draft; restoring its content must not unpublish the workflow.
	if restored.Status != coreworkflow.StatusPublished {
		t.Fatalf("restored status = %q, want published", restored.Status)
	}

	if _, err := svc.RestoreWorkflowRevision(context.Background(), RestoreWorkflowRevisionCmd{
		TeamID: "tm_1", UserID: "u3", WorkflowID: created.ID, Revision: 99,
	}); err != ErrWorkflowRevisionNotFound {
		t.Fatalf("restore of missing revision err = %v, want ErrWorkflowRevisionNotFound", err)
	}
}

// TestDeletedAgent_RunFinishesButNextStepIsRefused pins the admission boundary:
// a TaskRun already in flight completes, but a later workflow step cannot
// create a new Task for an agent deleted in the meantime.
func TestDeletedAgent_RunFinishesButNextStepIsRefused(t *testing.T) {
	definition := `{"steps":[{"step_id":"collect","type":"agent_task","target_agent_id":"a_1","prompt":"collect data"},{"step_id":"summarize","type":"agent_task","target_agent_id":"a_2","prompt":"summarize"}]}`
	workflowStore := &mock.MockWorkflowStore{
		Workflows: []coreworkflow.Workflow{{
			ID:         "w_1",
			TeamID:     "tm_1",
			Name:       "WF",
			Definition: definition,
			Status:     coreworkflow.StatusPublished,
			Revision:   1,
		}},
	}
	taskStore := &mock.MockTaskStore{}
	agentStore := &mock.MockAgentStore{
		Agents: []agentdef.Agent{
			{ID: "a_1", TeamID: "tm_1", Name: "Collector", Instructions: "collect carefully", Revision: 1},
			{ID: "a_2", TeamID: "tm_1", Name: "Summarizer", Instructions: "summarize carefully", Revision: 1},
		},
	}
	svc := &Service{
		Workflows: workflowStore,
		Agents:    agentStore,
		TaskService: &task.Service{
			Agents: agentStore,
			Tasks:  taskStore,
		},
	}
	run, steps, err := svc.StartWorkflowRun(context.Background(), StartWorkflowRunCmd{
		TeamID: "tm_1", UserID: "u1", WorkflowID: "w_1",
	})
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}

	if err := agentStore.DeleteAgentInTeam(context.Background(), "a_2", "tm_1"); err != nil {
		t.Fatalf("DeleteAgentInTeam: %v", err)
	}

	output := "done"
	if err := svc.HandleTaskRunTerminal(context.Background(), coretask.RunTerminalInfo{
		TaskRunID: *steps[0].TaskRunID,
		TaskID:    *steps[0].TaskID,
		UserID:    "u1",
		Status:    string(coretask.RunStatusSucceeded),
		Output:    &output,
	}); err != task.ErrAgentNotFound {
		t.Fatalf("HandleTaskRunTerminal err = %v, want ErrAgentNotFound", err)
	}
	updated, err := workflowStore.ListWorkflowStepRuns(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ListWorkflowStepRuns: %v", err)
	}
	if updated[1].Status != coreworkflow.StepRunStatusFailed {
		t.Fatalf("step[1] status = %q, want failed after admission refusal", updated[1].Status)
	}
	if len(taskStore.List) != 1 {
		t.Fatalf("created %d tasks, want only the already-admitted first task", len(taskStore.List))
	}

	// Nothing new may name the deleted agent: not a fresh run of the workflow
	// that still references it, and not a definition written from now on.
	if _, _, err := svc.StartWorkflowRun(context.Background(), StartWorkflowRunCmd{
		TeamID: "tm_1", UserID: "u1", WorkflowID: "w_1",
	}); err != ErrInvalidTargetAgent {
		t.Fatalf("start with deleted agent err = %v, want ErrInvalidTargetAgent", err)
	}
	if _, err := svc.CreateWorkflow(context.Background(), CreateWorkflowCmd{
		TeamID: "tm_1", UserID: "u1", Name: "New", Definition: definition,
	}); err != ErrInvalidTargetAgent {
		t.Fatalf("create referencing deleted agent err = %v, want ErrInvalidTargetAgent", err)
	}
}

func TestPublishedWorkflowsUsingAgent(t *testing.T) {
	using := `{"steps":[{"step_id":"s","type":"agent_task","target_agent_id":"a_1","prompt":"p"}]}`
	other := `{"steps":[{"step_id":"s","type":"agent_task","target_agent_id":"a_2","prompt":"p"}]}`
	workflowStore := &mock.MockWorkflowStore{
		Workflows: []coreworkflow.Workflow{
			{ID: "w_pub", TeamID: "tm_1", Name: "Published", Definition: using, Status: coreworkflow.StatusPublished},
			{ID: "w_draft", TeamID: "tm_1", Name: "Draft", Definition: using, Status: coreworkflow.StatusDraft},
			{ID: "w_arch", TeamID: "tm_1", Name: "Archived", Definition: using, Status: coreworkflow.StatusArchived},
			{ID: "w_other", TeamID: "tm_1", Name: "Other agent", Definition: other, Status: coreworkflow.StatusPublished},
			{ID: "w_broken", TeamID: "tm_1", Name: "Broken", Definition: "not json", Status: coreworkflow.StatusPublished},
		},
	}
	svc := &Service{Workflows: workflowStore}
	found, err := svc.PublishedWorkflowsUsingAgent(context.Background(), "tm_1", "a_1")
	if err != nil {
		t.Fatalf("PublishedWorkflowsUsingAgent: %v", err)
	}
	if len(found) != 1 || found[0].ID != "w_pub" {
		t.Fatalf("found = %v, want only w_pub — a draft or archived workflow cannot run, and a broken one never could", found)
	}
}

func TestStepAgent_FallsBackToLiveAgentForLegacyStepRun(t *testing.T) {
	agentStore := &mock.MockAgentStore{
		Agents: []agentdef.Agent{{ID: "a_1", TeamID: "tm_1", Name: "Collector", Instructions: "collect"}},
	}
	svc := &Service{Agents: agentStore}
	agent, err := svc.stepAgent(context.Background(), "tm_1", "a_1", coreworkflow.StepRun{})
	if err != nil {
		t.Fatalf("stepAgent: %v", err)
	}
	if agent.Name != "Collector" || agent.Instructions != "collect" {
		t.Fatalf("agent = %q/%q, want Collector/collect", agent.Name, agent.Instructions)
	}
	if _, err := svc.stepAgent(context.Background(), "tm_other", "a_1", coreworkflow.StepRun{}); err != ErrInvalidTargetAgent {
		t.Fatalf("cross-team stepAgent err = %v, want ErrInvalidTargetAgent", err)
	}
}

// A canceled step stops the run without calling it a failure. The engine reacts
// to a cancel exactly as it does to a failure — later steps are blocked, the run
// ends — but a reader has to be able to tell "someone stopped this" from
// "something went wrong".
func TestHandleTaskRunTerminal_CancelStopsTheRunWithoutFailingIt(t *testing.T) {
	workflowStore := &mock.MockWorkflowStore{
		Workflows: []coreworkflow.Workflow{{
			ID:         "w_1",
			TeamID:     "tm_1",
			Name:       "WF",
			Definition: `{"steps":[{"step_id":"collect","type":"agent_task","target_agent_id":"a_1","prompt":"collect data"},{"step_id":"summarize","type":"agent_task","target_agent_id":"a_2","prompt":"summarize"}]}`,
			Status:     coreworkflow.StatusPublished,
		}},
	}
	agentStore := &mock.MockAgentStore{
		Agents: []agentdef.Agent{
			{ID: "a_1", TeamID: "tm_1", Name: "Collector", Instructions: "collect"},
			{ID: "a_2", TeamID: "tm_1", Name: "Summarizer", Instructions: "summarize"},
		},
	}
	svc := &Service{
		Workflows:   workflowStore,
		Agents:      agentStore,
		TaskService: &task.Service{Agents: agentStore, Tasks: &mock.MockTaskStore{}},
	}
	run, steps, err := svc.StartWorkflowRun(context.Background(), StartWorkflowRunCmd{
		TeamID: "tm_1", UserID: "u1", WorkflowID: "w_1",
	})
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}

	if err := svc.HandleTaskRunTerminal(context.Background(), coretask.RunTerminalInfo{
		TaskRunID: *steps[0].TaskRunID,
		TaskID:    *steps[0].TaskID,
		UserID:    "u1",
		Status:    string(coretask.RunStatusCanceled),
	}); err != nil {
		t.Fatalf("HandleTaskRunTerminal: %v", err)
	}

	updatedSteps, err := workflowStore.ListWorkflowStepRuns(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ListWorkflowStepRuns: %v", err)
	}
	if updatedSteps[0].Status != coreworkflow.StepRunStatusCanceled {
		t.Errorf("step[0] status = %q, want canceled", updatedSteps[0].Status)
	}
	if updatedSteps[1].Status != coreworkflow.StepRunStatusBlocked {
		t.Errorf("step[1] status = %q, want blocked — a canceled step must not start the next one", updatedSteps[1].Status)
	}
	updatedRun, err := workflowStore.GetWorkflowRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun: %v", err)
	}
	if updatedRun.Status != coreworkflow.RunStatusCanceled {
		t.Errorf("run status = %q, want canceled", updatedRun.Status)
	}
}
