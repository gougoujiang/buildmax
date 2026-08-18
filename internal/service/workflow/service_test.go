package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/task"
)

func TestCreateWorkflow_ValidateDefinition(t *testing.T) {
	svc := &WorkflowService{
		Workflows: &mock.MockWorkflowStore{},
		Agents: &mock.MockAgentStore{
			Agents: []model.Agent{{AgentID: "a_1", TeamID: "tm_1", Name: "Agent 1"}},
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
		Agents: []model.Agent{
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

func TestStartWorkflowRun_StepsUseAgentSnapshot(t *testing.T) {
	workflowStore := &mock.MockWorkflowStore{
		Workflows: []model.Workflow{{
			WorkflowID: "w_1",
			TeamID:     "tm_1",
			Name:       "WF",
			Definition: `{"steps":[{"step_id":"collect","type":"agent_task","target_agent_id":"a_1","prompt":"collect data"},{"step_id":"summarize","type":"agent_task","target_agent_id":"a_2","prompt":"summarize"}]}`,
			Status:     model.WorkflowStatusPublished,
		}},
	}
	taskStore := &mock.MockTaskStore{}
	agentStore := &mock.MockAgentStore{
		Agents: []model.Agent{
			{AgentID: "a_1", TeamID: "tm_1", Name: "Collector", Description: "collects", Instructions: "collect carefully"},
			{AgentID: "a_2", TeamID: "tm_1", Name: "Summarizer", Description: "summarizes", Instructions: "summarize carefully"},
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
	if steps[1].AgentName != "Summarizer" || steps[1].AgentInstructions != "summarize carefully" {
		t.Fatalf("step[1] snapshot = %q/%q, want Summarizer/summarize carefully", steps[1].AgentName, steps[1].AgentInstructions)
	}

	// Editing the agent after the run started must not change a step still pending.
	for i := range agentStore.Agents {
		if agentStore.Agents[i].AgentID == "a_2" {
			agentStore.Agents[i].Name = "Renamed"
			agentStore.Agents[i].Instructions = "rewritten mid-run"
		}
	}

	output := "done"
	if err := svc.HandleTaskRunTerminal(context.Background(), model.TaskRunTerminalInfo{
		TaskRunID: *steps[0].TaskRunID,
		TaskID:    *steps[0].TaskID,
		UserID:    "u1",
		Status:    string(model.RunStatusSucceeded),
		Output:    &output,
	}); err != nil {
		t.Fatalf("HandleTaskRunTerminal: %v", err)
	}

	updated, err := workflowStore.ListWorkflowStepRuns(context.Background(), run.WorkflowRunID)
	if err != nil {
		t.Fatalf("ListWorkflowStepRuns: %v", err)
	}
	if updated[1].Status != model.WorkflowStepRunStatusRunning {
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

func TestStepAgent_FallsBackToLiveAgentForLegacyStepRun(t *testing.T) {
	agentStore := &mock.MockAgentStore{
		Agents: []model.Agent{{AgentID: "a_1", TeamID: "tm_1", Name: "Collector", Instructions: "collect"}},
	}
	svc := &WorkflowService{Agents: agentStore}
	agent, err := svc.stepAgent(context.Background(), "tm_1", "a_1", model.WorkflowStepRun{})
	if err != nil {
		t.Fatalf("stepAgent: %v", err)
	}
	if agent.Name != "Collector" || agent.Instructions != "collect" {
		t.Fatalf("agent = %q/%q, want Collector/collect", agent.Name, agent.Instructions)
	}
	if _, err := svc.stepAgent(context.Background(), "tm_other", "a_1", model.WorkflowStepRun{}); err != ErrInvalidTargetAgent {
		t.Fatalf("cross-team stepAgent err = %v, want ErrInvalidTargetAgent", err)
	}
}
