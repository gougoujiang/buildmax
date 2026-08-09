package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const settingsValidOne = `
models:
  - model: glm4.5
    name: My GLM
    api_url: https://api.example.com
    api_key: sk-xxx
`
const settingsValidTwo = `
models:
  - model: a
    api_url: https://a
    api_key: k1
  - model: b
    name: B
    api_url: https://b
    api_key: k2
`
const settingsEmptyModels = "models: []\n"
const settingsInvalidYAML = "models: [\n  invalid: {\n"

func TestDataDir_Default(t *testing.T) {
	// Ensure BUILDMAX_HOME does not affect this test (unset or restore after).
	t.Setenv(EnvKeyBuildmaxHome, "")
	dir := DataDir()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	if !strings.Contains(dir, home) {
		t.Errorf("DataDir() = %q, want path containing %q", dir, home)
	}
	if !strings.HasSuffix(filepath.Clean(dir), ".buildmax") {
		t.Errorf("DataDir() = %q, want path ending with .buildmax", dir)
	}
}

func TestDataDir_Override(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	dir := DataDir()
	want := filepath.Clean(tmp)
	if dir != want {
		t.Errorf("DataDir() = %q, want %q", dir, want)
	}
}

func TestSessionsDir_Default(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxHome, "")
	dir := SessionsDir()
	if !strings.HasSuffix(filepath.Clean(dir), filepath.Join(".buildmax", "sessions")) {
		t.Errorf("SessionsDir() = %q, want path ending with .buildmax/sessions", dir)
	}
}

func TestSessionsDir_Override(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	dir := SessionsDir()
	want := filepath.Join(filepath.Clean(tmp), "sessions")
	if dir != want {
		t.Errorf("SessionsDir() = %q, want %q", dir, want)
	}
}

func TestLogsDir_Default(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxHome, "")
	dir := LogsDir()
	if !strings.HasSuffix(filepath.Clean(dir), filepath.Join(".buildmax", "logs")) {
		t.Errorf("LogsDir() = %q, want path ending with .buildmax/logs", dir)
	}
}

func TestLogsDir_Override(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	dir := LogsDir()
	want := filepath.Join(filepath.Clean(tmp), "logs")
	if dir != want {
		t.Errorf("LogsDir() = %q, want %q", dir, want)
	}
}

func TestSkillSearchPaths_Order(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxHome, "")
	workspace := filepath.Join("C:", "projects", "myapp")
	paths := SkillSearchPaths(workspace)

	if len(paths) != 2 {
		t.Fatalf("SkillSearchPaths returned %d paths, want 2", len(paths))
	}

	// 1. project-level .buildmax/skills
	want0 := filepath.Join(workspace, ".buildmax", "skills")
	if paths[0] != want0 {
		t.Errorf("paths[0] = %q, want %q", paths[0], want0)
	}

	// 2. global DataDir()/skills
	want1 := filepath.Join(DataDir(), "skills")
	if paths[1] != want1 {
		t.Errorf("paths[1] = %q, want %q", paths[1], want1)
	}
}

func TestSkillSearchPaths_HomeDirOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	workspace := filepath.Join("D:", "work", "project")
	paths := SkillSearchPaths(workspace)

	if len(paths) != 2 {
		t.Fatalf("SkillSearchPaths returned %d paths, want 2", len(paths))
	}

	// Global path should use the overridden DataDir.
	wantGlobal := filepath.Join(filepath.Clean(tmp), "skills")
	if paths[1] != wantGlobal {
		t.Errorf("paths[1] = %q, want %q (BUILDMAX_HOME override)", paths[1], wantGlobal)
	}
}

func TestAgentDefsSearchPaths_Order(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxHome, "")
	workspace := filepath.Join("C:", "projects", "myapp")
	paths := AgentDefsSearchPaths(workspace)

	if len(paths) != 2 {
		t.Fatalf("AgentDefsSearchPaths returned %d paths, want 2", len(paths))
	}

	// 1. project-level .buildmax/agents
	want0 := filepath.Join(workspace, ".buildmax", "agents")
	if paths[0] != want0 {
		t.Errorf("paths[0] = %q, want %q", paths[0], want0)
	}

	// 2. global DataDir()/agents
	want1 := filepath.Join(DataDir(), "agents")
	if paths[1] != want1 {
		t.Errorf("paths[1] = %q, want %q", paths[1], want1)
	}
}

