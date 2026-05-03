package workflow

import (
	"context"
	"testing"

	"buildmax/internal/core/model"
	"buildmax/internal/core/task"
	"buildmax/internal/mock"
)

func TestCreateWorkflow_ValidateDefinition(t *testing.T) {
	svc := &WorkflowService{
		Workflows: &mock.MockWorkflowStore{},
		Agents: &mock.MockAgentStore{
			Agents: []model.AgentDefinition{{AgentID: "a_1", TeamID: "tm_1", Name: "Agent 1"}},
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
	if workflow.WorkflowID == "" {
		t.Fatal("expected workflow id")
	}
	if workflow.Status != model.WorkflowStatusDraft {
		t.Fatalf("workflow status = %q, want draft", workflow.Status)
	}
}

func TestStartWorkflowRunAndAdvanceOnTerminal(t *testing.T) {
	workflowStore := &mock.MockWorkflowStore{
		Workflows: []model.Workflow{{
			WorkflowID:  "w_1",
			TeamID:      "tm_1",
			Name:        "WF",
			Definition:  `{"steps":[{"step_id":"collect","type":"agent_task","target_agent_id":"a_1","prompt":"collect data"},{"step_id":"summarize","type":"agent_task","target_agent_id":"a_2","prompt":"summarize"}]}`,
			Description: "desc",
			Status:      model.WorkflowStatusPublished,
		}},
	}
	taskStore := &mock.MockTaskStore{}
	agentStore := &mock.MockAgentStore{
		Agents: []model.AgentDefinition{
			{AgentID: "a_1", TeamID: "tm_1", Name: "Collector", Instructions: "collect"},
			{AgentID: "a_2", TeamID: "tm_1", Name: "Summarizer", Instructions: "summarize"},
		},
	}
	svc := &WorkflowService{
		Workflows:     workflowStore,
		Agents:        agentStore,
		Conversations: &mock.MockConversationStore{},
		TaskService: &task.TaskService{
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
	if run.Status != model.WorkflowRunStatusRunning {
		t.Fatalf("run status = %q, want running", run.Status)
	}
	if len(steps) != 2 {
		t.Fatalf("steps len = %d, want 2", len(steps))
	}
	if steps[0].Status != model.WorkflowStepRunStatusRunning {
		t.Fatalf("step[0] status = %q, want running", steps[0].Status)
	}
	if steps[0].TaskRunID == nil {
		t.Fatal("expected first step task run id")
	}

	output := "done"
	if err := svc.HandleTaskRunTerminal(context.Background(), model.TaskRunTerminalInfo{
		TaskRunID: *steps[0].TaskRunID,
		TaskID:    *steps[0].TaskID,
		UserID:    "u1",
		Status:    string(model.RunStatusSucceeded),
		Output:    &output,
	}); err != nil {
		t.Fatalf("HandleTaskRunTerminal first step: %v", err)
	}
	updatedSteps, err := workflowStore.ListWorkflowStepRuns(context.Background(), run.WorkflowRunID)
	if err != nil {
		t.Fatalf("ListWorkflowStepRuns: %v", err)
	}
	if updatedSteps[0].Status != model.WorkflowStepRunStatusSucceeded {
		t.Fatalf("step[0] status = %q, want succeeded", updatedSteps[0].Status)
	}
	if updatedSteps[1].Status != model.WorkflowStepRunStatusRunning {
		t.Fatalf("step[1] status = %q, want running", updatedSteps[1].Status)
	}
}
