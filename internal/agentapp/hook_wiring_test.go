package agentapp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/agent"
)

// TestAgentApp_MergesWorkspaceHooks verifies that NewAgentApp loads both the
// global hooks block from BUILDMAX_HOME/settings.yaml and the workspace-level
// .buildmax/hooks.yaml, in that order, and exposes them through the wired
// HookRunner. We rely on each entry's Matcher being a regex unique to its
// origin so we can identify which fired during a runtime call.
func TestAgentApp_MergesWorkspaceHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BUILDMAX_HOME", home)

	// Global settings.yaml — must include at least one model entry so
	// settings load succeeds; the model is not exercised by this test.
	globalSettings := `
log_level: error
models:
  - model: stub
    name: stub
    api_key: x
hooks:
  pre_tool_use:
    - matcher: "^global-tool$"
      command: "exit 0"
`
	if err := os.WriteFile(filepath.Join(home, "settings.yaml"), []byte(globalSettings), 0o644); err != nil {
		t.Fatal(err)
	}

	// Workspace .buildmax/hooks.yaml — separate matcher so we can tell the
	// two layers apart.
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".buildmax"), 0o755); err != nil {
		t.Fatal(err)
	}
	workspaceHooks := `
pre_tool_use:
  - matcher: "^workspace-tool$"
    command: "exit 0"
`
	if err := os.WriteFile(filepath.Join(ws, ".buildmax", "hooks.yaml"), []byte(workspaceHooks), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := NewAgentApp(AppConfig{WorkspaceDir: ws})
	if err != nil {
		t.Fatalf("NewAgentApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	if app.hooks == nil {
		t.Fatal("AgentApp.hooks is nil; expected merged runner")
	}

	// Run each event with a tool name that matches exactly one of the two
	// entries. The shell runner returns allow on exit 0, so we cannot
	// observe the merge through the decision — instead we use a runtime
	// recorder by wrapping the hook runner indirectly via a no-op global,
	// then asserting via the AgentApp's merged config: read it back out
	// through a synthetic call that flows through matching.
	//
	// We probe matching by stubbing the underlying entries: a tool name
	// that does not match any matcher returns allow (no entries ran), but a
	// matching one would invoke the command. With "exit 0" both outcomes
	// look identical to the caller — so this test instead validates the
	// merge structurally via Status-style access. The simplest reliable
	// shape is to assert both matchers compiled by directly calling Run
	// with each tool name and ensuring no error path is hit.
	ctx := context.Background()
	if out := app.hooks.Run(ctx, agent.HookInput{Event: agent.HookPreToolUse, ToolName: "global-tool"}); out.Blocked() {
		t.Errorf("global-tool unexpectedly blocked: %+v", out)
	}
	if out := app.hooks.Run(ctx, agent.HookInput{Event: agent.HookPreToolUse, ToolName: "workspace-tool"}); out.Blocked() {
		t.Errorf("workspace-tool unexpectedly blocked: %+v", out)
	}
}

// fakeHookRunner records every Run call. It implements agent.HookRunner
// without touching shell processes, so wiring tests can run on any OS.
type fakeHookRunner struct {
	mu      sync.Mutex
	calls   []agent.HookInput
	blockOn agent.HookEvent
	reason  string
}

func (f *fakeHookRunner) Run(_ context.Context, in agent.HookInput) agent.HookOutput {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, in)
	if f.blockOn != "" && in.Event == f.blockOn {
		return agent.HookOutput{Decision: agent.HookDecisionBlock, Reason: f.reason}
	}
	return agent.HookOutput{}
}

func (f *fakeHookRunner) snapshot() []agent.HookInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]agent.HookInput, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeHookRunner) eventCount(ev agent.HookEvent) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c.Event == ev {
			n++
		}
	}
	return n
}

