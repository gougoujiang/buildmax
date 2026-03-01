package tools

import (
	"context"
	"strings"
	"testing"

	"buildmax/internal/core"
)

func TestNewTodoWrite(t *testing.T) {
	w, err := NewTodoWrite()
	if err != nil {
		t.Fatalf("NewTodoWrite: %v", err)
	}
	if w == nil {
		t.Fatal("NewTodoWrite returned nil")
	}
	if w.Name() != ToolNameTodoWrite {
		t.Errorf("Name() = %q, want TodoWrite", w.Name())
	}
	var _ core.Tool = (*TodoWrite)(nil)
}

func TestTodoWrite_Execute_ValidTodos_ReturnsFormattedList(t *testing.T) {
	w, err := NewTodoWrite()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	todos := []any{
		map[string]any{"content": "Task one", "status": "pending", "active_form": "Pending"},
		map[string]any{"content": "Task two", "status": "in_progress"},
	}
	result, err := w.Execute(ctx, map[string]any{"todos": todos})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "Todo list (2 items)") {
		t.Errorf("result should contain count: %q", result)
	}
	if !strings.Contains(result, "[pending] Task one") {
		t.Errorf("result should contain first item: %q", result)
	}
	if !strings.Contains(result, "[in_progress] Task two") {
		t.Errorf("result should contain second item: %q", result)
	}
	if !strings.Contains(result, "Pending") {
		t.Errorf("result should contain active_form: %q", result)
	}
}

func TestTodoWrite_Execute_EmptyList(t *testing.T) {
	w, err := NewTodoWrite()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	result, err := w.Execute(ctx, map[string]any{"todos": []any{}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "0 items") {
		t.Errorf("result = %q, want to contain 0 items", result)
	}
}

func TestTodoWrite_Execute_MissingTodos(t *testing.T) {
	w, err := NewTodoWrite()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = w.Execute(ctx, map[string]any{})
	if err == nil {
		t.Fatal("Execute without todos should return error")
	}
	if !strings.Contains(err.Error(), "todos") {
		t.Errorf("error should mention todos: %v", err)
	}
}

func TestTodoWrite_Execute_TodosNotArray(t *testing.T) {
	w, err := NewTodoWrite()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, err = w.Execute(ctx, map[string]any{"todos": "not an array"})
	if err == nil {
		t.Fatal("Execute with todos not array should return error")
	}
	if !strings.Contains(err.Error(), "array") {
		t.Errorf("error should mention array: %v", err)
	}
}

func TestTodoWrite_Execute_InvalidStatus(t *testing.T) {
	w, err := NewTodoWrite()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	todos := []any{
		map[string]any{"content": "Task", "status": "invalid_status"},
	}
	_, err = w.Execute(ctx, map[string]any{"todos": todos})
	if err == nil {
		t.Fatal("Execute with invalid status should return error")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Errorf("error should mention status: %v", err)
	}
}

func TestTodoWrite_Execute_EmptyContent(t *testing.T) {
	w, err := NewTodoWrite()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	todos := []any{
		map[string]any{"content": "", "status": "pending"},
	}
	_, err = w.Execute(ctx, map[string]any{"todos": todos})
	if err == nil {
		t.Fatal("Execute with empty content should return error")
	}
	if !strings.Contains(err.Error(), "content") {
		t.Errorf("error should mention content: %v", err)
	}
}

func TestTodoWrite_NameDescriptionParameters(t *testing.T) {
	w, err := NewTodoWrite()
	if err != nil {
		t.Fatal(err)
	}
	if w.Name() != ToolNameTodoWrite {
		t.Errorf("Name() = %q", w.Name())
	}
	desc := w.Description()
	if desc == "" || !strings.Contains(desc, "task list") {
		t.Errorf("Description() = %q", desc)
	}
	params := w.Parameters()
	m, ok := params.(map[string]any)
	if !ok {
		t.Fatalf("Parameters() should return map")
	}
	req, ok := m["required"].([]string)
	if !ok || len(req) == 0 {
		t.Errorf("required should include todos")
	}
}
