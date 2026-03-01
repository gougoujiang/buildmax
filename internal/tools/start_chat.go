// Conversation tool: start_chat. Creates and schedules a Tier 2 background chat task.
package tools

import (
	"context"
	"fmt"

	"buildmax/internal/core"
)

// StartChatRunner is the interface used by the start_chat tool to create a background chat.
// Callers (e.g. server) implement this from their config; the conversation layer passes the runner
// into RunLoop/RunLoopStream and the agent builds the StartChat tool from it.
type StartChatRunner interface {
	StartChat(ctx context.Context, workspaceID, userID, input string, agentID *string) (chatID, runID string, err error)
}

// StartChatFunc creates a background chat task (Tier 2) and schedules it to run.
// Used as a convenience; wrap with FuncStartChatRunner to implement StartChatRunner.
type StartChatFunc func(ctx context.Context, input string, agentID *string) (chatID, runID string, err error)

// FuncStartChatRunner adapts a StartChatFunc to StartChatRunner (ignores workspaceID/userID in the call).
func FuncStartChatRunner(fn StartChatFunc) StartChatRunner {
	if fn == nil {
		return nil
	}
	return &funcStartChatRunner{fn: fn}
}

type funcStartChatRunner struct{ fn StartChatFunc }

func (f *funcStartChatRunner) StartChat(ctx context.Context, _, _, input string, agentID *string) (string, string, error) {
	return f.fn(ctx, input, agentID)
}

// startChatTool implements Tool for StartChat; it delegates to StartChatRunner.
type startChatTool struct {
	workspaceID string
	userID      string
	runner      StartChatRunner
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
	if t.runner == nil {
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
	chatID, runID, err := t.runner.StartChat(ctx, t.workspaceID, t.userID, inputVal, agentID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Background chat task created and scheduled. chat_id: %s, run_id: %s. The task will run in the background; the user can check progress or results in the Activity or chat detail.", chatID, runID), nil
}

// NewStartChatTool returns a core.Tool that uses runner to create a chat. If runner is nil, Execute returns "not configured".
func NewStartChatTool(workspaceID, userID string, runner StartChatRunner) core.Tool {
	return &startChatTool{workspaceID: workspaceID, userID: userID, runner: runner}
}
