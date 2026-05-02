package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"buildmax/internal/core/model"
)

// BuildToolDefs builds LLM tool definitions from a slice of Tool. Used by both the CLI agent and the conversation loop.
func BuildToolDefs(toolList []model.Tool) []model.ToolDef {
	defs := make([]model.ToolDef, 0, len(toolList))
	for _, t := range toolList {
		defs = append(defs, model.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return defs
}

// BuildToolsByName builds a lookup table for executing LLM-selected tools by name.
func BuildToolsByName(toolList []model.Tool) map[string]model.Tool {
	byName := make(map[string]model.Tool, len(toolList))
	for _, t := range toolList {
		byName[t.Name()] = t
	}
	return byName
}

// ExecuteTool runs one tool call: parses tc.Arguments, calls tool.Execute, and returns the result or an error string.
// Used by both the CLI agent (which then appends to session) and the conversation loop.
func ExecuteTool(ctx context.Context, t model.Tool, tc model.ToolCall) string {
	var args map[string]any
	if tc.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			return fmt.Sprintf("error: invalid arguments: %v", err)
		}
	}
	if args == nil {
		args = make(map[string]any)
	}
	result, err := t.Execute(ctx, args)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return result
}

// toolCallsSummary returns a short summary of tool calls for logging.
func toolCallsSummary(calls []model.ToolCall) []string {
	s := make([]string, 0, len(calls))
	for _, tc := range calls {
		args := tc.Arguments
		if len(args) > 80 {
			args = args[:80] + "..."
		}
		s = append(s, tc.Name+": "+strings.TrimSpace(args))
	}
	return s
}
