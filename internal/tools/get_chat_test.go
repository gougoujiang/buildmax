package tools

import (
	"context"
	"errors"
	"testing"
)

func TestNewGetTaskTool_nilRunner(t *testing.T) {
	tool := NewGetTaskTool("w_1", nil)
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"task_id": "c_1"})
	if err == nil {
		t.Fatal("Execute: expected error when runner is nil")
	}
	if err.Error() != "GetTask not configured" {
		t.Errorf("Execute error = %q, want GetTask not configured", err.Error())
	}
}

func TestNewGetTaskTool_missingTaskID(t *testing.T) {
	tool := NewGetTaskTool("w_1", GetTaskRunnerFunc(func(ctx context.Context, workspaceID, chatID string) (string, error) {
		return "detail", nil
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("Execute: expected error when task_id is missing")
	}
	if err.Error() != "task_id is required" {
		t.Errorf("Execute error = %q", err.Error())
	}
}

func TestNewGetTaskTool_notInWorkspace(t *testing.T) {
	tool := NewGetTaskTool("w_1", GetTaskRunnerFunc(func(ctx context.Context, workspaceID, chatID string) (string, error) {
		return "", errors.New("task not found or not in this workspace")
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"task_id": "c_other"})
	if err == nil {
		t.Fatal("Execute: expected error when task not in workspace")
	}
}

func TestNewGetTaskTool_success(t *testing.T) {
	tool := NewGetTaskTool("w_1", GetTaskRunnerFunc(func(ctx context.Context, workspaceID, chatID string) (string, error) {
		if workspaceID != "w_1" || chatID != "c_abc" {
			return "", errors.New("bad args")
		}
		return "task_id: c_abc\nstatus: SUCCEEDED", nil
	}))
	ctx := context.Background()
	out, err := tool.Execute(ctx, map[string]any{"task_id": "c_abc"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "task_id: c_abc\nstatus: SUCCEEDED" {
		t.Errorf("Execute = %q", out)
	}
}

// GetTaskRunnerFunc adapts a function to GetTaskRunner.
type GetTaskRunnerFunc func(ctx context.Context, workspaceID, chatID string) (string, error)

func (f GetTaskRunnerFunc) GetTask(ctx context.Context, workspaceID, chatID string) (string, error) {
	return f(ctx, workspaceID, chatID)
}
