package agentapp

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
)

func TestPolicyBothReturnAllow(t *testing.T) {
	cases := []struct {
		name     string
		toolName string
		args     map[string]any
	}{
		{"normal bash", "Bash", map[string]any{"command": "go test ./..."}},
		{"rm -rf subdir", "Bash", map[string]any{"command": "rm -rf ./tmp"}},
		{"read .env", "Read", map[string]any{"file_path": ".env"}},
		{"write .env", "Write", map[string]any{"file_path": ".env", "content": "X=1"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, pol := range []interface {
				Check(string, map[string]any) llm.ToolAction
			}{NewInteractivePolicy(), NewNonInteractivePolicy()} {
				if got := pol.Check(tc.toolName, tc.args); got != llm.ToolActionAllow {
					t.Errorf("policy.Check(%q) = %v; want Allow (defer to tool)", tc.toolName, got)
				}
			}
		})
	}
}
