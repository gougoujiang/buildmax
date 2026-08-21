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

// Access implements llm.AccessDeclarer.
func (t *loadMCPToolsTool) Access(_ map[string]any) llm.Access { return llm.AccessReadOnly }

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

// GrantScope implements llm.GrantScoper. Without it, approving one MCP call for
// the session would approve every tool on every configured server.
func (t *callMCPToolTool) GrantScope(args map[string]any) string {
	server, _ := args["server"].(string)
	name, _ := args["tool_name"].(string)
	if server == "" || name == "" {
		return ""
	}
	return server + "/" + name
}

// Access implements llm.AccessDeclarer, reporting what the server says the
// named tool does. A server that says nothing is a write.
func (t *callMCPToolTool) Access(args map[string]any) llm.Access {
	if t.readOnly(args) {
		return llm.AccessReadOnly
	}
	return llm.AccessWrite
}

// CheckArgs implements llm.ArgChecker. Access above already stops a read-only
// call from prompting; this exists for the other half, which the derived tier
// cannot reach: a write must also be refused where nobody can be asked, and
// layer 4 does not run on an autonomous surface.
//
// Returning Allow here is an abstention, not a grant — the read-only case falls
// through to Access.
func (t *callMCPToolTool) CheckArgs(args map[string]any) llm.ToolAction {
	if t.readOnly(args) {
		return llm.ToolActionAllow
	}
	return llm.ToolActionAsk
}

// readOnly reports the server's claim about the named tool. The claim decides
// whether the runtime asks; it never decides whether the call is trusted.
func (t *callMCPToolTool) readOnly(args map[string]any) bool {
	server, _ := args["server"].(string)
	name, _ := args["tool_name"].(string)
	if server == "" || name == "" {
		return false
	}
	return t.reg.ToolIsReadOnly(server, name)
}

func (t *callMCPToolTool) Name() string { return ToolNameCallMCPTool }

func (t *callMCPToolTool) Description() string { return callMCPToolDesc }

func (t *callMCPToolTool) Parameters() any { return callMCPToolParams }

func (t *callMCPToolTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	out, err := t.ExecuteMultimodal(ctx, args)
	return out.Text, err
}

// ExecuteMultimodal keeps content an MCP server returned that text cannot hold.
// This is the one tool that forwards results it did not produce, so it is the
// one that needs the wider contract.
func (t *callMCPToolTool) ExecuteMultimodal(ctx context.Context, args map[string]any) (llm.ToolResult, error) {
	srv, err := parseRequiredString(args, "server")
	if err != nil {
		return llm.ToolResult{}, err
	}
	toolName, err := parseRequiredString(args, "tool_name")
	if err != nil {
		return llm.ToolResult{}, err
	}
	var toolArgs map[string]any
	if v, ok := args["arguments"]; ok && v != nil {
		m, ok := v.(map[string]any)
		if !ok {
			return llm.ToolResult{}, fmt.Errorf("arguments must be a JSON object")
		}
		toolArgs = m
	}
	return t.reg.CallMcpMultimodal(ctx, srv, toolName, toolArgs)
}

var _ llm.MultimodalTool = (*callMCPToolTool)(nil)
