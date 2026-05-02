package agenttool

import (
	"context"
	"strings"
	"testing"

	"buildmax/internal/core/model"
)

type mockRunner struct {
	reply string
}

func (m *mockRunner) RunSubAgent(_ context.Context, _ []model.Tool, _ string, _ string, _ string) (string, error) {
	return m.reply, nil
}

// mockAgentTool is a minimal model.Tool for constructing AgentTypeConfig in tests.
type mockAgentTool struct {
	name string
}

func (t *mockAgentTool) Name() string        { return t.name }
func (t *mockAgentTool) Description() string { return "mock tool" }
func (t *mockAgentTool) Parameters() any     { return map[string]any{"type": "object"} }
func (t *mockAgentTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	return "ok", nil
}

// Compile-time check: TaskTool implements model.Tool.
var _ model.Tool = (*TaskTool)(nil)

func testAgentTypes() map[string]AgentTypeConfig {
	return map[string]AgentTypeConfig{
		"general": {
			Tools:        []model.Tool{&mockAgentTool{name: ToolNameRead}},
			SystemPrompt: "General prompt.",
			Description:  "General-purpose agent.",
		},
		"explore": {
			Tools:        []model.Tool{&mockAgentTool{name: ToolNameRead}},
			SystemPrompt: "Explore prompt.",
			Description:  "Read-only agent.",
		},
		"custom-agent": {
			Tools:        []model.Tool{&mockAgentTool{name: ToolNameGrep}},
			SystemPrompt: "Custom prompt.",
			Description:  "A custom user-defined agent.",
		},
	}
}

func TestNewTask_NilCaller(t *testing.T) {
	_, err := NewTask(nil, testAgentTypes())
	if err == nil {
		t.Fatal("expected error for nil runner")
	}
}

func TestNewTask_EmptyTypes(t *testing.T) {
	runner := &mockRunner{reply: "ok"}
	_, err := NewTask(runner, map[string]AgentTypeConfig{})
	if err == nil {
		t.Fatal("expected error for empty agentTypes")
	}
}

func TestTaskTool_Name(t *testing.T) {
	runner := &mockRunner{reply: "ok"}
	tool, err := NewTask(runner, testAgentTypes())
	if err != nil {
		t.Fatal(err)
	}
	if tool.Name() != ToolNameTask {
		t.Errorf("Name() = %q, want %q", tool.Name(), ToolNameTask)
	}
}

func TestTaskTool_Description(t *testing.T) {
	runner := &mockRunner{reply: "ok"}
	tool, err := NewTask(runner, testAgentTypes())
	if err != nil {
		t.Fatal(err)
	}
	desc := tool.Description()
	// Should contain all type names and descriptions.
	for _, name := range []string{"general", "explore", "custom-agent"} {
		if !strings.Contains(desc, name) {
			t.Errorf("Description() missing type name %q", name)
		}
	}
	if !strings.Contains(desc, "General-purpose agent.") {
		t.Error("Description() missing general description")
	}
	if !strings.Contains(desc, "A custom user-defined agent.") {
		t.Error("Description() missing custom-agent description")
	}
	// Built-in types should appear before user-defined.
	generalIdx := strings.Index(desc, "general")
	exploreIdx := strings.Index(desc, "explore")
	customIdx := strings.Index(desc, "custom-agent")
	if generalIdx > exploreIdx {
		t.Error("general should appear before explore in description")
	}
	if exploreIdx > customIdx {
		t.Error("explore should appear before custom-agent in description")
	}
}

func TestTaskTool_Parameters(t *testing.T) {
	runner := &mockRunner{reply: "ok"}
	tool, err := NewTask(runner, testAgentTypes())
	if err != nil {
		t.Fatal(err)
	}
	params, ok := tool.Parameters().(map[string]any)
	if !ok {
		t.Fatal("Parameters() is not map[string]any")
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties is not map[string]any")
	}
	st, ok := props["subagent_type"].(map[string]any)
	if !ok {
		t.Fatal("subagent_type is not map[string]any")
	}
	enum, ok := st["enum"].([]string)
	if !ok {
		t.Fatal("enum is not []string")
	}
	// Should contain all type names.
	enumSet := make(map[string]bool)
	for _, e := range enum {
		enumSet[e] = true
	}
	for _, name := range []string{"general", "explore", "custom-agent"} {
		if !enumSet[name] {
			t.Errorf("enum missing %q", name)
		}
	}
}

func TestTaskTool_Execute_UnknownType(t *testing.T) {
	runner := &mockRunner{reply: "ok"}
	tool, err := NewTask(runner, testAgentTypes())
	if err != nil {
		t.Fatal(err)
	}
	_, err = tool.Execute(context.Background(), map[string]any{
		"description":   "test",
		"prompt":        "do something",
		"subagent_type": "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for unknown subagent_type")
	}
	if !strings.Contains(err.Error(), "unknown subagent_type") {
		t.Errorf("error = %q, want to contain 'unknown subagent_type'", err.Error())
	}
}

func TestTaskTool_Execute_MissingPrompt(t *testing.T) {
	runner := &mockRunner{reply: "ok"}
	tool, err := NewTask(runner, testAgentTypes())
	if err != nil {
		t.Fatal(err)
	}
	_, err = tool.Execute(context.Background(), map[string]any{
		"description":   "test",
		"prompt":        "",
		"subagent_type": "general",
	})
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
	if !strings.Contains(err.Error(), "prompt is required") {
		t.Errorf("error = %q, want to contain 'prompt is required'", err.Error())
	}
}

func TestTaskTool_Execute_MissingDescription(t *testing.T) {
	runner := &mockRunner{reply: "ok"}
	tool, err := NewTask(runner, testAgentTypes())
	if err != nil {
		t.Fatal(err)
	}
	_, err = tool.Execute(context.Background(), map[string]any{
		"prompt":        "do something",
		"subagent_type": "general",
	})
	if err == nil {
		t.Fatal("expected error for missing description")
	}
	if !strings.Contains(err.Error(), "description is required") {
		t.Errorf("error = %q, want to contain 'description is required'", err.Error())
	}
}

func TestTaskTool_Execute_BuiltinType(t *testing.T) {
	expectedReply := "Sub-agent result for general."
	runner := &mockRunner{reply: expectedReply}
	tool, err := NewTask(runner, testAgentTypes())
	if err != nil {
		t.Fatal(err)
	}
	reply, err := tool.Execute(context.Background(), map[string]any{
		"description":   "test task",
		"prompt":        "do something general",
		"subagent_type": "general",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if reply != expectedReply {
		t.Errorf("reply = %q, want %q", reply, expectedReply)
	}
}

func TestTaskTool_Execute_UserDefinedType(t *testing.T) {
	expectedReply := "Sub-agent result for custom."
	runner := &mockRunner{reply: expectedReply}
	tool, err := NewTask(runner, testAgentTypes())
	if err != nil {
		t.Fatal(err)
	}
	reply, err := tool.Execute(context.Background(), map[string]any{
		"description":   "custom task",
		"prompt":        "do something custom",
		"subagent_type": "custom-agent",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if reply != expectedReply {
		t.Errorf("reply = %q, want %q", reply, expectedReply)
	}
}
