// ContinueChat is a Tier 1 conversation tool that adds a follow-up run to an existing chat.
package tools

import (
	"context"
	"fmt"

	"buildmax/internal/core"
)

// ContinueChatRunner is the interface used by the ContinueChat tool. Callers implement this
// using ChatService.CreateRun; must verify chat belongs to workspace before creating run.
type ContinueChatRunner interface {
	ContinueChat(ctx context.Context, workspaceID, userID, chatID, input string) (runID string, err error)
}

type continueChatTool struct {
	workspaceID string
	userID      string
	runner      ContinueChatRunner
}

func (t *continueChatTool) Name() string { return ToolNameContinueChat }

func (t *continueChatTool) Description() string {
	return "Continue an existing chat with a follow-up message. Use this when the user wants to add to a chat, try again with different input, or follow up on a previous result (e.g. 'add this to chat c_xyz', 'run again with ...'). Creates a new run for that chat. Tell the user the chat_id and run_id when done. Fails if the chat is not in the current workspace."
}

func (t *continueChatTool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"chat_id": map[string]any{
				"type":        "string",
				"description": "The chat id to continue (required).",
			},
			"input": map[string]any{
				"type":        "string",
				"description": "The follow-up message or instruction (required).",
			},
		},
		"required": []any{"chat_id", "input"},
	}
}

func (t *continueChatTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.runner == nil {
		return "", fmt.Errorf("%s not configured", ToolNameContinueChat)
	}
	chatID, _ := args["chat_id"].(string)
	if chatID == "" {
		return "", fmt.Errorf("chat_id is required")
	}
	inputVal, _ := args["input"].(string)
	if inputVal == "" {
		return "", fmt.Errorf("input is required")
	}
	runID, err := t.runner.ContinueChat(ctx, t.workspaceID, t.userID, chatID, inputVal)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Follow-up scheduled. chat_id: %s, run_id: %s. The new run will execute in the background; the user can check progress in Activity or chat detail.", chatID, runID), nil
}

// NewContinueChatTool returns a core.Tool that continues a chat with a follow-up. If runner is nil, Execute returns "not configured".
func NewContinueChatTool(workspaceID, userID string, runner ContinueChatRunner) core.Tool {
	return &continueChatTool{workspaceID: workspaceID, userID: userID, runner: runner}
}
