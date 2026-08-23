package tool

import (
	"context"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// ListTasksRunner is the interface used by the ListTasks tool. Callers implement this
// using TaskStore; the conversation layer passes the runner when building tools.
type ListTasksRunner interface {
	ListTasks(ctx context.Context, scopeID string) (summary string, err error)
}

type listTasksTool struct {
	scopeID string
	runner  ListTasksRunner
}

// Access implements llm.AccessDeclarer. Listing reads the task store and
// returns; it changes nothing, so it may overlap its neighbours.
func (t *listTasksTool) Access(_ map[string]any) llm.Access { return llm.AccessReadOnly }

func (t *listTasksTool) Name() string { return ToolNameListTasks }

func (t *listTasksTool) Description() string {
	return "List recent tasks in the current conversation (at most 10, most recent first). Use this when the user asks what tasks they have, what background tasks exist, or to see recent activity. Returns task_id, title/snippet, status, created_at per task."
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
	summary, err := t.runner.ListTasks(ctx, t.scopeID)
	if err != nil {
		return "", err
	}
	return summary, nil
}

// NewListTasksTool returns a llm.Tool that lists recent tasks. If runner is nil, Execute returns "not configured".
func NewListTasksTool(scopeID string, runner ListTasksRunner) llm.Tool {
	return &listTasksTool{scopeID: scopeID, runner: runner}
}
