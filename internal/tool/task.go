package tool

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/gougoujiang/buildmax/internal/agentapp/job"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/session"
)

// maxSubAgentReplyRunes is the character cap for a sub-agent reply returned to the parent.
// Oversized replies are head+tail truncated so the parent context is not flooded.
const maxSubAgentReplyRunes = 20_000

// Built-in sub-agent system prompts.
const (
	GeneralSubAgentPrompt = `You are a general-purpose AI assistant sub-agent. You have access to tools for reading, writing, editing files, running commands, and more. Complete the task described in the user message thoroughly and return a clear, concise result.`

	ExploreSubAgentPrompt = `You are a code exploration sub-agent. You have read-only access to the codebase via Read, Glob, and Grep tools. Explore the codebase to answer the question or find the information requested. Return a clear, organized summary of your findings.`

	ShellSubAgentPrompt = `You are a command execution sub-agent. You have access to the Bash tool for running shell commands. Execute the requested commands and return the results. Be careful with destructive operations.`
)

// BuiltinSubAgentDef describes a built-in sub-agent type declaratively.
// ToolNames lists tool names resolved at runtime; nil means all base tools.
type BuiltinSubAgentDef struct {
	Name         string
	ToolNames    []string // nil ⇒ all base tools
	SystemPrompt string
	Description  string
}

