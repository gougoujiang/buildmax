// Tools for the conversation loop. Each tool is invoked by name when the LLM requests it.
package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"buildmax/internal/llm"
)

// Tool is a capability the conversation loop can invoke by name.
type Tool interface {
	Name() string
	Description() string
	Parameters() any // JSON schema for arguments (e.g. map[string]any)
	Execute(ctx context.Context, args map[string]any) (result string, err error)
}

// GetCurrentDate returns the current date in YYYY-MM-DD format.
type GetCurrentDate struct{}

func (GetCurrentDate) Name() string        { return "get_current_date" }
func (GetCurrentDate) Description() string { return "Returns the current date in YYYY-MM-DD format." }
func (GetCurrentDate) Parameters() any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (GetCurrentDate) Execute(ctx context.Context, args map[string]any) (string, error) {
	return time.Now().Format("2006-01-02"), nil
}

// DefaultTools returns the default tool set for the conversation loop.
func DefaultTools() []Tool {
	return []Tool{GetCurrentDate{}}
}

// StartChatFunc creates a background chat task (Tier 2) and schedules it to run.
// Called by the start_chat tool with (ctx, input, optional agentID). Returns chat_id, run_id, and error.
// When not configured, the server should not add the start_chat tool (or pass nil and BuildToolsWithStartChat will omit it).
type StartChatFunc func(ctx context.Context, input string, agentID *string) (chatID, runID string, err error)

// startChatTool implements Tool for start_chat; it delegates to StartChatFunc.
type startChatTool struct {
	workspaceID string
	userID      string
	fn          StartChatFunc
}

func (t *startChatTool) Name() string        { return "start_chat" }
func (t *startChatTool) Description() string { return "Start a background chat task (Tier 2). The task is created and scheduled to run; it may take a while. Use this when the user asks for a long-running job, analysis, or work that should run in the background. You must tell the user that a background task was started and give them the chat id so they can check progress or results later." }
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
		return "", fmt.Errorf("start_chat not configured")
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

// NewStartChatTool returns a Tool that uses fn to create a chat. If fn is nil, the tool's Execute will return "not configured".
func NewStartChatTool(workspaceID, userID string, fn StartChatFunc) Tool {
	return &startChatTool{workspaceID: workspaceID, userID: userID, fn: fn}
}

// BuildToolsWithStartChat returns default tools plus start_chat when fn is non-nil.
func BuildToolsWithStartChat(workspaceID, userID string, fn StartChatFunc) []Tool {
	tools := DefaultTools()
	if fn != nil {
		tools = append(tools, NewStartChatTool(workspaceID, userID, fn))
	}
	return tools
}

// toolDefs builds LLM tool definitions from a slice of Tool.
func toolDefs(tools []Tool) []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return defs
}

// executeTool runs one tool call: parses arguments, calls tool.Execute, returns result or error message.
func executeTool(ctx context.Context, tool Tool, tc llm.ToolCall) string {
	var args map[string]any
	if tc.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			return fmt.Sprintf("error: invalid arguments: %v", err)
		}
	}
	if args == nil {
		args = make(map[string]any)
	}
	result, err := tool.Execute(ctx, args)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return result
}
