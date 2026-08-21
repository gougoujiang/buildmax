// Package llm holds LLM wire types and the Tool contract used across core services and agent execution.
// Keep this package at the bottom of the dependency graph to avoid import cycles.
package llm

import "context"

// Tool is a capability the agent can invoke by name.
// Both the core agent loop (internal/core/agent) and runtime tools (internal/tool) use this interface.
type Tool interface {
	Name() string
	Description() string
	Parameters() any // JSON schema for arguments (e.g. map[string]any)
	Execute(ctx context.Context, args map[string]any) (result string, err error)
}

// ToolResult is what a tool produces when text alone cannot say it.
//
// Text is always set and is what hooks, traces, the terminal, and token
// estimation read; Parts carries whatever the text could only describe.
type ToolResult struct {
	Text  string
	Parts []ContentPart
}

// MultimodalTool is a Tool whose result can carry non-text content.
//
// It is an optional upgrade rather than a change to Execute because exactly one
// tool needs it — the MCP gateway, which forwards whatever a server returns —
// and widening the contract would make sixteen text-only tools carry a field
// they never set. The agent loop asks for it and falls back to Execute.
type MultimodalTool interface {
	Tool
	ExecuteMultimodal(ctx context.Context, args map[string]any) (ToolResult, error)
}

// ToolRegistry keeps the tools available to an agent run.
type ToolRegistry struct {
	tools []Tool
}

// NewToolRegistry builds an empty registry for the tools available to an agent run.
func NewToolRegistry() ToolRegistry {
	return ToolRegistry{}
}

// AppendTools adds tools to the registry.
func (r *ToolRegistry) AppendTools(tools ...Tool) {
	r.tools = append(r.tools, tools...)
}

// Tools returns the registered tools in append order.
func (r ToolRegistry) Tools() []Tool {
	out := make([]Tool, len(r.tools))
	copy(out, r.tools)
	return out
}

// GetDefs builds the LLM-facing tool definitions.
func (r ToolRegistry) GetDefs() []ToolDef {
	defs := make([]ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Parameters(),
		})
	}
	return defs
}

// Lookup returns the executable tool matching name, or nil if none match.
func (r ToolRegistry) Lookup(name string) Tool {
	for _, tool := range r.tools {
		if tool.Name() == name {
			return tool
		}
	}
	return nil
}

// Access describes what a tool call does to the world. The zero value is
// AccessWrite so an undeclared tool is treated conservatively. Read by the
// permission layer and the tool scheduler; see docs/design/tool-permissions.md.
type Access uint8

const (
	// AccessWrite means the call changes durable or process state.
	AccessWrite Access = iota
	// AccessReadOnly means the call observes and returns.
	AccessReadOnly
)

func (a Access) String() string {
	if a == AccessReadOnly {
		return "read-only"
	}
	return "write"
}

// AccessDeclarer is an optional interface a Tool can implement to classify its
// own calls. Args are passed so the answer can depend on the call.
//
// AccessReadOnly is a claim about effect only. It does not promise that Execute
// is safe on several goroutines at once — CallMcpTool reports what a third
// party says about itself, which is not something this runtime can underwrite.
// A scheduler must require concurrency safety separately.
type AccessDeclarer interface {
	Access(args map[string]any) Access
}

// GrantScoper is an optional interface a Tool can implement to narrow what a
// session grant covers. A tool that dispatches to something else — an MCP
// server, another agent — would otherwise have one approval cover every target
// it can reach.
type GrantScoper interface {
	GrantScope(args map[string]any) string
}
