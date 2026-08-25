package agentapp

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/agent"
	"github.com/gougoujiang/buildmax/internal/core/llm"
)

// TestConfiguredPolicy_LayersOverFallback checks the wiring that makes
// settings.yaml matter: a rule answers, and anything unmatched falls through to
// the surface policy, which has no opinion.
func TestConfiguredPolicy_LayersOverFallback(t *testing.T) {
	res := config.ResolvePermissions(config.ToolsConfig{Permissions: map[string]string{
		"Write":                "allow",
		"Task":                 "deny",
		"CallMcpTool:github/*": "allow",
	}})
	pol := NewConfiguredPolicy(res, agent.AllowAllPolicy())

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
	fallback := agent.AllowAllPolicy()
	if got := NewConfiguredPolicy(config.PermissionResolution{}, fallback); got != fallback {
		t.Errorf("with no rules, want the fallback returned unwrapped")
	}
}
