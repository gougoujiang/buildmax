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
