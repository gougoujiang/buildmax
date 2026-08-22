package config

import (
	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
	"os"
	"path/filepath"
	"testing"
)

// TestHooksConfig_LoadFromSettings asserts that hooks declared under the snake_case
// "hooks" block of settings.yaml round-trip through LoadSettings.
func TestHooksConfig_LoadFromSettings(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)

	settings := `
log_level: info
models:
  - model: openai/gpt-4o-mini
    api_key: test-key
hooks:
  pre_tool_use:
    - matcher: "writefile|editfile"
      command: "./guard.sh"
      timeout: 5
  post_tool_use:
    - matcher: "writefile"
      command: "gofmt -w ."
  stop:
    - command: "/usr/local/bin/audit.sh"
`
	if err := os.WriteFile(filepath.Join(tmp, "settings.yaml"), []byte(settings), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if got := s.Hooks.Entries(corehook.EventPreToolUse); len(got) != 1 || got[0].Matcher != "writefile|editfile" || got[0].Command != "./guard.sh" || got[0].Timeout != 5 {
		t.Errorf("pre_tool_use entries = %+v", got)
	}
	if got := s.Hooks.Entries(corehook.EventPostToolUse); len(got) != 1 || got[0].Command != "gofmt -w ." {
		t.Errorf("post_tool_use entries = %+v", got)
	}
	if got := s.Hooks.Entries(corehook.EventStop); len(got) != 1 || got[0].Command != "/usr/local/bin/audit.sh" {
		t.Errorf("stop entries = %+v", got)
	}
	if got := s.Hooks.Entries("UnknownEvent"); got != nil {
		t.Errorf("unknown event returned %d entries, want nil", len(got))
	}
	if s.Hooks.IsEmpty() {
		t.Error("IsEmpty = true, want false when entries are configured")
	}
}

func TestHooksConfig_IsEmptyDefault(t *testing.T) {
	var h corehook.Config
	if !h.IsEmpty() {
		t.Error("zero-value corehook.Config.IsEmpty = false")
	}
}

// TestHookEntry_ResolvedTypeDefault asserts that omitting "type" falls back
// to command, matching pre-v2 behavior.
func TestHookEntry_ResolvedTypeDefault(t *testing.T) {
	if got := (corehook.Entry{}).ResolvedType(); got != corehook.TypeCommand {
		t.Errorf("ResolvedType() = %q, want %q", got, corehook.TypeCommand)
	}
	if got := (corehook.Entry{Type: corehook.TypeHTTP}).ResolvedType(); got != corehook.TypeHTTP {
		t.Errorf("ResolvedType() = %q, want %q", got, corehook.TypeHTTP)
	}
}

// TestHooksConfig_TypeAndExtraFields asserts that polymorphic per-type
// fields (url, server, prompt, etc.) round-trip from settings.yaml.
func TestHooksConfig_TypeAndExtraFields(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)

	settings := `
hooks:
  pre_tool_use:
    - type: http
      matcher: "bash"
      url: "https://policy.example/check"
      headers:
        Authorization: "Bearer $POLICY_TOKEN"
      allowed_env: [POLICY_TOKEN]
      timeout: 7
    - type: mcp_tool
      matcher: "writefile"
      server: "code-scanner"
      tool: "scan_file"
      input:
        path: "${tool_args.path}"
    - type: prompt
      prompt: "judge $ARGUMENTS"
      model: "fast"
`
	if err := os.WriteFile(filepath.Join(tmp, "settings.yaml"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	entries := s.Hooks.Entries(corehook.EventPreToolUse)
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(entries))
	}
	if entries[0].Type != corehook.TypeHTTP || entries[0].URL != "https://policy.example/check" {
		t.Errorf("http entry = %+v", entries[0])
	}
	// Viper lowercases map keys on load; HTTP headers are case-insensitive
	// so the driver normalizes back to canonical form when sending.
	if entries[0].Headers["authorization"] != "Bearer $POLICY_TOKEN" {
		t.Errorf("http headers = %v", entries[0].Headers)
	}
	if len(entries[0].AllowedEnv) != 1 || entries[0].AllowedEnv[0] != "POLICY_TOKEN" {
		t.Errorf("allowed_env = %v", entries[0].AllowedEnv)
	}
	if entries[1].Type != corehook.TypeMCP || entries[1].Server != "code-scanner" || entries[1].Tool != "scan_file" {
		t.Errorf("mcp_tool entry = %+v", entries[1])
	}
	if entries[1].Input["path"] != "${tool_args.path}" {
		t.Errorf("mcp input = %v", entries[1].Input)
	}
	if entries[2].Type != corehook.TypePrompt || entries[2].Prompt != "judge $ARGUMENTS" || entries[2].Model != "fast" {
		t.Errorf("prompt entry = %+v", entries[2])
	}
}

