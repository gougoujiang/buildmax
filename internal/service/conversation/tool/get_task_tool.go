package tool

import (
	"context"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// GetTaskRunner is the interface used by the GetTask tool. Callers implement this
// using TaskStore; results must be scoped to the current conversation context.
type GetTaskRunner interface {
	GetTask(ctx context.Context, scopeID, taskID string) (detail string, err error)
}

type getTaskTool struct {
	scopeID string
	runner  GetTaskRunner
}

// Access implements llm.AccessDeclarer. Reading one task's detail changes
// nothing, so it may overlap its neighbours.
func (t *getTaskTool) Access(_ map[string]any) llm.Access { return llm.AccessReadOnly }

func (t *getTaskTool) Name() string { return ToolNameGetTask }

func (t *getTaskTool) Description() string {
	return "Get detail for one task by task_id. Use this when the user asks about a specific task's status, result, or content. Returns task_id, title, input, status, created_at, last_run_id, and optional output snippet. Fails with a clear error if the task is not in the current conversation."
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
	detail, err := t.runner.GetTask(ctx, t.scopeID, taskID)
	if err != nil {
		return "", err
	}
	return detail, nil
}

// NewGetTaskTool returns a llm.Tool that gets one task's detail. If runner is nil, Execute returns "not configured".
func NewGetTaskTool(scopeID string, runner GetTaskRunner) llm.Tool {
	return &getTaskTool{scopeID: scopeID, runner: runner}
}
