package agentapp

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
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
			for _, pol := range []agent.ToolPolicy{NewInteractivePolicy(), NewNonInteractivePolicy()} {
				if got, ok := pol.Check(tc.toolName, "", tc.args); ok || got != llm.ToolActionAllow {
					t.Errorf("policy.Check(%q) = (%v, %v); want (Allow, false) — no opinion", tc.toolName, got, ok)
				}
			}
		})
	}
}

// TestConfiguredPolicy_LayersOverFallback checks the wiring that makes
// settings.yaml matter: a rule answers, and anything unmatched falls through to
// the surface policy, which has no opinion.
func TestConfiguredPolicy_LayersOverFallback(t *testing.T) {
	res := config.ResolvePermissions(config.ToolsConfig{Permissions: map[string]string{
		"Write":                "allow",
		"Task":                 "deny",
		"CallMcpTool:github/*": "allow",
	}})
	pol := NewConfiguredPolicy(res, NewInteractivePolicy())

	for _, tc := range []struct {
		name, scope string
		want        llm.ToolAction
		wantOK      bool
	}{
		{"Write", "", llm.ToolActionAllow, true},
		{"Task", "", llm.ToolActionDeny, true},
		{"CallMcpTool", "CallMcpTool:github/get_issue", llm.ToolActionAllow, true},
		{"CallMcpTool", "CallMcpTool:jira/get_issue", llm.ToolActionAllow, false},
		{"Read", "", llm.ToolActionAllow, false},
	} {
		got, ok := pol.Check(tc.name, tc.scope, nil)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("Check(%q, %q) = (%v, %v), want (%v, %v)", tc.name, tc.scope, got, ok, tc.want, tc.wantOK)
		}
	}
}

// TestConfiguredPolicy_NoRulesReturnsFallback keeps the zero-config path free of
// a wrapper that would answer every call the same way.
func TestConfiguredPolicy_NoRulesReturnsFallback(t *testing.T) {
	fallback := NewInteractivePolicy()
	if got := NewConfiguredPolicy(config.PermissionResolution{}, fallback); got != fallback {
		t.Errorf("with no rules, want the fallback returned unwrapped")
	}
}
