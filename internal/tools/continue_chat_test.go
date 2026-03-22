package tools

import (
	"context"
	"errors"
	"testing"
)

func TestNewContinueTaskTool_nilRunner(t *testing.T) {
	tool := NewContinueTaskTool("w_1", "u_1", nil)
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"task_id": "t_1", "input": "follow up"})
	if err == nil {
		t.Fatal("Execute: expected error when runner is nil")
	}
	if err.Error() != "ContinueTask not configured" {
		t.Errorf("Execute error = %q, want ContinueTask not configured", err.Error())
	}
}

func TestNewContinueTaskTool_missingTaskID(t *testing.T) {
	tool := NewContinueTaskTool("w_1", "u_1", ContinueTaskRunnerFunc(func(ctx context.Context, workspaceID, userID, chatID, input string) (string, error) {
		return "r_1", nil
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"input": "follow up"})
	if err == nil {
		t.Fatal("Execute: expected error when task_id is missing")
	}
	if err.Error() != "task_id is required" {
		t.Errorf("Execute error = %q", err.Error())
	}
}

func TestNewContinueTaskTool_missingInput(t *testing.T) {
	tool := NewContinueTaskTool("w_1", "u_1", ContinueTaskRunnerFunc(func(ctx context.Context, workspaceID, userID, chatID, input string) (string, error) {
		return "r_1", nil
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"task_id": "t_1"})
	if err == nil {
		t.Fatal("Execute: expected error when input is missing")
	}
	if err.Error() != "input is required" {
		t.Errorf("Execute error = %q", err.Error())
	}
}

func TestNewContinueTaskTool_notInWorkspace(t *testing.T) {
	tool := NewContinueTaskTool("w_1", "u_1", ContinueTaskRunnerFunc(func(ctx context.Context, workspaceID, userID, chatID, input string) (string, error) {
		return "", errors.New("task not found or not in this workspace")
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"task_id": "t_other", "input": "retry"})
	if err == nil {
		t.Fatal("Execute: expected error when task not in workspace")
	}
}

func TestNewContinueTaskTool_success(t *testing.T) {
	tool := NewContinueTaskTool("w_1", "u_1", ContinueTaskRunnerFunc(func(ctx context.Context, workspaceID, userID, chatID, input string) (string, error) {
		if workspaceID != "w_1" || userID != "u_1" || chatID != "t_xyz" || input != "add this" {
			return "", errors.New("bad args")
		}
		return "r_new", nil
	}))
	ctx := context.Background()
	out, err := tool.Execute(ctx, map[string]any{"task_id": "t_xyz", "input": "add this"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := "Follow-up scheduled. task_id: t_xyz, run_id: r_new. The new run will execute in the background; the user can check progress in Activity or task detail."
	if out != want {
		t.Errorf("Execute = %q, want %q", out, want)
	}
}

// ContinueTaskRunnerFunc adapts a function to ContinueTaskRunner.
type ContinueTaskRunnerFunc func(ctx context.Context, workspaceID, userID, chatID, input string) (string, error)

func (f ContinueTaskRunnerFunc) ContinueTask(ctx context.Context, workspaceID, userID, chatID, input string) (string, error) {
	return f(ctx, workspaceID, userID, chatID, input)
}
