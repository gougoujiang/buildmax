package agentapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
	"github.com/gougoujiang/buildmax/internal/core/localproject"
)

// switchApp builds the smallest AgentApp the derived-configuration reload
// touches: a root, the registries it fills, and a hook manager to refresh.
func switchApp(t *testing.T, root string) *AgentApp {
	t.Helper()
	app := &AgentApp{
		workspace:         NewMovableRoot(root),
		skillsRegistry:    &SkillRegistry{},
		subagentsRegistry: &SubAgentRegistry{},
		toolRegistries:    map[string]cllm.ToolRegistry{},
	}
	app.hookManager = NewHookManager(config.MergeHooks(), nil)
	app.hooks = app.hookManager
	if err := app.skillsRegistry.Load(root, nil); err != nil {
		t.Fatalf("initial skill load: %v", err)
	}
	return app
}

func writeSkill(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, ".buildmax", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: " + description + "\n---\n\nDo the thing.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The skills a session is offered must be the ones in the tree it is working
// in. Left stale, a session in a worktree would be offered the launch tree's
// skills and told they apply here.
func TestSkillsFollowTheRoot(t *testing.T) {
	launch := t.TempDir()
	other := t.TempDir()
	writeSkill(t, launch, "launch-only", "only in the launch tree")
	writeSkill(t, other, "other-only", "only in the other tree")

	app := switchApp(t, launch)
	if err := app.skillsRegistry.Load(launch, nil); err != nil {
		t.Fatal(err)
	}
	if !hasSkill(app, "launch-only") {
		t.Fatal("the launch tree's skill was not loaded to begin with")
	}

	sessionRoot{app: app}.Set(other)

	if hasSkill(app, "launch-only") {
		t.Error("the launch tree's skill is still offered after the move")
	}
	if !hasSkill(app, "other-only") {
		t.Error("the new tree's skill was not picked up")
	}
}

func hasSkill(app *AgentApp, name string) bool {
	for _, e := range app.skillsRegistry.Entries() {
		if e.Name == name {
			return true
		}
	}
	return false
}

// Hooks are a gate on what the agent may do, so running the launch tree's
// hooks against another tree's files is the worst of the stale-configuration
// cases.
func TestWorkspaceHooksFollowTheRoot(t *testing.T) {
	launch := t.TempDir()
	other := t.TempDir()
	writeHooks(t, other, `pre_tool_use:
  - matcher: "Bash"
    type: command
    command: "echo blocked"
`)

	app := switchApp(t, launch)
	if n := app.hookManager.Status().TotalHooks; n != 0 {
		t.Fatalf("the launch tree has %d hooks, want none to start", n)
	}

	sessionRoot{app: app}.Set(other)

	if n := app.hookManager.Status().TotalHooks; n != 1 {
		t.Fatalf("after the move the manager dispatches %d hooks, want the new tree's 1", n)
	}

	// And moving back drops them again, or a hook would outlive the tree that
	// declared it.
	sessionRoot{app: app}.Set(launch)
	if n := app.hookManager.Status().TotalHooks; n != 0 {
		t.Fatalf("after moving back the manager dispatches %d hooks, want none", n)
	}
}

func writeHooks(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, ".buildmax")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hooks.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The cached per-model registries hold the Skill tool and the subagent types
// built from a snapshot. A move has to drop them or the session keeps being
// offered what the launch tree had.
func TestToolRegistryCacheIsDroppedOnAMove(t *testing.T) {
	launch := t.TempDir()
	app := switchApp(t, launch)
	app.toolRegistries["some-model"] = cllm.NewToolRegistry()

	sessionRoot{app: app}.Set(t.TempDir())

	if len(app.toolRegistries) != 0 {
		t.Fatalf("the registry cache still holds %d entries after a move", len(app.toolRegistries))
	}
}

// The AGENTS.md layer is the one derived thing nothing here reloads: the
// prompt is built from the current root every turn, so it already follows.
// This test is what makes that claim safe to rely on rather than a comment.
func TestPromptLayerFollowsTheRoot(t *testing.T) {
	launch := t.TempDir()
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(launch, AgentsMdFilename), []byte("LAUNCH TREE RULES"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, AgentsMdFilename), []byte("OTHER TREE RULES"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := switchApp(t, launch)
	before := BuildEffectiveSystemPrompt(app.workspace.Root(), "", "", PromptCapabilities{})
	if !strings.Contains(before, "LAUNCH TREE RULES") {
		t.Fatal("the launch tree's AGENTS.md is not in the prompt to begin with")
	}

	sessionRoot{app: app}.Set(other)

	after := BuildEffectiveSystemPrompt(app.workspace.Root(), "", "", PromptCapabilities{})
	if strings.Contains(after, "LAUNCH TREE RULES") {
		t.Error("the launch tree's AGENTS.md is still in the prompt after the move")
	}
	if !strings.Contains(after, "OTHER TREE RULES") {
		t.Error("the new tree's AGENTS.md did not reach the prompt")
	}
	if before == after {
		t.Error("the prompt did not change; the cacheable prefix should have been invalidated by the move")
	}
}

// Everything the root decides follows a move; the Project does not. A session
// that re-resolved its Project on entering a worktree would land in the same
// one today and would silently start switching memory domains the moment
// resolution changed. See docs/design/local-project-memory.md §6.2.
func TestProjectDoesNotFollowTheRoot(t *testing.T) {
	launch := t.TempDir()
	app := switchApp(t, launch)
	app.project = localproject.Project{ID: "hyzc3kqxa2vw7m4t9pbn", Name: "repo"}
	app.sessionManager = NewSessionManager(t.TempDir()).ForProject(app.project.ID)

	sessionRoot{app: app}.Set(t.TempDir())

	if got := app.Project().ID; got != "hyzc3kqxa2vw7m4t9pbn" {
		t.Errorf("Project after a root move = %q, want it unchanged", got)
	}
	if got := app.sessionManager.ProjectID(); got != "hyzc3kqxa2vw7m4t9pbn" {
		t.Errorf("new sessions after a root move would belong to %q, want the unchanged project", got)
	}
	if app.workspace.Root() == launch {
		t.Error("the root did not move, so this test proved nothing")
	}
}
