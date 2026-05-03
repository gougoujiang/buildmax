package tool

import (
	"context"
	"errors"
	"testing"
)

func TestNewListTasksTool_nilRunner(t *testing.T) {
	tool := NewListTasksTool("w_1", nil)
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("Execute: expected error when runner is nil")
	}
	if err.Error() != "ListTasks not configured" {
		t.Errorf("Execute error = %q, want ListTasks not configured", err.Error())
	}
}

func TestNewListTasksTool_success(t *testing.T) {
	tool := NewListTasksTool("w_1", ListTasksRunnerFunc(func(ctx context.Context, workspaceID string) (string, error) {
		if workspaceID != "w_1" {
			return "", errors.New("wrong workspace")
		}
		return "1. t_1 | title1 | PENDING | 2025-01-01", nil
	}))
	ctx := context.Background()
	out, err := tool.Execute(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "1. t_1 | title1 | PENDING | 2025-01-01" {
		t.Errorf("Execute = %q", out)
	}
}

func TestNewListTasksTool_runnerError(t *testing.T) {
	tool := NewListTasksTool("w_1", ListTasksRunnerFunc(func(ctx context.Context, workspaceID string) (string, error) {
		return "", errors.New("db error")
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("Execute: expected error from runner")
	}
}

// ListTasksRunnerFunc adapts a function to ListTasksRunner.
type ListTasksRunnerFunc func(ctx context.Context, workspaceID string) (string, error)

func (f ListTasksRunnerFunc) ListTasks(ctx context.Context, workspaceID string) (string, error) {
	return f(ctx, workspaceID)
}
