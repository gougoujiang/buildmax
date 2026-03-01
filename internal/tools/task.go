package tools

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"buildmax/internal/core"
)

// SubAgentRunner runs a sub-agent with the given tools and prompt.
// It is implemented by agent and injected when building the Task tool so that
// tools do not depend on a concrete runner; tests can inject a mock.
type SubAgentRunner interface {
	RunSubAgent(ctx context.Context, tools []core.Tool, systemPrompt string, description string, prompt string) (reply string, err error)
}

// Built-in sub-agent system prompts.
const (
	GeneralSubAgentPrompt = `You are a general-purpose AI assistant sub-agent. You have access to tools for reading, writing, editing files, running commands, and more. Complete the task described in the user message thoroughly and return a clear, concise result.`

	ExploreSubAgentPrompt = `You are a code exploration sub-agent. You have read-only access to the codebase via Read, Glob, and Grep tools. Explore the codebase to answer the question or find the information requested. Return a clear, organized summary of your findings.`

	ShellSubAgentPrompt = `You are a command execution sub-agent. You have access to the Bash tool for running shell commands. Execute the requested commands and return the results. Be careful with destructive operations.`
)

// BuiltinAgentDef describes a built-in sub-agent type declaratively.
// ToolNames lists tool names resolved at runtime; nil means all base tools.
type BuiltinAgentDef struct {
	Name         string
	ToolNames    []string // nil ⇒ all base tools
	SystemPrompt string
	Description  string
}

// BuiltinAgentDefs defines the built-in sub-agent types in display order.
var BuiltinAgentDefs = []BuiltinAgentDef{
	{
		Name:         "general",
		ToolNames:    nil, // all base tools
		SystemPrompt: GeneralSubAgentPrompt,
		Description:  "General-purpose agent with all tools for multi-step tasks.",
	},
	{
		Name:         "explore",
		ToolNames:    []string{ToolNameRead, ToolNameGlob, ToolNameGrep},
		SystemPrompt: ExploreSubAgentPrompt,
		Description:  "Read-only agent for fast codebase exploration (Read, Glob, Grep).",
	},
	{
		Name:         "shell",
		ToolNames:    []string{ToolNameBash},
		SystemPrompt: ShellSubAgentPrompt,
		Description:  "Command execution specialist (Bash only).",
	},
}

// builtinTypeOrder returns the names in declaration order (derived from BuiltinAgentDefs).
var builtinTypeOrder = func() []string {
	names := make([]string, len(BuiltinAgentDefs))
	for i, d := range BuiltinAgentDefs {
		names[i] = d.Name
	}
	return names
}()

// builtinNames is a set for quick lookup (derived from BuiltinAgentDefs).
var builtinNames = func() map[string]bool {
	m := make(map[string]bool, len(BuiltinAgentDefs))
	for _, d := range BuiltinAgentDefs {
		m[d.Name] = true
	}
	return m
}()

// AgentTypeConfig holds the configuration for one agent type (built-in or user-defined).
type AgentTypeConfig struct {
	Tools        []core.Tool // tools available to this agent type
	SystemPrompt string       // system prompt for the sub-agent
	Description  string       // LLM-readable description of this agent type
}

// TaskTool is an agent tool that spawns sub-agents to handle complex subtasks.
// It implements core.Tool.
type TaskTool struct {
	runner     SubAgentRunner
	agentTypes map[string]AgentTypeConfig
	typeOrder  []string // deterministic ordering: built-in first, then user-defined alphabetically
}

// NewTask creates a TaskTool with the given sub-agent runner and agent type configurations.
// runner must not be nil; agentTypes must have at least one entry.
func NewTask(runner SubAgentRunner, agentTypes map[string]AgentTypeConfig) (*TaskTool, error) {
	if runner == nil {
		return nil, errors.New("runner must not be nil")
	}
	if len(agentTypes) == 0 {
		return nil, errors.New("agentTypes must have at least one entry")
	}

	// Build deterministic type order: built-in first, then user-defined alphabetically.
	var builtins []string
	var userDefined []string
	for name := range agentTypes {
		if builtinNames[name] {
			builtins = append(builtins, name)
		} else {
			userDefined = append(userDefined, name)
		}
	}
	// Sort builtins in the fixed order.
	sort.Slice(builtins, func(i, j int) bool {
		return indexOf(builtinTypeOrder, builtins[i]) < indexOf(builtinTypeOrder, builtins[j])
	})
	sort.Strings(userDefined)
	typeOrder := append(builtins, userDefined...)

	return &TaskTool{
		runner:     runner,
		agentTypes: agentTypes,
		typeOrder:  typeOrder,
	}, nil
}

// Name returns the tool name for the LLM.
func (t *TaskTool) Name() string { return ToolNameTask }

// Description returns a dynamically built description listing all available agent types.
func (t *TaskTool) Description() string {
	var b strings.Builder
	b.WriteString("Launch a sub-agent to handle a complex subtask autonomously. The sub-agent processes the prompt using its own session and tools, then returns the final reply. Available agent types:\n")
	for _, name := range t.typeOrder {
		cfg := t.agentTypes[name]
		b.WriteString(fmt.Sprintf("- %s: %s\n", name, cfg.Description))
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// Parameters returns the OpenAI-style JSON schema with a dynamic enum for subagent_type.
func (t *TaskTool) Parameters() any {
	typeNames := make([]string, len(t.typeOrder))
	copy(typeNames, t.typeOrder)

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"description": map[string]any{
				"type":        "string",
				"description": "Short 3-5 word summary of the task",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "Detailed task description for the sub-agent. Be specific about what you want the sub-agent to do and what information to return.",
			},
			"subagent_type": map[string]any{
				"type":        "string",
				"description": "The type of sub-agent to use",
				"enum":        typeNames,
			},
		},
		"required": []string{"description", "prompt", "subagent_type"},
	}
}

// Execute spawns a sub-agent of the requested type, runs the prompt, and returns the reply.
func (t *TaskTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	// Parse and validate description.
	description, _ := args["description"].(string)
	description = strings.TrimSpace(description)
	if description == "" {
		return "", errors.New("description is required")
	}

	// Parse and validate prompt.
	prompt, _ := args["prompt"].(string)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("prompt is required")
	}

	// Parse and validate subagent_type.
	subagentType, _ := args["subagent_type"].(string)
	subagentType = strings.TrimSpace(subagentType)
	config, ok := t.agentTypes[subagentType]
	if !ok {
		available := strings.Join(t.typeOrder, ", ")
		return "", fmt.Errorf("unknown subagent_type %q; available types: %s", subagentType, available)
	}

	// Filter out the Task tool itself to prevent infinite recursion.
	var subTools []core.Tool
	for _, tl := range config.Tools {
		if tl.Name() != ToolNameTask {
			subTools = append(subTools, tl)
		}
	}

	slog.Info("task: spawning sub-agent", "type", subagentType, "description", description)

	reply, err := t.runner.RunSubAgent(ctx, subTools, config.SystemPrompt, description, prompt)
	if err != nil {
		return "", fmt.Errorf("sub-agent failed: %w", err)
	}

	slog.Info("task: sub-agent completed", "type", subagentType, "reply_len", len(reply))
	return reply, nil
}

// indexOf returns the index of s in slice, or len(slice) if not found.
func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}
	return len(slice)
}
