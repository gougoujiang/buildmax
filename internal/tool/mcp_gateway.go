package tool

import (
	"context"
	"fmt"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/infra/mcp"
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
func GatewayTools(reg *mcp.Registry) []llm.Tool {
	return []llm.Tool{
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
	srv, err := parseRequiredString(args, "server")
	if err != nil {
		return "", err
	}
	toolName, err := parseRequiredString(args, "tool_name")
	if err != nil {
		return "", err
	}
	return t.reg.ToolSchemaDetail(srv, toolName)
}

type callMCPToolTool struct {
	reg *mcp.Registry
}

func (t *callMCPToolTool) Name() string { return ToolNameCallMCPTool }

func (t *callMCPToolTool) Description() string { return callMCPToolDesc }

func (t *callMCPToolTool) Parameters() any { return callMCPToolParams }

func (t *callMCPToolTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	srv, err := parseRequiredString(args, "server")
	if err != nil {
		return "", err
	}
	toolName, err := parseRequiredString(args, "tool_name")
	if err != nil {
		return "", err
	}
	var toolArgs map[string]any
	if v, ok := args["arguments"]; ok && v != nil {
		m, ok := v.(map[string]any)
		if !ok {
			return "", fmt.Errorf("arguments must be a JSON object")
		}
		toolArgs = m
	}
	return t.reg.CallMcp(ctx, srv, toolName, toolArgs)
}
