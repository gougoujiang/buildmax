package tools

import (
	"context"
	"errors"
	"testing"
)

func TestNewStartChatTool_nilFunc(t *testing.T) {
	tool := NewStartChatTool("w_1", "u_1", nil)
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"input": "do something"})
	if err == nil {
		t.Fatal("Execute: expected error when fn is nil")
	}
	if err.Error() != "StartChat not configured" {
		t.Errorf("Execute error = %q, want StartChat not configured", err.Error())
	}
}

func TestNewStartChatTool_missingInput(t *testing.T) {
	tool := NewStartChatTool("w_1", "u_1", FuncStartChatRunner(func(ctx context.Context, input string, agentID *string) (string, string, error) {
		return "c_1", "r_1", nil
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("Execute: expected error when input is missing")
	}
}

func TestNewStartChatTool_success(t *testing.T) {
	tool := NewStartChatTool("w_1", "u_1", FuncStartChatRunner(func(ctx context.Context, input string, agentID *string) (string, string, error) {
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
	if out != "Background chat task created and scheduled. chat_id: c_abc, run_id: r_xyz. The task will run in the background; the user can check progress or results in the Activity or chat detail." {
		t.Errorf("Execute = %q", out)
	}
}
