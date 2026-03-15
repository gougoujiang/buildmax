// Conversation tool: start_task. Creates and schedules a Tier 2 background task.
package tools

import (
	"context"
	"fmt"

	"buildmax/internal/core"
)

// StartTaskRunner is the interface used by the start_task tool to create a background task.
// Callers (e.g. server) implement this from their config; the conversation layer passes the runner
// into RunLoop/RunLoopStream and the agent builds the StartTask tool from it.
type StartTaskRunner interface {
	StartTask(ctx context.Context, workspaceID, userID, input string, agentID *string) (taskID, runID string, err error)
}

// StartTaskFunc creates a background task (Tier 2) and schedules it to run.
// Used as a convenience; wrap with FuncStartTaskRunner to implement StartTaskRunner.
type StartTaskFunc func(ctx context.Context, input string, agentID *string) (taskID, runID string, err error)

// FuncStartTaskRunner adapts a StartTaskFunc to StartTaskRunner (ignores workspaceID/userID in the call).
func FuncStartTaskRunner(fn StartTaskFunc) StartTaskRunner {
	if fn == nil {
		return nil
	}
	return &funcStartTaskRunner{fn: fn}
}

type funcStartTaskRunner struct{ fn StartTaskFunc }

func (f *funcStartTaskRunner) StartTask(ctx context.Context, _, _, input string, agentID *string) (string, string, error) {
	return f.fn(ctx, input, agentID)
}

// startTaskTool implements Tool for StartTask; it delegates to StartTaskRunner.
type startTaskTool struct {
	workspaceID string
	userID      string
	runner      StartTaskRunner
}

func (t *startTaskTool) Name() string { return ToolNameStartTask }

func (t *startTaskTool) Description() string {
	return "Start a background task (Tier 2). The task is created and scheduled to run; it may take a while. Use this when the user asks for a long-running job, analysis, or work that should run in the background. You must tell the user that a background task was started and give them the task id so they can check progress or results later."
}

func (t *startTaskTool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "string",
				"description": "The task input or instruction for the background task (required).",
			},
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Optional workspace agent id to run the task with.",
			},
		},
		"required": []any{"input"},
	}
}

func (t *startTaskTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.runner == nil {
		return "", fmt.Errorf("%s not configured", ToolNameStartTask)
	}
	inputVal, _ := args["input"].(string)
	if inputVal == "" {
		return "", fmt.Errorf("input is required")
	}
	var agentID *string
	if aid, ok := args["agent_id"].(string); ok && aid != "" {
		agentID = &aid
	}
	taskID, runID, err := t.runner.StartTask(ctx, t.workspaceID, t.userID, inputVal, agentID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Background task created and scheduled. task_id: %s, run_id: %s. The task will run in the background; the user can check progress or results in Activity or task detail.", taskID, runID), nil
}

// NewStartTaskTool returns a core.Tool that uses runner to create a task. If runner is nil, Execute returns "not configured".
func NewStartTaskTool(workspaceID, userID string, runner StartTaskRunner) core.Tool {
	return &startTaskTool{workspaceID: workspaceID, userID: userID, runner: runner}
}
