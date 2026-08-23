package agentapp

import (
	"context"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/subagent"
	tools "github.com/gougoujiang/buildmax/internal/tool"
)

type noopSubAgentRunner struct{}

func (noopSubAgentRunner) RunSubAgent(context.Context, tools.SubAgentRunOpts, string) (string, error) {
	return "", nil
}

// TestAgentTypeAccess pins the classification the parallel scheduler reads off
// the real built-in agent types. It is written against BuildAgentTypes rather
// than the type table because the eligibility follows from the resolved tool
// set: adding one writing tool to explore, or letting its names fail to
// resolve, must take it out of the read-only group.
func TestAgentTypeAccess(t *testing.T) {
	registry := llm.NewToolRegistry()
	registry.AppendTools(
		tools.NewReadFile(t.TempDir()),
		tools.NewGlob(t.TempDir()),
		tools.NewGrep(t.TempDir()),
		tools.NewBash(t.TempDir()),
		tools.NewWriteFile(t.TempDir()),
	)
	userDefs := []subagent.Def{
		{Name: "reader", ToolNames: []string{tools.ToolNameRead, tools.ToolNameGrep}, SystemPrompt: "read", Description: "read"},
		{Name: "writer", ToolNames: []string{tools.ToolNameRead, tools.ToolNameWrite}, SystemPrompt: "write", Description: "write"},
	}

	task, err := tools.NewTask(noopSubAgentRunner{}, BuildAgentTypes(registry, userDefs))
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}

	tests := map[string]llm.Access{
		"explore": llm.AccessReadOnly,
		"reader":  llm.AccessReadOnly,
		"general": llm.AccessWrite,
		"shell":   llm.AccessWrite,
		"writer":  llm.AccessWrite,
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			got := task.Access(map[string]any{"subagent_type": name})
			if got != want {
				t.Errorf("Access(%q) = %v, want %v", name, got, want)
			}
		})
	}
}