func TestAgentDefsSearchPaths_HomeDirOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	workspace := filepath.Join("D:", "work", "project")
	paths := AgentDefsSearchPaths(workspace)

	if len(paths) != 2 {
		t.Fatalf("AgentDefsSearchPaths returned %d paths, want 2", len(paths))
	}

	// Global path should use the overridden DataDir.
	wantGlobal := filepath.Join(filepath.Clean(tmp), "agents")
	if paths[1] != wantGlobal {
		t.Errorf("paths[1] = %q, want %q (BUILDMAX_HOME override)", paths[1], wantGlobal)
	}
}

func TestSettingsPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	got := SettingsPath()
	want := filepath.Join(filepath.Clean(tmp), "settings.yaml")
	if got != want {
		t.Errorf("SettingsPath() = %q, want %q", got, want)
	}
}

func TestLoadSettings_ValidOneModel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	if err := os.WriteFile(path, []byte(settingsValidOne), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if len(s.Models) != 1 {
		t.Fatalf("len(Models) = %d, want 1", len(s.Models))
	}
	m := s.Models[0]
	if m.Model != "glm4.5" || m.Name != "My GLM" || m.APIURL != "https://api.example.com" || m.APIKey != "sk-xxx" {
		t.Errorf("first model = %+v", m)
	}
}

func TestLoadSettings_ValidTwoModels(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	if err := os.WriteFile(path, []byte(settingsValidTwo), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if len(s.Models) != 2 {
		t.Fatalf("len(Models) = %d, want 2", len(s.Models))
	}
	if s.Models[0].Model != "a" || s.Models[1].Model != "b" {
		t.Errorf("models = %+v", s.Models)
	}
}

func TestLoadSettings_MissingFileReturnsEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	if _, err := os.Stat(SettingsPath()); err == nil {
		t.Fatal("settings.yaml should not exist yet")
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() with missing file: %v", err)
	}
	if len(s.Models) != 0 {
		t.Errorf("LoadSettings() with missing file should return empty Models, got len=%d", len(s.Models))
	}
}

func TestLoadSettings_EmptyModels(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	if err := os.WriteFile(path, []byte(settingsEmptyModels), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if len(s.Models) != 0 {
		t.Errorf("len(Models) = %d, want 0", len(s.Models))
	}
}

func TestLoadSettings_InvalidYAML(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	if err := os.WriteFile(path, []byte(settingsInvalidYAML), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSettings()
	if err == nil {
		t.Error("LoadSettings(invalid YAML) should return error")
	}
}

func TestLoadSettings_UsesSettingsPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	if err := os.WriteFile(path, []byte(settingsValidOne), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings(): %v", err)
	}
	if len(s.Models) != 1 {
		t.Fatalf("LoadSettings() with file present: len(Models) = %d, want 1", len(s.Models))
	}
}

func TestEffectiveLLM_FromFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	if err := os.WriteFile(path, []byte(settingsValidOne), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, displayName := EffectiveLLM()
	if cfg.Model != "glm4.5" || cfg.BaseURL != "https://api.example.com" || cfg.APIKey != "sk-xxx" {
		t.Errorf("EffectiveLLM = %+v", cfg)
	}
	if displayName != "My GLM" {
		t.Errorf("displayName = %q, want My GLM", displayName)
	}
}

func TestEffectiveLLM_FromFile_DisplayNameFallbackToModel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	// name is empty; display name should be model id
	json := `{"models":[{"model":"x","name":"","api_url":"https://u","api_key":"k"}]}`
	if err := os.WriteFile(path, []byte(json), 0644); err != nil {
		t.Fatal(err)
	}
	_, displayName := EffectiveLLM()
	if displayName != "x" {
		t.Errorf("displayName = %q, want x", displayName)
	}
}

func TestEffectiveLLM_ErrorWhenNoSettings(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	_, _, err := EffectiveLLMWithModelName("")
	if err == nil {
		t.Error("EffectiveLLMWithModelName(\"\") should error when settings.yaml has no models")
	}
}

