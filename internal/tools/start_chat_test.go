package tools

import (
	"context"
	"errors"
	"testing"
)

func TestNewStartTaskTool_nilFunc(t *testing.T) {
	tool := NewStartTaskTool("w_1", "u_1", nil)
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"input": "do something"})
	if err == nil {
		t.Fatal("Execute: expected error when fn is nil")
	}
	if err.Error() != "StartTask not configured" {
		t.Errorf("Execute error = %q, want StartTask not configured", err.Error())
	}
}

func TestNewStartTaskTool_missingInput(t *testing.T) {
	tool := NewStartTaskTool("w_1", "u_1", FuncStartTaskRunner(func(ctx context.Context, input string, agentID *string) (string, string, error) {
		return "c_1", "r_1", nil
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("Execute: expected error when input is missing")
	}
}

func TestNewStartTaskTool_success(t *testing.T) {
	tool := NewStartTaskTool("w_1", "u_1", FuncStartTaskRunner(func(ctx context.Context, input string, agentID *string) (string, string, error) {
		if input != "analyze repo" {
			return "", "", errors.New("bad input")
		}
		return "c_abc", "r_xyz", nil
	}))
	ctx := context.Background()
	out, err := tool.Execute(ctx, map[string]any{"input": "analyze repo"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "Background task created and scheduled. task_id: c_abc, run_id: r_xyz. The task will run in the background; the user can check progress or results in Activity or task detail." {
		t.Errorf("Execute = %q", out)
	}
}
