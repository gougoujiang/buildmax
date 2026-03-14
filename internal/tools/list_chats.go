// ListChats is a Tier 1 conversation tool that lists recent chats in the workspace.
package tools

import (
	"context"
	"fmt"

	"buildmax/internal/core"
)

// ListChatsRunner is the interface used by the ListChats tool. Callers implement this
// using ChatStore; the conversation layer passes the runner when building tools.
type ListChatsRunner interface {
	ListChats(ctx context.Context, workspaceID string) (summary string, err error)
}

type listChatsTool struct {
	workspaceID string
	runner      ListChatsRunner
}

func (t *listChatsTool) Name() string { return ToolNameListChats }

func (t *listChatsTool) Description() string {
	return "List recent chats in the current workspace (at most 10, most recent first). Use this when the user asks what chats they have, what background tasks exist, or to see recent activity. Returns chat_id, title/snippet, status, created_at per chat."
}

func (t *listChatsTool) Parameters() any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []any{},
	}
}

func (t *listChatsTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.runner == nil {
		return "", fmt.Errorf("%s not configured", ToolNameListChats)
	}
	summary, err := t.runner.ListChats(ctx, t.workspaceID)
	if err != nil {
		return "", err
	}
	return summary, nil
}

// NewListChatsTool returns a core.Tool that lists recent chats. If runner is nil, Execute returns "not configured".
func NewListChatsTool(workspaceID string, runner ListChatsRunner) core.Tool {
	return &listChatsTool{workspaceID: workspaceID, runner: runner}
}