func TestEffectiveLLM_ErrorWhenEmptyModels(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	if err := os.WriteFile(path, []byte(settingsEmptyModels), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := EffectiveLLMWithModelName("")
	if err == nil {
		t.Error("EffectiveLLMWithModelName(\"\") should error when models list is empty")
	}
}

func TestEffectiveLLMWithModelName_EmptyModelNameSameAsEffectiveLLM(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	if err := os.WriteFile(path, []byte(settingsValidOne), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, name, err := EffectiveLLMWithModelName("")
	if err != nil {
		t.Fatalf("EffectiveLLMWithModelName(\"\") should not error: %v", err)
	}
	if cfg.Model != "glm4.5" || name != "My GLM" {
		t.Errorf("EffectiveLLMWithModelName(\"\") = %+v, name %q; want first model", cfg, name)
	}
}

func TestEffectiveLLMWithModelName_MatchByModel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	if err := os.WriteFile(path, []byte(settingsValidTwo), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, name, err := EffectiveLLMWithModelName("b")
	if err != nil {
		t.Fatalf("EffectiveLLMWithModelName(..., \"b\") err: %v", err)
	}
	if cfg.Model != "b" || cfg.BaseURL != "https://b" || cfg.APIKey != "k2" {
		t.Errorf("EffectiveLLMWithModelName(..., \"b\") = %+v", cfg)
	}
	if name != "B" {
		t.Errorf("displayName = %q, want B", name)
	}
}

func TestEffectiveLLMWithModelName_MatchByName(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	if err := os.WriteFile(path, []byte(settingsValidOne), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, name, err := EffectiveLLMWithModelName("My GLM")
	if err != nil {
		t.Fatalf("EffectiveLLMWithModelName(..., \"My GLM\") err: %v", err)
	}
	if cfg.Model != "glm4.5" {
		t.Errorf("EffectiveLLMWithModelName(..., \"My GLM\") = %+v", cfg)
	}
	if name != "My GLM" {
		t.Errorf("displayName = %q, want My GLM", name)
	}
}

func TestEffectiveLLMWithModelName_NoMatchReturnsError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	if err := os.WriteFile(path, []byte(settingsValidTwo), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := EffectiveLLMWithModelName("nonexistent")
	if err == nil {
		t.Error("EffectiveLLMWithModelName(..., \"nonexistent\") should return error")
	}
	if err != nil && !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error message should contain selector: %v", err)
	}
}

func TestEffectiveLLMWithModelName_NoModelsAndModelNameSetReturnsError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	if err := os.WriteFile(path, []byte(settingsEmptyModels), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := EffectiveLLMWithModelName("any")
	if err == nil {
		t.Error("EffectiveLLMWithModelName(empty models, \"any\") should return error")
	}
	if err != nil && !strings.Contains(err.Error(), "no models") {
		t.Errorf("error message should mention no models: %v", err)
	}
}

func TestEffectiveLLMWithModelName_MissingFileReturnsError(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	_, _, err := EffectiveLLMWithModelName("any")
	if err == nil {
		t.Error("EffectiveLLMWithModelName(\"any\") should return error when settings.yaml has no models")
	}
}

func TestPersistentWorkspaceDir(t *testing.T) {
	tmp := t.TempDir()
	got := PersistentWorkspaceDir(tmp, "ws-123")
	want := filepath.Join(filepath.Clean(tmp), "ws-123", "home")
	if got != want {
		t.Errorf("PersistentWorkspaceDir = %q, want %q", got, want)
	}
}

func TestRuntimeTaskRunHomeDir(t *testing.T) {
	tmp := t.TempDir()
	got := RuntimeTaskRunHomeDir(tmp, "ws-1", "chat-456", "run-789")
	want := filepath.Join(filepath.Clean(tmp), "ws-1", "tasks", "chat-456", "run-789", "home")
	if got != want {
		t.Errorf("RuntimeTaskRunHomeDir = %q, want %q", got, want)
	}
}

func TestRuntimeTaskRunArtifactsDir(t *testing.T) {
	tmp := t.TempDir()
	got := RuntimeTaskRunArtifactsDir(tmp, "ws-1", "chat-456", "run-789")
	want := filepath.Join(filepath.Clean(tmp), "ws-1", "tasks", "chat-456", "run-789", "artifacts")
	if got != want {
		t.Errorf("RuntimeTaskRunArtifactsDir = %q, want %q", got, want)
	}
}

func TestRuntimeTaskRunGlobalDir(t *testing.T) {
	tmp := t.TempDir()
	got := RuntimeTaskRunGlobalDir(tmp, "ws-1", "chat-456", "run-789")
	want := filepath.Join(filepath.Clean(tmp), "ws-1", "tasks", "chat-456", "run-789", "global")
	if got != want {
		t.Errorf("RuntimeTaskRunGlobalDir = %q, want %q", got, want)
	}
}

func TestLogLevel(t *testing.T) {
	if got := LogLevel("warn"); got != "warn" {
		t.Errorf("LogLevel(settings=warn) = %q, want warn", got)
	}
	if got := LogLevel(""); got != "info" {
		t.Errorf("LogLevel(empty) = %q, want info", got)
	}
}
