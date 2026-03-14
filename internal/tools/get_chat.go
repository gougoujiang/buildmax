// GetChat is a Tier 1 conversation tool that returns detail for one chat by id.
package tools

import (
	"context"
	"fmt"

	"buildmax/internal/core"
)

// GetChatRunner is the interface used by the GetChat tool. Callers implement this
// using ChatStore; must return workspace-scoped results only.
type GetChatRunner interface {
	GetChat(ctx context.Context, workspaceID, chatID string) (detail string, err error)
}

type getChatTool struct {
	workspaceID string
	runner      GetChatRunner
}

func (t *getChatTool) Name() string { return ToolNameGetChat }

func (t *getChatTool) Description() string {
	return "Get detail for one chat by chat_id. Use this when the user asks about a specific chat's status, result, or content. Returns chat_id, title, input, status, created_at, last_run_id, and optional output snippet. Fails with a clear error if the chat is not in the current workspace."
}

func (t *getChatTool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"chat_id": map[string]any{
				"type":        "string",
				"description": "The chat id (required).",
			},
		},
		"required": []any{"chat_id"},
	}
}

func (t *getChatTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.runner == nil {
		return "", fmt.Errorf("%s not configured", ToolNameGetChat)
	}
	chatID, _ := args["chat_id"].(string)
	if chatID == "" {
		return "", fmt.Errorf("chat_id is required")
	}
	detail, err := t.runner.GetChat(ctx, t.workspaceID, chatID)
	if err != nil {
		return "", err
	}
	return detail, nil
}

// NewGetChatTool returns a core.Tool that gets one chat's detail. If runner is nil, Execute returns "not configured".
func NewGetChatTool(workspaceID string, runner GetChatRunner) core.Tool {
	return &getChatTool{workspaceID: workspaceID, runner: runner}
}
