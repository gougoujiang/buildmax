package tools

import (
	"context"
	"errors"
	"testing"
)

func TestNewContinueChatTool_nilRunner(t *testing.T) {
	tool := NewContinueChatTool("w_1", "u_1", nil)
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"chat_id": "c_1", "input": "follow up"})
	if err == nil {
		t.Fatal("Execute: expected error when runner is nil")
	}
	if err.Error() != "ContinueChat not configured" {
		t.Errorf("Execute error = %q, want ContinueChat not configured", err.Error())
	}
}

func TestNewContinueChatTool_missingChatID(t *testing.T) {
	tool := NewContinueChatTool("w_1", "u_1", ContinueChatRunnerFunc(func(ctx context.Context, workspaceID, userID, chatID, input string) (string, error) {
		return "r_1", nil
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"input": "follow up"})
	if err == nil {
		t.Fatal("Execute: expected error when chat_id is missing")
	}
	if err.Error() != "chat_id is required" {
		t.Errorf("Execute error = %q", err.Error())
	}
}

func TestNewContinueChatTool_missingInput(t *testing.T) {
	tool := NewContinueChatTool("w_1", "u_1", ContinueChatRunnerFunc(func(ctx context.Context, workspaceID, userID, chatID, input string) (string, error) {
		return "r_1", nil
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"chat_id": "c_1"})
	if err == nil {
		t.Fatal("Execute: expected error when input is missing")
	}
	if err.Error() != "input is required" {
		t.Errorf("Execute error = %q", err.Error())
	}
}

func TestNewContinueChatTool_notInWorkspace(t *testing.T) {
	tool := NewContinueChatTool("w_1", "u_1", ContinueChatRunnerFunc(func(ctx context.Context, workspaceID, userID, chatID, input string) (string, error) {
		return "", errors.New("chat not found or not in this workspace")
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"chat_id": "c_other", "input": "retry"})
	if err == nil {
		t.Fatal("Execute: expected error when chat not in workspace")
	}
}

func TestNewContinueChatTool_success(t *testing.T) {
	tool := NewContinueChatTool("w_1", "u_1", ContinueChatRunnerFunc(func(ctx context.Context, workspaceID, userID, chatID, input string) (string, error) {
		if workspaceID != "w_1" || userID != "u_1" || chatID != "c_xyz" || input != "add this" {
			return "", errors.New("bad args")
		}
		return "r_new", nil
	}))
	ctx := context.Background()
	out, err := tool.Execute(ctx, map[string]any{"chat_id": "c_xyz", "input": "add this"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := "Follow-up scheduled. chat_id: c_xyz, run_id: r_new. The new run will execute in the background; the user can check progress in Activity or chat detail."
	if out != want {
		t.Errorf("Execute = %q, want %q", out, want)
	}
}

// ContinueChatRunnerFunc adapts a function to ContinueChatRunner.
type ContinueChatRunnerFunc func(ctx context.Context, workspaceID, userID, chatID, input string) (string, error)

func (f ContinueChatRunnerFunc) ContinueChat(ctx context.Context, workspaceID, userID, chatID, input string) (string, error) {
	return f(ctx, workspaceID, userID, chatID, input)
}
