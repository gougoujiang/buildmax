// ContinueTask is a Tier 1 conversation tool that adds a follow-up run to an existing task.
package conversation

import (
	"context"
	"fmt"

	"buildmax/internal/core/model"
)

// ContinueTaskRunner is the interface used by the ContinueTask tool. Callers implement this
// using TaskService.CreateRun; must verify task belongs to the current conversation before creating run.
type ContinueTaskRunner interface {
	ContinueTask(ctx context.Context, scopeID, userID, taskID, input string) (runID string, err error)
}

type continueTaskTool struct {
	scopeID string
	userID  string
	runner  ContinueTaskRunner
}

func (t *continueTaskTool) Name() string { return ToolNameContinueTask }

func (t *continueTaskTool) Description() string {
	return "Continue an existing task with a follow-up message. Use this when the user wants to add to a task, try again with different input, or follow up on a previous result. Creates a new run for that task. Tell the user the task_id and run_id when done. Fails if the task is not in the current conversation."
}

func (t *continueTaskTool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "The task id to continue (required).",
			},
			"input": map[string]any{
				"type":        "string",
				"description": "The follow-up message or instruction (required).",
			},
		},
		"required": []any{"task_id", "input"},
	}
}

func (t *continueTaskTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.runner == nil {
		return "", fmt.Errorf("%s not configured", ToolNameContinueTask)
	}
	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	inputVal, _ := args["input"].(string)
	if inputVal == "" {
		return "", fmt.Errorf("input is required")
	}
	runID, err := t.runner.ContinueTask(ctx, t.scopeID, t.userID, taskID, inputVal)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Follow-up scheduled. task_id: %s, run_id: %s. The new run will execute in the background; the user can check progress in Activity or task detail.", taskID, runID), nil
}

// NewContinueTaskTool returns a model.Tool that continues a task with a follow-up. If runner is nil, Execute returns "not configured".
func NewContinueTaskTool(scopeID, userID string, runner ContinueTaskRunner) model.Tool {
	return &continueTaskTool{scopeID: scopeID, userID: userID, runner: runner}
}
