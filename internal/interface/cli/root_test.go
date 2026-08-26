package cli

import (
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/interface/auth"
)

func TestRootCommand_InvalidSessionIDReturnsError(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
	}{
		{"non-uuid string", "not-a-uuid"},
		{"short", "x"},
		{"too short", "123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := NewRootCommand()
			root.SetArgs([]string{"--session-id", tt.sessionID})
			err := root.Execute()
			if err == nil {
				t.Fatal("Execute(): want error for invalid --session-id")
			}
			if !strings.Contains(err.Error(), "invalid session-id") {
				t.Errorf("error message should contain 'invalid session-id': %q", err.Error())
			}
		})
	}
}

// TestRootCommand_FlagErrorPrecedesModelCheck pins the order of the two usage checks. A bad flag
// combination is fixable without a model configured, so reporting the missing configuration
// first would send the user to solve the wrong problem.
func TestRootCommand_FlagErrorPrecedesModelCheck(t *testing.T) {
	t.Setenv("BUILDMAX_HOME", t.TempDir()) // no settings.yaml, so the model check would also fail

	root := NewRootCommand()
	root.SetArgs([]string{
		"--append-system-prompt", "a",
		"--append-system-prompt-file", "b",
		"-p", "hi",
	})
	err := root.Execute()

	if err == nil {
		t.Fatal("Execute(): want an error for two mutually exclusive flags")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want the flag conflict rather than the model configuration", err.Error())
	}
}

// --workspace reached the agent definition lookup but not the agent itself, so
// tools, AGENTS.md, and the footer's git branch all ran against the current
// directory while --agent resolved somewhere else. Both halves are pinned here.
func TestRootCommand_PassesWorkspaceToTUI(t *testing.T) {
	t.Setenv("BUILDMAX_HOME", t.TempDir())
	writeTestSettings(t, "models:\n  - model: stub\n    name: stub\n    api_key: x\n")
	workspace := t.TempDir()

	var gotWorkspace string
	var gotSandbox config.SandboxRunOverride
	orig := runTUIFunc
	runTUIFunc = func(resumeID, modelName, additionalSystemPrompt, ws string, sandboxRun config.SandboxRunOverride) error {
		gotWorkspace = ws
		gotSandbox = sandboxRun
		return nil
	}
	t.Cleanup(func() { runTUIFunc = orig })

	root := NewRootCommand()
	root.SetArgs([]string{"--workspace", workspace, "--sandbox", "--sandbox-mode", "regular"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotWorkspace != workspace {
		t.Errorf("workspace passed to the TUI = %q, want %q", gotWorkspace, workspace)
	}
	if !gotSandbox.Enable {
		t.Error("--sandbox did not reach the TUI runtime")
	}
	if gotSandbox.AutoAllowBashIfSandboxed == nil || *gotSandbox.AutoAllowBashIfSandboxed {
		t.Errorf("sandbox mode = %v, want regular", gotSandbox.AutoAllowBashIfSandboxed)
	}
}

func TestTUIAppConfigCarriesWorkspace(t *testing.T) {
	regular := false
	sandboxRun := config.SandboxRunOverride{Enable: true, AutoAllowBashIfSandboxed: &regular}
	cfg := tuiAppConfig("/tmp/some-workspace", "extra prompt", auth.ModelSource{}, sandboxRun)
	if cfg.WorkspaceDir != "/tmp/some-workspace" {
		t.Errorf("WorkspaceDir = %q, want %q", cfg.WorkspaceDir, "/tmp/some-workspace")
	}
	if cfg.AdditionalSystemPrompt != "extra prompt" {
		t.Errorf("AdditionalSystemPrompt = %q, want %q", cfg.AdditionalSystemPrompt, "extra prompt")
	}
	if !cfg.EnableMCP {
		t.Error("EnableMCP should stay on for an interactive session")
	}
	if cfg.Policy == nil {
		t.Error("an interactive session needs a policy that can ask")
	}
	if !cfg.SandboxRunOverride.Enable || cfg.SandboxRunOverride.AutoAllowBashIfSandboxed == nil || *cfg.SandboxRunOverride.AutoAllowBashIfSandboxed {
		t.Errorf("SandboxRunOverride = %+v, want enabled regular mode", cfg.SandboxRunOverride)
	}
}

// An empty --workspace still means "current directory"; the fix must not turn the
// default into an empty path handed to the runtime.
func TestTUIAppConfigWithoutWorkspaceLeavesTheDefault(t *testing.T) {
	if cfg := tuiAppConfig("", "", auth.ModelSource{}, config.SandboxRunOverride{}); cfg.WorkspaceDir != "" {
		t.Errorf("WorkspaceDir = %q, want empty so the runtime falls back to the working directory", cfg.WorkspaceDir)
	}
}

func TestRootCommand_SandboxModeErrorsPrecedeModelCheck(t *testing.T) {
	t.Setenv("BUILDMAX_HOME", t.TempDir())

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "mode requires sandbox", args: []string{"--sandbox-mode", "regular"}, want: "requires --sandbox"},
		{name: "invalid mode", args: []string{"--sandbox", "--sandbox-mode", "permissive"}, want: "want auto_allow or regular"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := NewRootCommand()
			root.SetArgs(tt.args)
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Execute() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestPrintAppConfigCarriesSandboxRunOverride(t *testing.T) {
	autoAllow := true
	opts := printOptions{SandboxRunOverride: config.SandboxRunOverride{
		Enable:                   true,
		AutoAllowBashIfSandboxed: &autoAllow,
	}}
	cfg := printAppConfig(opts, auth.ModelSource{})
	if !cfg.SandboxRunOverride.Enable || cfg.SandboxRunOverride.AutoAllowBashIfSandboxed == nil || !*cfg.SandboxRunOverride.AutoAllowBashIfSandboxed {
		t.Errorf("SandboxRunOverride = %+v, want enabled auto_allow mode", cfg.SandboxRunOverride)
	}
}

// Cobra re-adds the default completion command on every Execute, so hiding it has to survive
// that, and the generated script has to keep working for anyone who already installed one.
func TestRootCommand_CompletionStaysOutOfHelp(t *testing.T) {
	var out strings.Builder
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(--help): %v", err)
	}
	if strings.Contains(out.String(), "completion") {
		t.Errorf("help should not list the completion command:\n%s", out.String())
	}

	out.Reset()
	root = NewRootCommand()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"completion", "zsh"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(completion zsh): %v", err)
	}
	if !strings.Contains(out.String(), "compdef") {
		t.Error("completion zsh should still print a usable script")
	}
}
