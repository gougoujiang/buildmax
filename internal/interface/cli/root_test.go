package cli

import (
	"strings"
	"testing"
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

	var got string
	orig := runTUIFunc
	runTUIFunc = func(resumeID, modelName, additionalSystemPrompt, ws string) error {
		got = ws
		return nil
	}
	t.Cleanup(func() { runTUIFunc = orig })

	root := NewRootCommand()
	root.SetArgs([]string{"--workspace", workspace})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != workspace {
		t.Errorf("workspace passed to the TUI = %q, want %q", got, workspace)
	}
}

func TestTUIAppConfigCarriesWorkspace(t *testing.T) {
	cfg := tuiAppConfig("/tmp/some-workspace", "extra prompt")
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
}

// An empty --workspace still means "current directory"; the fix must not turn the
// default into an empty path handed to the runtime.
func TestTUIAppConfigWithoutWorkspaceLeavesTheDefault(t *testing.T) {
	if cfg := tuiAppConfig("", ""); cfg.WorkspaceDir != "" {
		t.Errorf("WorkspaceDir = %q, want empty so the runtime falls back to the working directory", cfg.WorkspaceDir)
	}
}
