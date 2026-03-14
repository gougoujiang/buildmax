package tools

import (
	"context"
	"errors"
	"testing"
)

func TestNewGetChatTool_nilRunner(t *testing.T) {
	tool := NewGetChatTool("w_1", nil)
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"chat_id": "c_1"})
	if err == nil {
		t.Fatal("Execute: expected error when runner is nil")
	}
	if err.Error() != "GetChat not configured" {
		t.Errorf("Execute error = %q, want GetChat not configured", err.Error())
	}
}

func TestNewGetChatTool_missingChatID(t *testing.T) {
	tool := NewGetChatTool("w_1", GetChatRunnerFunc(func(ctx context.Context, workspaceID, chatID string) (string, error) {
		return "detail", nil
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("Execute: expected error when chat_id is missing")
	}
	if err.Error() != "chat_id is required" {
		t.Errorf("Execute error = %q", err.Error())
	}
}

func TestNewGetChatTool_notInWorkspace(t *testing.T) {
	tool := NewGetChatTool("w_1", GetChatRunnerFunc(func(ctx context.Context, workspaceID, chatID string) (string, error) {
		return "", errors.New("chat not found or not in this workspace")
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"chat_id": "c_other"})
	if err == nil {
		t.Fatal("Execute: expected error when chat not in workspace")
	}
}

func TestNewGetChatTool_success(t *testing.T) {
	tool := NewGetChatTool("w_1", GetChatRunnerFunc(func(ctx context.Context, workspaceID, chatID string) (string, error) {
		if workspaceID != "w_1" || chatID != "c_abc" {
			return "", errors.New("bad args")
		}
		return "chat_id: c_abc\nstatus: SUCCEEDED", nil
	}))
	ctx := context.Background()
	out, err := tool.Execute(ctx, map[string]any{"chat_id": "c_abc"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "chat_id: c_abc\nstatus: SUCCEEDED" {
		t.Errorf("Execute = %q", out)
	}
}

// GetChatRunnerFunc adapts a function to GetChatRunner.
type GetChatRunnerFunc func(ctx context.Context, workspaceID, chatID string) (string, error)

func (f GetChatRunnerFunc) GetChat(ctx context.Context, workspaceID, chatID string) (string, error) {
	return f(ctx, workspaceID, chatID)
}
