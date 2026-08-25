package hook

import (
	"context"
	"log/slog"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
)

// Identity belongs in an attr, not in every message string.
func componentLog() *slog.Logger { return slog.With("component", "hook") }

// Driver executes one configured hook entry and returns the resulting
// HookOutput. Implementations are stateless except for cheap caches
// (e.g. compiled regex). Dispatch concerns — matcher evaluation, entry
// ordering, first-block-wins aggregation — live in the manager, not here.
//
// A Driver must fail open on internal errors: hook failures should never
// silently break the agent loop. Returning a blocking HookOutput is reserved
// for explicit policy decisions surfaced by the hook itself (e.g. command
// exit 2, HTTP 422, JSON {"decision":"block"}).
type Driver interface {
	// Type returns the value the manager uses to address this driver. Must
	// match the values produced by corehook.Entry.ResolvedType.
	Type() string
	// Run executes entry against in and returns the resulting decision.
	Run(ctx context.Context, entry corehook.Entry, in agent.HookInput) agent.HookOutput
}

// MCPCaller invokes a named tool on a named MCP server. Implementations live
// in agentapp where MCPManager is wired; the MCPDriver depends only on this
// interface so infra/hook stays free of agentapp imports.
type MCPCaller interface {
	CallMCPTool(ctx context.Context, server, tool string, input map[string]any) (string, error)
}

// LLMCaller runs a single-turn prompt against a model and returns the text
// content. An empty model means "use the default fast model". Implementations
// live in agentapp.
type LLMCaller interface {
	CompleteHookPrompt(ctx context.Context, model, prompt string) (string, error)
}

// Deps groups the externals that drivers need to be constructed. It is
// passed to NewDriverRegistry. Nil fields disable the corresponding driver
// type (the registry skips it instead of returning a broken implementation).
type Deps struct {
	MCPCaller MCPCaller
	LLMCaller LLMCaller
}

// NewDriverRegistry builds the per-type driver map used by the HookManager.
// Drivers with external dependencies are included only when their caller is
// available; dispatch then skips the corresponding configured entries.
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
