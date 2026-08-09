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
