package conversation

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewStartTaskTool_nilFunc(t *testing.T) {
	tool := NewStartTaskTool("w_1", "u_1", nil, nil)
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
		return "t_1", "r_1", nil
	}), nil)
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
		return "t_abc", "r_xyz", nil
	}), nil)
	ctx := context.Background()
	out, err := tool.Execute(ctx, map[string]any{"input": "analyze repo"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "Background task created and scheduled (task_id: t_abc). The task is now running in the background." {
		t.Errorf("Execute = %q", out)
	}
}

func TestNewStartTaskTool_descriptionWithAgents(t *testing.T) {
	agents := []AgentSummary{
		{ID: "a_111", Name: "code-reviewer", Description: "reviews code"},
		{ID: "a_222", Name: "test-writer", Description: ""},
	}
	tool := NewStartTaskTool("w_1", "u_1", nil, agents)
	desc := tool.Description()
	if !strings.Contains(desc, "code-reviewer (id: a_111) - reviews code") {
		t.Errorf("description missing agent with description: %s", desc)
	}
	if !strings.Contains(desc, "test-writer (id: a_222)") {
		t.Errorf("description missing agent without description: %s", desc)
	}
	if strings.Contains(desc, "test-writer (id: a_222) -") {
		t.Errorf("description should not have trailing dash for empty description: %s", desc)
	}
}

func TestNewStartTaskTool_descriptionNoAgents(t *testing.T) {
	tool := NewStartTaskTool("w_1", "u_1", nil, nil)
	desc := tool.Description()
	if strings.Contains(desc, "Available agents") {
		t.Errorf("description should not have agents section when nil: %s", desc)
	}
}
