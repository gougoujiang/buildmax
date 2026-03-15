// ListTasks is a Tier 1 conversation tool that lists recent tasks in the workspace.
package tools

import (
	"context"
	"fmt"

	"buildmax/internal/core"
)

// ListTasksRunner is the interface used by the ListTasks tool. Callers implement this
// using TaskStore; the conversation layer passes the runner when building tools.
type ListTasksRunner interface {
	ListTasks(ctx context.Context, workspaceID string) (summary string, err error)
}

type listTasksTool struct {
	workspaceID string
	runner      ListTasksRunner
}

func (t *listTasksTool) Name() string { return ToolNameListTasks }

func (t *listTasksTool) Description() string {
	return "List recent tasks in the current workspace (at most 10, most recent first). Use this when the user asks what tasks they have, what background tasks exist, or to see recent activity. Returns task_id, title/snippet, status, created_at per task."
}

func (t *listTasksTool) Parameters() any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []any{},
	}
}

func (t *listTasksTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.runner == nil {
		return "", fmt.Errorf("%s not configured", ToolNameListTasks)
	}
	summary, err := t.runner.ListTasks(ctx, t.workspaceID)
	if err != nil {
		return "", err
	}
	return summary, nil
}

// NewListTasksTool returns a core.Tool that lists recent tasks. If runner is nil, Execute returns "not configured".
func NewListTasksTool(workspaceID string, runner ListTasksRunner) core.Tool {
	return &listTasksTool{workspaceID: workspaceID, runner: runner}
}
