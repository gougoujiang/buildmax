// Package tools provides concrete agent tools (e.g. read_file, write_file, webfetch, todowrite).
package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// Valid todo statuses. The values live in internal/core/agent, which owns durable session
// state; these names stay for the tool's own schema and messages.
const (
	StatusPending    = agent.TodoPending
	StatusInProgress = agent.TodoInProgress
	StatusCompleted  = agent.TodoCompleted
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

// TodoWrite records the task list the LLM uses to trace progress.
//
// The list is durable session state, not a message: it survives history compaction and is
// re-rendered on every model call. It is stored through the context, because the tool registry
// is cached per model and shared across sessions. A run with no store — a subagent, for
// instance — still gets a formatted list back, and is told the list was not kept.
type TodoWrite struct{}

// NewTodoWrite creates a TodoWrite tool.
func NewTodoWrite() *TodoWrite {
	return &TodoWrite{}
}

// Name returns the tool name for the LLM.
// Access implements llm.AccessDeclarer. Session task list has no lock, so this is a
// write and cannot run concurrently with a sibling call.
func (t *TodoWrite) Access(_ map[string]any) llm.Access { return llm.AccessWrite }

// DefaultAction implements llm.PolicyProvider, overriding the action the
// permission layer would otherwise derive from Access.
//
// What this writes is the agent's own scratch state, not anything the user
// owns. Asking a user to approve the agent taking a note would be noise, and
// the noise would arrive on every run. The write classification above is still
// the honest one — it is what keeps this tool out of a parallel batch.
// See docs/design/tool-permissions.md §5.2.
func (t *TodoWrite) DefaultAction() llm.ToolAction { return llm.ToolActionAllow }

func (t *TodoWrite) Name() string { return ToolNameTodoWrite }

// Description returns a short description so the LLM knows when to use this tool.
func (t *TodoWrite) Description() string {
	return "Record the task list so you can trace progress. Pass the complete list of todos " +
		"(content, status: pending/in_progress/completed, optional active_form); it replaces the " +
		"stored one. Exactly one task is in_progress at a time. The list survives compaction of " +
		"the conversation history and is shown to you on every turn."
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

// Execute parses and validates todos from args, stores them on the session when the run keeps
// durable state, and returns a formatted list for the LLM.
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

	todos := make([]agent.Todo, len(items))
	for i, it := range items {
		todos[i] = agent.Todo{Content: it.content, Status: it.status, ActiveForm: it.activeForm}
	}
	if err := agent.ValidateTodos(todos); err != nil {
		return "", err
	}

	store, ok := agent.NoteStoreFromContext(ctx)
	if !ok {
		return formatTodoList(items) + "\n\n(This run keeps no durable task list, so the list was not stored.)", nil
	}
	store.SetTodos(todos, agent.IterationFromContext(ctx))
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
	fmt.Fprintf(&b, "Todo list (%d %s):\n", n, itemWord)
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
