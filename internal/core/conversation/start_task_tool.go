// Conversation tool: start_task. Creates and schedules a Tier 2 background task.
package conversation

import (
	"context"
	"fmt"
	"strings"

	"buildmax/internal/core/model"
)

// AgentSummary holds the minimal info needed for the StartTask tool description.
type AgentSummary struct {
	ID          string
	Name        string
	Description string
}

// StartTaskRunner is the interface used by the start_task tool to create a background task.
// Callers (e.g. server) implement this from their config; the conversation layer passes the runner
// into RunLoop/RunLoopStream and the agent builds the StartTask tool from it.
type StartTaskRunner interface {
	StartTask(ctx context.Context, scopeID, userID, input string, agentID *string) (taskID, runID string, err error)
}

// StartTaskFunc creates a background task (Tier 2) and schedules it to run.
// Used as a convenience; wrap with FuncStartTaskRunner to implement StartTaskRunner.
type StartTaskFunc func(ctx context.Context, input string, agentID *string) (taskID, runID string, err error)

// FuncStartTaskRunner adapts a StartTaskFunc to StartTaskRunner (ignores scopeID/userID in the call).
func FuncStartTaskRunner(fn StartTaskFunc) StartTaskRunner {
	if fn == nil {
		return nil
	}
	return &funcStartTaskRunner{fn: fn}
}

type funcStartTaskRunner struct{ fn StartTaskFunc }

func (f *funcStartTaskRunner) StartTask(ctx context.Context, _, _, input string, agentID *string) (string, string, error) {
	return f.fn(ctx, input, agentID)
}

// startTaskTool implements Tool for StartTask; it delegates to StartTaskRunner.
type startTaskTool struct {
	scopeID string
	userID  string
	runner  StartTaskRunner
	agents  []AgentSummary
}

func (t *startTaskTool) Name() string { return ToolNameStartTask }

const startTaskBaseDescription = "Start a background task (Tier 2). The task is created and scheduled to run; it may take a while. Use this when the user asks for a long-running job, analysis, or work that should run in the background. Tell the user you have started a task and will report back when it completes. Do not provide internal task or run IDs to the user."

func (t *startTaskTool) Description() string {
	if len(t.agents) == 0 {
		return startTaskBaseDescription
	}
	var b strings.Builder
	b.WriteString(startTaskBaseDescription)
	b.WriteString("\n\nAvailable agents:\n")
	for _, a := range t.agents {
		b.WriteString("- ")
		b.WriteString(a.Name)
		b.WriteString(" (id: ")
		b.WriteString(a.ID)
		b.WriteString(")")
		if a.Description != "" {
			b.WriteString(" - ")
			b.WriteString(a.Description)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (t *startTaskTool) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "string",
				"description": "The task input or instruction for the background task (required).",
			},
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Optional agent id to run the task with.",
			},
		},
		"required": []any{"input"},
	}
}

func (t *startTaskTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.runner == nil {
		return "", fmt.Errorf("%s not configured", ToolNameStartTask)
	}
	inputVal, _ := args["input"].(string)
	if inputVal == "" {
		return "", fmt.Errorf("input is required")
	}
	var agentID *string
	if aid, ok := args["agent_id"].(string); ok && aid != "" {
		agentID = &aid
	}
	taskID, runID, err := t.runner.StartTask(ctx, t.scopeID, t.userID, inputVal, agentID)
	if err != nil {
		return "", err
	}
	_ = runID
	return fmt.Sprintf("Background task created and scheduled (task_id: %s). The task is now running in the background.", taskID), nil
}

// NewStartTaskTool returns a model.Tool that uses runner to create a task.
// agents provides the list of available agents to include in the tool description.
// If runner is nil, Execute returns "not configured".
func NewStartTaskTool(scopeID, userID string, runner StartTaskRunner, agents []AgentSummary) model.Tool {
	return &startTaskTool{scopeID: scopeID, userID: userID, runner: runner, agents: agents}
}