// BuiltinSubAgentDefs defines the built-in sub-agent types in display order.
var BuiltinSubAgentDefs = []BuiltinSubAgentDef{
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

// builtinTypeOrder returns the names in declaration order (derived from BuiltinSubAgentDefs).
var builtinTypeOrder = func() []string {
	names := make([]string, len(BuiltinSubAgentDefs))
	for i, d := range BuiltinSubAgentDefs {
		names[i] = d.Name
	}
	return names
}()

// builtinNames is a set for quick lookup (derived from BuiltinSubAgentDefs).
var builtinNames = func() map[string]bool {
	m := make(map[string]bool, len(BuiltinSubAgentDefs))
	for _, d := range BuiltinSubAgentDefs {
		m[d.Name] = true
	}
	return m
}()

// AgentTypeConfig holds the configuration for one agent type (built-in or user-defined).
type AgentTypeConfig struct {
	Tools         []llm.Tool // tools available to this agent type
	SystemPrompt  string     // system prompt for the sub-agent
	Description   string     // LLM-readable description of this agent type
	Model         string     // model name to use; "" = runner default
	MaxIterations int        // iteration cap; 0 = defaultSubAgentMaxIter (50)
}

// TaskTool is an agent tool that spawns sub-agents to handle complex subtasks.
// It implements llm.Tool.
type TaskTool struct {
	runner     SubAgentRunner
	agentTypes map[string]AgentTypeConfig
	typeOrder  []string // deterministic ordering: built-in first, then user-defined alphabetically
	// jobs enables run_in_background, following the same rule as Bash: nil
	// keeps the parameter out of the schema entirely.
	jobs *job.Manager
	// workspace names the workspace a background subagent shares, recorded in
	// the job's provenance.
	workspace string
	// readOnlyTypes marks the agent types whose whole tool set is read-only,
	// computed once at construction because the sets never change afterwards.
	readOnlyTypes map[string]bool
}

// WithJobs returns a copy of t that can detach subagents to the given job
// manager. Nil leaves background execution unavailable.
func (t *TaskTool) WithJobs(m *job.Manager, workspace string) *TaskTool {
	out := *t
	out.jobs = m
	out.workspace = workspace
	return &out
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

	readOnlyTypes := make(map[string]bool, len(agentTypes))
	for name, cfg := range agentTypes {
		readOnlyTypes[name] = subAgentToolsAreReadOnly(cfg.Tools)
	}

	return &TaskTool{
		runner:        runner,
		agentTypes:    agentTypes,
		typeOrder:     typeOrder,
		readOnlyTypes: readOnlyTypes,
	}, nil
}

// subAgentToolsAreReadOnly reports whether every tool the type can reach
// declares itself read-only. Task is skipped because Execute strips it from
// the sub-agent's registry, and an empty set answers false: a type that
// resolved to nothing must not become eligible by having no counterexample.
func subAgentToolsAreReadOnly(list []llm.Tool) bool {
	found := 0
	for _, tl := range list {
		if tl.Name() == ToolNameTask {
			continue
		}
		if agent.DeclaredAccess(tl, nil) != llm.AccessReadOnly {
			return false
		}
		found++
	}
	return found > 0
}

// Access implements llm.AccessDeclarer. A sub-agent writes whatever its tools
// write, so the answer is the requested type's tool set rather than the Task
// tool itself: a type restricted to read-only tools runs a nested loop that
// cannot write, and may overlap its neighbours.
//
// Everything else is a write — an unknown type, a background run (which
// mutates the job manager and outlives the call), and a nil-args capability
// listing, which is asking about the tool and not about a call.
func (t *TaskTool) Access(args map[string]any) llm.Access {
	if background, _ := args["run_in_background"].(bool); background {
		return llm.AccessWrite
	}
	name, _ := args["subagent_type"].(string)
	if t.readOnlyTypes[strings.TrimSpace(name)] {
		return llm.AccessReadOnly
	}
	return llm.AccessWrite
}

// Name returns the tool name for the LLM.

func (t *TaskTool) Name() string { return ToolNameTask }

// Description returns a dynamically built description listing all available agent types.
func (t *TaskTool) Description() string {
	var b strings.Builder
	b.WriteString("Launch a sub-agent to handle a complex subtask autonomously. The sub-agent processes the prompt using its own session and tools, then returns the final reply. Available agent types:\n")
	for _, name := range t.typeOrder {
		cfg := t.agentTypes[name]
		fmt.Fprintf(&b, "- %s: %s\n", name, cfg.Description)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// Parameters returns the OpenAI-style JSON schema with a dynamic enum for subagent_type.
func (t *TaskTool) Parameters() any {
	typeNames := make([]string, len(t.typeOrder))
	copy(typeNames, t.typeOrder)

	properties := map[string]any{
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
	}
	if t.jobs != nil {
		properties["run_in_background"] = map[string]any{
			"type":        "boolean",
			"description": "Run the sub-agent as a background job and return its job ID immediately instead of waiting. Read its final reply with JobOutput; stop it with JobStop. The sub-agent shares this workspace, so avoid delegating edits that would race yours.",
		}
		properties["deliver_result"] = map[string]any{
			"type":        "boolean",
			"description": "With run_in_background: when true, the sub-agent's final reply is delivered into this conversation when it completes. Otherwise completion is only shown in the UI and the reply is read with JobOutput.",
		}
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   []string{"description", "prompt", "subagent_type"},
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
	var subTools []llm.Tool
	for _, tl := range config.Tools {
		if tl.Name() != ToolNameTask {
			subTools = append(subTools, tl)
		}
	}

	slog.Info("task: spawning sub-agent", "type", subagentType, "description", description,
		"max_iter", config.MaxIterations, "model", config.Model)

	opts := SubAgentRunOpts{
		Tools:        subTools,
		SystemPrompt: config.SystemPrompt,
		Description:  description,
		MaxIter:      config.MaxIterations,
		Model:        config.Model,
	}
	if background, _ := args["run_in_background"].(bool); background {
		deliver, _ := args["deliver_result"].(bool)
		return t.executeBackground(ctx, description, opts, prompt, deliver)
	}
	reply, err := t.runner.RunSubAgent(ctx, opts, prompt)
	if err != nil {
		return "", fmt.Errorf("sub-agent failed: %w", err)
	}

	// Truncate oversized replies so the parent context is not flooded.
	reply = truncateSubAgentReply(reply, maxSubAgentReplyRunes)

	slog.Info("task: sub-agent completed", "type", subagentType, "reply_len", len(reply))
	return reply, nil
}

// executeBackground detaches the subagent to the job manager and returns its
// job identity. The manager owns the run's context; the values a detached run
// still needs — owner session, launching run and tool call — are copied onto
// it explicitly, so nothing else of the request context leaks into work that
// outlives the call.
func (t *TaskTool) executeBackground(ctx context.Context, description string, opts SubAgentRunOpts, prompt string, deliver bool) (string, error) {
	if agent.SubagentFromCtx(ctx) {
		return "Background execution is not available inside a subagent. Return your findings and let the parent session delegate further work.", nil
	}
	if t.jobs == nil {
		return "Background execution is not available on this surface. Run the subagent in the foreground.", nil
	}
	sessionID, _ := session.SessionIDFromContext(ctx)
	runID := agent.RunIDFromCtx(ctx)
	toolCallID := agent.ToolCallFromCtx(ctx)
	runner := t.runner

	j, err := t.jobs.StartSubagent(job.SubagentSpec{
		Description: description,
		Deliver:     deliver,
	}, job.Provenance{
		Workspace:        t.workspace,
		SessionID:        sessionID,
		ParentTraceID:    runID,
		ParentToolCallID: toolCallID,
	}, func(jobCtx context.Context) (string, error) {
		jobCtx = session.CtxWithSessionID(jobCtx, sessionID)
		jobCtx = agent.CtxWithRunID(jobCtx, runID)
		jobCtx = agent.CtxWithToolCall(jobCtx, toolCallID)
		reply, err := runner.RunSubAgent(jobCtx, opts, prompt)
		if err != nil {
			return "", err
		}
		return truncateSubAgentReply(reply, maxSubAgentReplyRunes), nil
	})
	if err != nil {
		return "Cannot start background subagent: " + err.Error(), nil
	}
	return "Started background subagent job " + j.ID + " — " + description + ".\n" +
		"Its final reply appears in JobOutput {\"job_id\": \"" + j.ID + "\"} when it completes; stop it with JobStop. " +
		"It runs in this workspace, so its file changes are shared with the conversation.", nil
}

// truncateSubAgentReply applies head+tail truncation when reply exceeds maxRunes.
func truncateSubAgentReply(reply string, maxRunes int) string {
	runes := []rune(reply)
	total := len(runes)
	if total <= maxRunes {
		return reply
	}
	head := maxRunes * 2 / 5
	tail := maxRunes * 2 / 5
	omitted := total - head - tail
	return string(runes[:head]) +
		fmt.Sprintf("\n\n(sub-agent reply truncated — %d characters total, %d omitted from middle)\n\n", total, omitted) +
		string(runes[total-tail:])
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
