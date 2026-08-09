package agentapp

import (
	"log/slog"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	tools "github.com/gougoujiang/buildmax/internal/tool"
)

// BuildEffectiveSystemPrompt builds the agent system prompt for a workspace and optional model name.
// Reads ~/.buildmax/AGENTS.md (personal rules) and <ws>/AGENTS.md (project rules), appending both
// when present: global first, workspace second. Pass an empty modelName when not yet known.
func BuildEffectiveSystemPrompt(workspaceDir, modelName string) string {
	effectivePrompt := DefaultSystemPrompt
	if modelName != "" {
		effectivePrompt += "\n\n# Runtime context\nCurrent model: " + modelName
	}
	if global, err := ReadAgentsMd(config.DataDir()); err == nil && global != "" {
		effectivePrompt += "\n\n" + global
	}
	if ws, err := ReadAgentsMd(workspaceDir); err == nil && ws != "" {
		effectivePrompt += "\n\n" + ws
	}
	return effectivePrompt
}

// BuildAgentTypes merges built-in sub-agent definitions with caller-provided user defs into
// an AgentTypeConfig map ready for tools.NewTask.
func BuildAgentTypes(registry llm.ToolRegistry, userDefs []tools.SubAgentDef) map[string]tools.AgentTypeConfig {
	agentTypes := make(map[string]tools.AgentTypeConfig, len(tools.BuiltinSubAgentDefs))
	for _, def := range tools.BuiltinSubAgentDefs {
		var resolved []llm.Tool
		if def.ToolNames == nil {
			resolved = registry.Tools()
		} else {
			resolved = ResolveAgentTypeTools(def.Name, def.ToolNames, registry)
		}
		agentTypes[def.Name] = tools.AgentTypeConfig{
			Tools:        resolved,
			SystemPrompt: def.SystemPrompt,
			Description:  def.Description,
			// Built-in types use runner defaults for Model and MaxIterations.
		}
	}
	for _, def := range userDefs {
		if _, exists := agentTypes[def.Name]; exists {
			slog.Warn("skip user-defined agent: name conflicts with built-in", "name", def.Name)
			continue
		}
		resolved := ResolveAgentTypeTools(def.Name, def.ToolNames, registry)
		if len(resolved) == 0 {
			slog.Warn("skip user-defined agent: no valid tools resolved", "name", def.Name)
			continue
		}
		agentTypes[def.Name] = tools.AgentTypeConfig{
			Tools:         resolved,
			SystemPrompt:  def.SystemPrompt,
			Description:   def.Description,
			Model:         def.Model,
			MaxIterations: def.MaxIterations,
		}
	}
	return agentTypes
}

// ResolveAgentTypeTools resolves tool names from a registry; skips unknowns with a warning.
func ResolveAgentTypeTools(agentName string, toolNames []string, registry llm.ToolRegistry) []llm.Tool {
	if toolNames == nil {
		return nil
	}
	resolved := make([]llm.Tool, 0, len(toolNames))
	for _, name := range toolNames {
		t := registry.Lookup(name)
		if t == nil {
			slog.Warn("skip unknown tool in agent def", "agent", agentName, "tool", name)
			continue
		}
		resolved = append(resolved, t)
	}
	return resolved
}

// buildBaseTools returns the standard set of workspace tools for an agent.
// sandboxView is the SandboxView the Bash tool wraps spawned commands
// through; pass agent.NoopSandbox{} (or nil) to leave bash unsandboxed.
func buildBaseTools(client llm.LLMClient, workspaceRoot string, skillTool llm.Tool, sandboxView agent.SandboxView) []llm.Tool {
	if sandboxView == nil {
		sandboxView = agent.NoopSandbox{}
	}
	return []llm.Tool{
		tools.NewReadFile(workspaceRoot),
		tools.NewWriteFile(workspaceRoot),
		tools.NewBash(workspaceRoot).WithSandbox(sandboxView),
		tools.NewGlob(workspaceRoot),
		tools.NewEditFile(workspaceRoot),
		tools.NewGrep(workspaceRoot),
		tools.NewWebFetch(client, 15*time.Minute).WithSandbox(sandboxView),
		tools.NewTodoWrite(),
		skillTool,
	}
}
