package agentapp

import (
	"context"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
	tools "github.com/gougoujiang/buildmax/internal/tool"
	"github.com/gougoujiang/buildmax/internal/util"
)

type stubIssueClient struct{}

func (stubIssueClient) Issue(context.Context) (tools.IssueSnapshot, error) {
	return tools.IssueSnapshot{}, nil
}

func (stubIssueClient) Report(context.Context, tools.IssueReport) error { return nil }

func registryNames(t *testing.T, cfg AppConfig) map[string]bool {
	t.Helper()
	cfg.WorkspaceDir = t.TempDir()
	app, err := NewAgentApp(cfg)
	if err != nil {
		t.Fatalf("NewAgentApp: %v", err)
	}
	defer func() { _ = app.Close() }()
	registry, err := app.buildToolRegistry(nil)
	if err != nil {
		t.Fatalf("buildToolRegistry: %v", err)
	}
	return toolNames(registry.Tools())
}

// A run with no Issue, or no way to reach the one it has, must not be offered
// tools that can only fail. See docs/design/issue-agent-access.md section 8.
func TestIssueToolsAreAbsentWithoutAClient(t *testing.T) {
	names := registryNames(t, AppConfig{})
	if names[tools.ToolNameGetIssue] || names[tools.ToolNameReportToIssue] {
		t.Error("a run with no Issue must not be offered the Issue tools")
	}
	if !names[tools.ToolNameRead] {
		t.Error("the ordinary tools should still be there")
	}
}

func TestIssueToolsArePresentWithAClient(t *testing.T) {
	names := registryNames(t, AppConfig{IssueClient: stubIssueClient{}})
	if !names[tools.ToolNameGetIssue] {
		t.Error("a run working an Issue should be able to read it")
	}
	if !names[tools.ToolNameReportToIssue] {
		t.Error("a run working an Issue should be able to report on it")
	}
}

// The Issue tools are appended after BuildAgentTypes, like Worktree and the Job
// tools, which is what keeps them out of subagents and delegates: a subagent
// reports to its parent, and several of them writing into one team thread would
// make that thread's attribution unreadable. buildBaseTools is what every
// delegate registry is built from, so their absence there is the guarantee.
func TestIssueToolsAreNeverInTheBaseSet(t *testing.T) {
	names := toolNames(buildBaseTools(nil, util.FixedRoot(t.TempDir()), stubTool{}, agent.NoopSandbox{}, stubPublisher{}, nil))
	if names[tools.ToolNameGetIssue] || names[tools.ToolNameReportToIssue] {
		t.Error("the Issue tools are in the base set, so a delegate or subagent could get them")
	}
}
