package hook

import (
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
)

// NewDriverRegistry builds the per-type driver map used by the HookManager.
// The command driver is always included. HTTP is included unconditionally
// since it has no external dependencies. MCP and Prompt drivers are only
// included when the corresponding caller is supplied in deps; missing
// callers log a single warning at startup and the corresponding entries
// are skipped at dispatch time.
func NewDriverRegistry(deps Deps) map[string]Driver {
	registry := map[string]Driver{
		corehook.TypeCommand: NewCommandDriver(),
		corehook.TypeHTTP:    NewHTTPDriver(),
	}
	if deps.MCPCaller != nil {
		registry[corehook.TypeMCP] = NewMCPDriver(deps.MCPCaller)
	} else {
		componentLog().Debug("no MCP caller; mcp_tool hooks disabled")
	}
	if deps.LLMCaller != nil {
		registry[corehook.TypePrompt] = NewPromptDriver(deps.LLMCaller)
	} else {
		componentLog().Debug("no LLM caller; prompt hooks disabled")
	}
	return registry
}
