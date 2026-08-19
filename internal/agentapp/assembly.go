package agentapp

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	tools "github.com/gougoujiang/buildmax/internal/tool"
)

// MaxAdditionalSystemPromptChars bounds the additional system prompt. It sits in the system
// prompt, which is re-sent in full on every call and has no trimming path, so it is bounded
// when it is resolved rather than degraded later.
const MaxAdditionalSystemPromptChars = 8192

// ValidateAdditionalSystemPrompt rejects text that does not fit the budget. The error names the
// size and the limit so whoever supplied it — a flag, a file, or an agent record — can see what
// to cut.
func ValidateAdditionalSystemPrompt(text string) error {
	if n := utf8.RuneCountInString(text); n > MaxAdditionalSystemPromptChars {
		return fmt.Errorf("additional system prompt is %d characters, limit is %d: it is sent with "+
			"every model call and cannot be trimmed, so it has to stay short", n, MaxAdditionalSystemPromptChars)
	}
	return nil
}

// BuildEffectiveSystemPrompt builds the agent system prompt for a workspace, an optional model
// name, and an optional additional system prompt.
//
// The layers run from least to most specific, and every one of them is additive:
//
//  1. the runtime prompt, which carries the tool-usage conventions
//  2. ~/.buildmax/AGENTS.md — personal rules
//  3. <ws>/AGENTS.md — project rules
//  4. the additional system prompt — this run's user-authored identity and constraints
//
// All four are stable for the life of a session, so together they form a cacheable prefix. The
// compaction summary changes, and RunLoop appends it after them; it is never added here.
//
// Pass an empty modelName when it is not yet known, and empty additional text when the run has
// none.
func BuildEffectiveSystemPrompt(workspaceDir, modelName, additionalSystemPrompt string) string {
	prompt, _ := BuildSystemPromptWithLayers(workspaceDir, modelName, additionalSystemPrompt)
	return prompt
}

// BuildSystemPromptWithLayers builds the prompt and reports which layers contributed to it.
// The layer list goes into the run trace, so a finished run can say what it was told before
// the conversation began rather than leaving it to be inferred from behaviour.
func BuildSystemPromptWithLayers(workspaceDir, modelName, additionalSystemPrompt string) (string, []agent.PromptLayer) {
	effectivePrompt := DefaultSystemPrompt
	layers := []agent.PromptLayer{{Name: "runtime", Chars: len(DefaultSystemPrompt)}}
	appendLayer := func(name, text string) {
		effectivePrompt += "\n\n" + text
		layers = append(layers, agent.PromptLayer{Name: name, Chars: len(text)})
	}

	if modelName != "" {
		effectivePrompt += "\n\n# Runtime context\nCurrent model: " + modelName
	}
	if global, err := ReadAgentsMd(config.DataDir()); err == nil && global != "" {
		appendLayer("user_agents_md", global)
	}
	if ws, err := ReadAgentsMd(workspaceDir); err == nil && ws != "" {
		appendLayer("workspace_agents_md", ws)
	}
	if extra := strings.TrimSpace(additionalSystemPrompt); extra != "" {
		appendLayer("additional_system_prompt", "# Additional instructions\n"+extra)
	}
	return effectivePrompt, layers
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
		tools.NewNoteWrite(),
		skillTool,
	}
}
