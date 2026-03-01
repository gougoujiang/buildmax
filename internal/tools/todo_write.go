// Package tools provides concrete agent tools (e.g. read_file, write_file, webfetch, todowrite).
package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Valid todo statuses.
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
)

var validStatuses = map[string]bool{
	StatusPending: true, StatusInProgress: true, StatusCompleted: true,
}

// todoItem represents one todo (content, status, active_form). Used when parsing args.
type todoItem struct {
	content    string
	status     string
	activeForm string
}

// TodoWrite is a tool that formats a task list for the LLM to trace progress.
// It does not store state; it validates the given todos and returns a formatted list.
// It implements the agent.Tool interface.
type TodoWrite struct{}

// NewTodoWrite creates a TodoWrite tool.
func NewTodoWrite() (*TodoWrite, error) {
	return &TodoWrite{}, nil
}

// Name returns the tool name for the LLM.
func (t *TodoWrite) Name() string { return ToolNameTodoWrite }

// Description returns a short description so the LLM knows when to use this tool.
func (t *TodoWrite) Description() string {
	return "Format a task list so you can trace progress. Pass the current list of todos (content, status: pending/in_progress/completed, optional active_form). Returns a formatted list for reference; does not store state."
}

// Parameters returns the OpenAI-style JSON schema for the tool arguments.
func (t *TodoWrite) Parameters() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"todos": map[string]any{
				"type":        "array",
				"description": "Current list of todo items to format",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{
							"type":        "string",
							"description": "Short description of the task",
						},
						"status": map[string]any{
							"type":        "string",
							"description": "pending, in_progress, or completed",
							"enum":        []string{StatusPending, StatusInProgress, StatusCompleted},
						},
						"active_form": map[string]any{
							"type":        "string",
							"description": "Optional present-tense description (e.g. Running tests)",
						},
					},
					"required": []string{"content", "status"},
				},
			},
		},
		"required": []string{"todos"},
	}
}

// Execute parses and validates todos from args, then returns a formatted list for the LLM.
func (t *TodoWrite) Execute(ctx context.Context, args map[string]any) (string, error) {
	v, ok := args["todos"]
	if !ok {
		return "", errors.New("todos is required")
	}
	slice, ok := v.([]any)
	if !ok {
		return "", errors.New("todos must be an array")
	}

	items := make([]todoItem, 0, len(slice))
	for i, elem := range slice {
		m, ok := elem.(map[string]any)
		if !ok {
			return "", fmt.Errorf("todos[%d] must be an object", i)
		}
		content, _ := m["content"].(string)
		content = strings.TrimSpace(content)
		if content == "" {
			return "", fmt.Errorf("todos[%d]: content is required and must be non-empty", i)
		}
		status, _ := m["status"].(string)
		status = strings.TrimSpace(status)
		if !validStatuses[status] {
			return "", fmt.Errorf("todos[%d]: status must be pending, in_progress, or completed", i)
		}
		activeForm, _ := m["active_form"].(string)
		items = append(items, todoItem{content: content, status: status, activeForm: strings.TrimSpace(activeForm)})
	}

	return formatTodoList(items), nil
}

// formatTodoList returns a human-readable list of todos for the LLM.
func formatTodoList(items []todoItem) string {
	if len(items) == 0 {
		return "Todo list (0 items)."
	}
	n := len(items)
	itemWord := "items"
	if n == 1 {
		itemWord = "item"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Todo list (%d %s):\n", n, itemWord))
	for i, it := range items {
		line := fmt.Sprintf("%d. [%s] %s", i+1, it.status, it.content)
		if it.activeForm != "" {
			line += " — " + it.activeForm
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
