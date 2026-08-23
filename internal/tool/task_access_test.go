package tool

import (
	"context"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// readOnlyAgentTool is a mockAgentTool that declares itself read-only, which
// mockAgentTool deliberately does not: an undeclared tool is a write, and the
// existing fixtures rely on that.
type readOnlyAgentTool struct{ mockAgentTool }

func (t *readOnlyAgentTool) Access(_ map[string]any) llm.Access { return llm.AccessReadOnly }

func readOnlyTool(name string) llm.Tool { return &readOnlyAgentTool{mockAgentTool{name: name}} }

func writingTool(name string) llm.Tool { return &mockAgentTool{name: name} }

// statelessRunner records nothing, unlike mockRunner, so a concurrent Execute
// exercises the tool rather than the fixture.
type statelessRunner struct{ reply string }

func (r *statelessRunner) RunSubAgent(_ context.Context, _ SubAgentRunOpts, _ string) (string, error) {
	return r.reply, nil
}

func accessTaskTool(t *testing.T, types map[string]AgentTypeConfig) *TaskTool {
	t.Helper()
	tool, err := NewTask(&statelessRunner{reply: "done"}, types)
	if err != nil {
		t.Fatalf("NewTask: %v", err)
	}
	return tool
}

func TestTaskTool_Access(t *testing.T) {
	tool := accessTaskTool(t, map[string]AgentTypeConfig{
		"explore": {Tools: []llm.Tool{readOnlyTool(ToolNameRead), readOnlyTool(ToolNameGrep)}},
		"general": {Tools: []llm.Tool{readOnlyTool(ToolNameRead), writingTool(ToolNameBash)}},
		"shell":   {Tools: []llm.Tool{writingTool(ToolNameBash)}},
		"empty":   {Tools: nil},
		// A type whose only writer is Task itself: Execute strips it before
		// the run, so it must not decide the answer.
		"nested": {Tools: []llm.Tool{readOnlyTool(ToolNameRead), writingTool(ToolNameTask)}},
	})

	tests := []struct {
		name string
		args map[string]any
		want llm.Access
	}{
		{"read-only type", map[string]any{"subagent_type": "explore"}, llm.AccessReadOnly},
		{"type with a writing tool", map[string]any{"subagent_type": "general"}, llm.AccessWrite},
		{"shell", map[string]any{"subagent_type": "shell"}, llm.AccessWrite},
		{"empty tool set", map[string]any{"subagent_type": "empty"}, llm.AccessWrite},
		{"Task in the set is skipped", map[string]any{"subagent_type": "nested"}, llm.AccessReadOnly},
		{"surrounding whitespace", map[string]any{"subagent_type": " explore "}, llm.AccessReadOnly},
		{"unknown type", map[string]any{"subagent_type": "nope"}, llm.AccessWrite},
		{"missing type", map[string]any{}, llm.AccessWrite},
		{"nil args", nil, llm.AccessWrite},
		{"background run", map[string]any{"subagent_type": "explore", "run_in_background": true}, llm.AccessWrite},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tool.Access(tc.args); got != tc.want {
				t.Errorf("Access(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

// WithJobs returns a copy, so the classification computed at construction has
// to survive it.
func TestTaskTool_AccessSurvivesWithJobs(t *testing.T) {
	tool := accessTaskTool(t, map[string]AgentTypeConfig{
		"explore": {Tools: []llm.Tool{readOnlyTool(ToolNameRead)}},
	}).WithJobs(nil, "/ws")
	if got := tool.Access(map[string]any{"subagent_type": "explore"}); got != llm.AccessReadOnly {
		t.Errorf("Access after WithJobs = %v, want read-only", got)
	}
}

// A read-only sub-agent still runs a full nested loop, so Execute must be safe
// to call from several goroutines at once. Run this one under -race.
func TestTaskTool_ExecuteIsConcurrencySafe(t *testing.T) {
	tool := accessTaskTool(t, map[string]AgentTypeConfig{
		"explore": {Tools: []llm.Tool{readOnlyTool(ToolNameRead)}},
	})
	args := map[string]any{"description": "look", "prompt": "find it", "subagent_type": "explore"}
	results := make(chan string, 4)
	for range 4 {
		go func() {
			out, err := tool.Execute(context.Background(), args)
			if err != nil {
				results <- "error: " + err.Error()
				return
			}
			results <- out
		}()
	}
	for range 4 {
		if got := <-results; got != "done" {
			t.Errorf("Execute = %q, want %q", got, "done")
		}
	}
}