// TestLoadWorkspaceHooks_MissingFile asserts a missing file yields an empty
// config rather than an error.
func TestLoadWorkspaceHooks_MissingFile(t *testing.T) {
	cfg, err := LoadWorkspaceHooks(t.TempDir())
	if err != nil {
		t.Fatalf("LoadWorkspaceHooks: %v", err)
	}
	if !cfg.IsEmpty() {
		t.Errorf("expected empty config, got %+v", cfg)
	}
}

// TestLoadWorkspaceHooks_EmptyWorkspace asserts the helper short-circuits on
// an empty path.
func TestLoadWorkspaceHooks_EmptyWorkspace(t *testing.T) {
	cfg, err := LoadWorkspaceHooks("")
	if err != nil || !cfg.IsEmpty() {
		t.Errorf("LoadWorkspaceHooks(\"\") = (%+v, %v); want empty, nil", cfg, err)
	}
}

// TestLoadWorkspaceHooks_Present asserts that a real file loads back.
func TestLoadWorkspaceHooks_Present(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, ".buildmax")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `
pre_tool_use:
  - matcher: "writefile"
    command: "./ws-guard.sh"
post_tool_use:
  - matcher: "writefile|editfile"
    command: "gofmt -w ."
`
	if err := os.WriteFile(filepath.Join(dir, "hooks.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadWorkspaceHooks(ws)
	if err != nil {
		t.Fatalf("LoadWorkspaceHooks: %v", err)
	}
	if got := cfg.Entries(corehook.EventPreToolUse); len(got) != 1 || got[0].Command != "./ws-guard.sh" {
		t.Errorf("pre_tool_use = %+v", got)
	}
	if got := cfg.Entries(corehook.EventPostToolUse); len(got) != 1 || got[0].Command != "gofmt -w ." {
		t.Errorf("post_tool_use = %+v", got)
	}
}

// TestLoadWorkspaceHooks_Malformed asserts that malformed YAML is reported.
func TestLoadWorkspaceHooks_Malformed(t *testing.T) {
	ws := t.TempDir()
	dir := filepath.Join(ws, ".buildmax")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hooks.yaml"), []byte("not: [valid: yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWorkspaceHooks(ws); err == nil {
		t.Error("LoadWorkspaceHooks: expected error for malformed YAML, got nil")
	}
}

// TestMergeHooks_AdditiveOrder asserts that global entries come before
// workspace entries for the same event.
func TestMergeHooks_AdditiveOrder(t *testing.T) {
	global := corehook.Config{
		PreToolUse: []corehook.Entry{{Command: "global-1"}, {Command: "global-2"}},
		Stop:       []corehook.Entry{{Command: "global-end"}},
	}
	ws := corehook.Config{
		PreToolUse:  []corehook.Entry{{Command: "ws-1"}},
		PostCompact: []corehook.Entry{{Command: "ws-compact"}},
	}
	merged := MergeHooks(global, ws)
	got := merged.Entries(corehook.EventPreToolUse)
	if len(got) != 3 || got[0].Command != "global-1" || got[1].Command != "global-2" || got[2].Command != "ws-1" {
		t.Errorf("pre_tool_use merged = %v", commands(got))
	}
	if got := merged.Entries(corehook.EventStop); len(got) != 1 || got[0].Command != "global-end" {
		t.Errorf("stop merged = %v", commands(got))
	}
	if got := merged.Entries(corehook.EventPostCompact); len(got) != 1 || got[0].Command != "ws-compact" {
		t.Errorf("post_compact merged = %v", commands(got))
	}
}

// TestMergeHooks_BothEmpty asserts a zero-result merge.
func TestMergeHooks_BothEmpty(t *testing.T) {
	merged := MergeHooks(corehook.Config{}, corehook.Config{})
	if !merged.IsEmpty() {
		t.Errorf("expected empty merged config, got %+v", merged)
	}
}

// TestMergeHooks_InputsNotMutated asserts that the merge does not modify
// either input slice.
func TestMergeHooks_InputsNotMutated(t *testing.T) {
	global := corehook.Config{PreToolUse: []corehook.Entry{{Command: "g"}}}
	ws := corehook.Config{PreToolUse: []corehook.Entry{{Command: "w"}}}
	merged := MergeHooks(global, ws)
	merged.PreToolUse[0].Command = "MUTATED"
	if global.PreToolUse[0].Command != "g" {
		t.Errorf("global mutated: %v", global.PreToolUse[0].Command)
	}
	if ws.PreToolUse[0].Command != "w" {
		t.Errorf("workspace mutated: %v", ws.PreToolUse[0].Command)
	}
}

func commands(entries []corehook.Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Command
	}
	return out
}
