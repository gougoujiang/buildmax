package model

import (
	"context"
	"testing"
)

func TestToolRegistry(t *testing.T) {
	mock := &mockTool{
		name:        "test_tool",
		description: "A test tool",
		params:      map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "string"}}},
	}
	registry := NewToolRegistry()
	registry.AppendTools(mock)
	defs := registry.GetDefs()
	if len(defs) != 1 {
		t.Fatalf("len(defs) = %d, want 1", len(defs))
	}
	if defs[0].Name != "test_tool" {
		t.Errorf("defs[0].Name = %q, want test_tool", defs[0].Name)
	}
	if defs[0].Description != "A test tool" {
		t.Errorf("defs[0].Description = %q, want A test tool", defs[0].Description)
	}
	if defs[0].Parameters == nil {
		t.Error("defs[0].Parameters is nil")
	}
	if got := registry.Lookup("test_tool"); got != mock {
		t.Errorf("Lookup(test_tool) = %v, want %v", got, mock)
	}

	extra := &mockTool{name: "extra_tool"}
	registry.AppendTools(extra)
	if got := registry.Lookup("extra_tool"); got != extra {
		t.Errorf("Lookup(extra_tool) = %v, want %v", got, extra)
	}
	gotTools := registry.Tools()
	if len(gotTools) != 2 {
		t.Fatalf("len(Tools()) = %d, want 2", len(gotTools))
	}
	if gotTools[0] != mock || gotTools[1] != extra {
		t.Fatalf("Tools() = %v, want [%v %v]", gotTools, mock, extra)
	}

	empty := NewToolRegistry()
	if len(empty.GetDefs()) != 0 {
		t.Errorf("NewToolRegistry().GetDefs() length = %d, want 0", len(empty.GetDefs()))
	}
	if empty.Lookup("missing") != nil {
		t.Error("NewToolRegistry().Lookup(missing) != nil, want nil")
	}
	if len(empty.Tools()) != 0 {
		t.Errorf("NewToolRegistry().Tools() length = %d, want 0", len(empty.Tools()))
	}
}

type mockTool struct {
	name        string
	description string
	params      any
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return t.description }
func (t *mockTool) Parameters() any     { return t.params }
func (t *mockTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	return "", nil
}
