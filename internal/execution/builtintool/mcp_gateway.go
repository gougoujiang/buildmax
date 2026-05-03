package builtintool

import (
	"context"
	"fmt"
	"strings"

	"buildmax/internal/core/model"
	"buildmax/internal/infra/mcp"
)

const (
	ToolNameLoadMCPTools = "LoadMcpTools"
	ToolNameCallMCPTool  = "CallMcpTool"
)

var loadMCPToolsParams = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"server": map[string]any{
			"type":        "string",
			"description": "MCP server id as configured under mcpServers in mcp.json",
		},
		"tool_name": map[string]any{
			"type":        "string",
			"description": "Tool name on that server (see LoadMcpTools catalog)",
		},
	},
	"required": []string{"server", "tool_name"},
}

var callMCPToolParams = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"server": map[string]any{
			"type":        "string",
			"description": "MCP server id from mcp.json",
		},
		"tool_name": map[string]any{
			"type":        "string",
			"description": "Name of the tool to invoke on that server",
		},
		"arguments": map[string]any{
			"type":        "object",
			"description": "Arguments object for the tool; use LoadMcpTools to get the input_schema first",
		},
	},
	"required": []string{"server", "tool_name"},
}

const loadMCPToolsDescPrefix = `Load the full input JSON Schema and description for one MCP tool before calling CallMcpTool.

Catalog of configured MCP servers and tools (name and short description only):`

const callMCPToolDesc = `Invoke an MCP tool on a configured server. Use LoadMcpTools(server, tool_name) first when you need the exact argument schema. Pass tool-specific fields inside arguments (a JSON object).`

// GatewayTools returns LoadMcpTools and CallMcpTool bound to reg.
func GatewayTools(reg *mcp.Registry) []model.Tool {
	return []model.Tool{
		&loadMCPToolsTool{reg: reg},
		&callMCPToolTool{reg: reg},
	}
}

type loadMCPToolsTool struct {
	reg *mcp.Registry
}

func (t *loadMCPToolsTool) Name() string { return ToolNameLoadMCPTools }

func (t *loadMCPToolsTool) Description() string {
	return loadMCPToolsDescPrefix + "\n\n" + t.reg.Catalog()
}

func (t *loadMCPToolsTool) Parameters() any { return loadMCPToolsParams }

func (t *loadMCPToolsTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	srv, _ := args["server"].(string)
	tn, _ := args["tool_name"].(string)
	srv = strings.TrimSpace(srv)
	tn = strings.TrimSpace(tn)
	if srv == "" || tn == "" {
		return "", fmt.Errorf("server and tool_name are required")
	}
	return t.reg.ToolSchemaDetail(srv, tn)
}

type callMCPToolTool struct {
	reg *mcp.Registry
}

func (t *callMCPToolTool) Name() string { return ToolNameCallMCPTool }

func (t *callMCPToolTool) Description() string { return callMCPToolDesc }

func (t *callMCPToolTool) Parameters() any { return callMCPToolParams }

func (t *callMCPToolTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	srv, _ := args["server"].(string)
	tn, _ := args["tool_name"].(string)
	srv = strings.TrimSpace(srv)
	tn = strings.TrimSpace(tn)
	if srv == "" || tn == "" {
		return "", fmt.Errorf("server and tool_name are required")
	}
	var toolArgs map[string]any
	if v, ok := args["arguments"]; ok && v != nil {
		switch m := v.(type) {
		case map[string]any:
			toolArgs = m
		default:
			return "", fmt.Errorf("arguments must be a JSON object")
		}
	}
	return t.reg.CallMcp(ctx, srv, tn, toolArgs)
}
