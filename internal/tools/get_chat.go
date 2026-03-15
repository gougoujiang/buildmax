// GetTask is a Tier 1 conversation tool that returns detail for one task by id.
package tools

import (
	"context"
	"fmt"

	"buildmax/internal/core"
)

// GetTaskRunner is the interface used by the GetTask tool. Callers implement this
// using TaskStore; must return workspace-scoped results only.
type GetTaskRunner interface {
	GetTask(ctx context.Context, workspaceID, taskID string) (detail string, err error)
}

type getTaskTool struct {
	workspaceID string
	runner      GetTaskRunner
}

func (t *getTaskTool) Name() string { return ToolNameGetTask }

func (t *getTaskTool) Description() string {
	return "Get detail for one task by task_id. Use this when the user asks about a specific task's status, result, or content. Returns task_id, title, input, status, created_at, last_run_id, and optional output snippet. Fails with a clear error if the task is not in the current workspace."
}

func (t *getTaskTool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "The task id (required).",
			},
		},
		"required": []any{"task_id"},
	}
}

func (t *getTaskTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.runner == nil {
		return "", fmt.Errorf("%s not configured", ToolNameGetTask)
	}
	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	detail, err := t.runner.GetTask(ctx, t.workspaceID, taskID)
	if err != nil {
		return "", err
	}
	return detail, nil
}

// NewGetTaskTool returns a core.Tool that gets one task's detail. If runner is nil, Execute returns "not configured".
func NewGetTaskTool(workspaceID string, runner GetTaskRunner) core.Tool {
	return &getTaskTool{workspaceID: workspaceID, runner: runner}
}
