// Package tool contains the Tier 1 conversation tools exposed to the model.
package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// AgentSummary holds the minimal info needed for the StartTask tool description.
type AgentSummary struct {
	ID          string
	Name        string
	Description string
}

// StartTaskRunner creates a new background task.
type StartTaskRunner interface {
	StartTask(ctx context.Context, input string, agentID *string) (taskID, runID string, err error)
}

type startTaskTool struct {
	runner StartTaskRunner
	agents []AgentSummary
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
	taskID, _, err := t.runner.StartTask(ctx, inputVal, agentID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Background task created and scheduled (task_id: %s). The task is now running in the background.", taskID), nil
}

// NewStartTaskTool returns a llm.Tool that uses runner to create a task.
// agents provides the list of available agents to include in the tool description.
// If runner is nil, Execute returns "not configured".
func NewStartTaskTool(runner StartTaskRunner, agents []AgentSummary) llm.Tool {
	return &startTaskTool{runner: runner, agents: agents}
}