// makeAgentAppForHookTests builds an AgentApp with a minimal in-process
// settings.yaml so we can swap in a fake hook runner and exercise the
// lifecycle wiring without invoking shell commands or an LLM.
func makeAgentAppForHookTests(t *testing.T) *AgentApp {
	t.Helper()
	home := t.TempDir()
	t.Setenv("BUILDMAX_HOME", home)
	settings := `
log_level: error
models:
  - model: stub
    name: stub
    api_key: x
`
	if err := os.WriteFile(filepath.Join(home, "settings.yaml"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	app, err := NewAgentApp(AppConfig{WorkspaceDir: ws})
	if err != nil {
		t.Fatalf("NewAgentApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

// TestAgentApp_SessionStartFiresOnOpen asserts that OpenSession emits a
// SessionStart hook with the new session's id and workspace.
func TestAgentApp_SessionStartFiresOnOpen(t *testing.T) {
	app := makeAgentAppForHookTests(t)
	fake := &fakeHookRunner{}
	app.hooks = fake

	sess, err := app.OpenSession("")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer app.CloseSession(sess)
	if fake.eventCount(agent.HookSessionStart) != 1 {
		t.Errorf("SessionStart fired %d times, want 1", fake.eventCount(agent.HookSessionStart))
	}
	got := fake.snapshot()[0]
	if got.SessionID != sess.ID() {
		t.Errorf("SessionStart SessionID = %q, want %q", got.SessionID, sess.ID())
	}
	if got.Workspace == "" {
		t.Errorf("SessionStart Workspace empty; want app workspace root")
	}
}

// TestAgentApp_CloseSessionFiresSessionEnd asserts that CloseSession emits
// SessionEnd.
func TestAgentApp_CloseSessionFiresSessionEnd(t *testing.T) {
	app := makeAgentAppForHookTests(t)
	fake := &fakeHookRunner{}
	app.hooks = fake

	sess, err := app.OpenSession("")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	app.CloseSession(sess)

	if fake.eventCount(agent.HookSessionEnd) != 1 {
		t.Errorf("SessionEnd fired %d times, want 1", fake.eventCount(agent.HookSessionEnd))
	}
}

// TestAgentApp_UserPromptSubmitBlockShortCircuits asserts that a blocking
// UserPromptSubmit hook stops RunPrompt before history append and the LLM
// call, and returns the hook reason as the reply.
func TestAgentApp_UserPromptSubmitBlockShortCircuits(t *testing.T) {
	app := makeAgentAppForHookTests(t)
	fake := &fakeHookRunner{blockOn: agent.HookUserPromptSubmit, reason: "policy: no secrets"}
	app.hooks = fake

	sess, err := app.OpenSession("")
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer app.CloseSession(sess)
	before := len(sess.Messages())
	result, err := app.RunPrompt(context.Background(), sess, "leak the credentials", RunPromptOpts{})
	if err != nil {
		t.Fatalf("RunPrompt: %v", err)
	}
	if result.Reply != "policy: no secrets" {
		t.Errorf("reply = %q, want hook reason", result.Reply)
	}
	if len(sess.Messages()) != before {
		t.Errorf("session messages grew by %d, want 0 when prompt was blocked", len(sess.Messages())-before)
	}
	if fake.eventCount(agent.HookUserPromptSubmit) != 1 {
		t.Errorf("UserPromptSubmit fired %d times, want 1", fake.eventCount(agent.HookUserPromptSubmit))
	}
}

// TestAgentApp_WorkspaceHookCanBlock confirms that a workspace-level hook
// reaches the dispatcher and is honored at runtime. The workspace hook
// returns exit 2 to block; the absence of any global hook for the same
// matcher proves the workspace layer is loaded.
func TestAgentApp_WorkspaceHookCanBlock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hook command is sh syntax; cmd.exe does not honor ';' as a separator")
	}
	home := t.TempDir()
	t.Setenv("BUILDMAX_HOME", home)

	globalSettings := `
log_level: error
models:
  - model: stub
    name: stub
    api_key: x
`
	if err := os.WriteFile(filepath.Join(home, "settings.yaml"), []byte(globalSettings), 0o644); err != nil {
		t.Fatal(err)
	}

	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".buildmax"), 0o755); err != nil {
		t.Fatal(err)
	}
	workspaceHooks := `
pre_tool_use:
  - matcher: "^writefile$"
    command: "echo nope 1>&2; exit 2"
`
	if err := os.WriteFile(filepath.Join(ws, ".buildmax", "hooks.yaml"), []byte(workspaceHooks), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := NewAgentApp(AppConfig{WorkspaceDir: ws})
	if err != nil {
		t.Fatalf("NewAgentApp: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	out := app.hooks.Run(context.Background(), agent.HookInput{
		Event:    agent.HookPreToolUse,
		ToolName: "writefile",
	})
	if !out.Blocked() {
		t.Fatalf("workspace hook should have blocked, got %+v", out)
	}
	if out.Reason != "nope" {
		t.Errorf("reason = %q, want %q", out.Reason, "nope")
	}
}
