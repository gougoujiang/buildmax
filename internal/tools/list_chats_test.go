package tools

import (
	"context"
	"errors"
	"testing"
)

func TestNewListChatsTool_nilRunner(t *testing.T) {
	tool := NewListChatsTool("w_1", nil)
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("Execute: expected error when runner is nil")
	}
	if err.Error() != "ListChats not configured" {
		t.Errorf("Execute error = %q, want ListChats not configured", err.Error())
	}
}

func TestNewListChatsTool_success(t *testing.T) {
	tool := NewListChatsTool("w_1", ListChatsRunnerFunc(func(ctx context.Context, workspaceID string) (string, error) {
		if workspaceID != "w_1" {
			return "", errors.New("wrong workspace")
		}
		return "1. c_1 | title1 | PENDING | 2025-01-01", nil
	}))
	ctx := context.Background()
	out, err := tool.Execute(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "1. c_1 | title1 | PENDING | 2025-01-01" {
		t.Errorf("Execute = %q", out)
	}
}

func TestNewListChatsTool_runnerError(t *testing.T) {
	tool := NewListChatsTool("w_1", ListChatsRunnerFunc(func(ctx context.Context, workspaceID string) (string, error) {
		return "", errors.New("db error")
	}))
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("Execute: expected error from runner")
	}
}

// ListChatsRunnerFunc adapts a function to ListChatsRunner.
type ListChatsRunnerFunc func(ctx context.Context, workspaceID string) (string, error)

func (f ListChatsRunnerFunc) ListChats(ctx context.Context, workspaceID string) (string, error) {
	return f(ctx, workspaceID)
}
