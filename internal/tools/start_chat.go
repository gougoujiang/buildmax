// Conversation tool: start_chat. Creates and schedules a Tier 2 background chat task.
package tools

import (
	"context"
	"fmt"

	"buildmax/internal/core"
)

// StartChatFunc creates a background chat task (Tier 2) and schedules it to run.
// Called by the start_chat tool with (ctx, input, optional agentID). Returns chat_id, run_id, and error.
// When not configured, the server should not add the start_chat tool (or pass nil and BuildConversationToolsWithStartChat will omit it).
type StartChatFunc func(ctx context.Context, input string, agentID *string) (chatID, runID string, err error)

// startChatTool implements Tool for StartChat; it delegates to StartChatFunc.
type startChatTool struct {
	workspaceID string
	userID      string
	fn          StartChatFunc
}

func (t *startChatTool) Name() string { return ToolNameStartChat }

func (t *startChatTool) Description() string {
	return "Start a background chat task (Tier 2). The task is created and scheduled to run; it may take a while. Use this when the user asks for a long-running job, analysis, or work that should run in the background. You must tell the user that a background task was started and give them the chat id so they can check progress or results later."
}

func (t *startChatTool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "string",
				"description": "The task input or instruction for the background chat (required).",
			},
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Optional workspace agent id to run the chat with.",
			},
		},
		"required": []any{"input"},
	}
}

func (t *startChatTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.fn == nil {
		return "", fmt.Errorf("%s not configured", ToolNameStartChat)
	}
	inputVal, _ := args["input"].(string)
	if inputVal == "" {
		return "", fmt.Errorf("input is required")
	}
	var agentID *string
	if aid, ok := args["agent_id"].(string); ok && aid != "" {
		agentID = &aid
	}
	chatID, runID, err := t.fn(ctx, inputVal, agentID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Background chat task created and scheduled. chat_id: %s, run_id: %s. The task will run in the background; the user can check progress or results in the Activity or chat detail.", chatID, runID), nil
}

// NewStartChatTool returns a core.Tool that uses fn to create a chat. If fn is nil, Execute returns "not configured".
func NewStartChatTool(workspaceID, userID string, fn StartChatFunc) core.Tool {
	return &startChatTool{workspaceID: workspaceID, userID: userID, fn: fn}
}

// DefaultConversationTools returns the default tool set for the conversation loop (GetCurrentDate only).
func DefaultConversationTools() []core.Tool {
	return []core.Tool{GetCurrentDate{}}
}

// BuildConversationToolsWithStartChat returns default conversation tools plus StartChat when fn is non-nil.
func BuildConversationToolsWithStartChat(workspaceID, userID string, fn StartChatFunc) []core.Tool {
	toolList := DefaultConversationTools()
	if fn != nil {
		toolList = append(toolList, NewStartChatTool(workspaceID, userID, fn))
	}
	return toolList
}
