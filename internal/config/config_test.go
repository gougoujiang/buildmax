package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const settingsValidOne = `{"models":[{"model":"glm4.5","name":"My GLM","api_url":"https://api.example.com","api_key":"sk-xxx"}]}`
const settingsValidTwo = `{"models":[{"model":"a","name":"","api_url":"https://a","api_key":"k1"},{"model":"b","name":"B","api_url":"https://b","api_key":"k2"}]}`
const settingsEmptyModels = `{"models":[]}`
const settingsInvalidJSON = `{invalid`

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
	want := filepath.Join(filepath.Clean(tmp), "settings.json")
	if got != want {
		t.Errorf("SettingsPath() = %q, want %q", got, want)
	}
}

func TestLoadSettings_ValidOneModel(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte(settingsValidOne), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings(path)
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
	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte(settingsValidTwo), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings(path)
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

func TestLoadSettings_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nonexistent.json")
	_, err := LoadSettings(path)
	if err == nil {
		t.Error("LoadSettings(nonexistent explicit path) should return error")
	}
	// Explicit path: file is not created (only default path gets default file).
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("LoadSettings(nonexistent explicit path) should not create file")
	}
}

func TestLoadSettings_CreatesDefaultFileWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := SettingsPath()
	if _, err := os.Stat(path); err == nil {
		t.Fatal("settings.json should not exist yet")
	}
	s, err := LoadSettings("")
	if err != nil {
		t.Fatalf("LoadSettings(\"\"): %v", err)
	}
	if s.Models != nil {
		t.Errorf("LoadSettings(empty path) with new dir should return empty Models, got len=%d", len(s.Models))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("settings.json should have been created: %v", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed != "{}" {
		t.Errorf("default settings.json content = %q, want %q", trimmed, "{}")
	}
}

func TestLoadSettings_EmptyModels(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte(settingsEmptyModels), 0644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if len(s.Models) != 0 {
		t.Errorf("len(Models) = %d, want 0", len(s.Models))
	}
}

func TestLoadSettings_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte(settingsInvalidJSON), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSettings(path)
	if err == nil {
		t.Error("LoadSettings(invalid JSON) should return error")
	}
}

func TestLoadSettings_EmptyPathUsesSettingsPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte(settingsValidOne), 0644); err != nil {
		t.Fatal(err)
	}
	// LoadSettings("") should read from SettingsPath() which is tmp/settings.json
	s, err := LoadSettings("")
	if err != nil {
		t.Fatalf("LoadSettings(\"\"): %v", err)
	}
	if len(s.Models) != 1 {
		t.Fatalf("LoadSettings(\"\") with file present: len(Models) = %d, want 1", len(s.Models))
	}
}

func TestEffectiveLLM_FromFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte(settingsValidOne), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, displayName := EffectiveLLM(path)
	if cfg.Model != "glm4.5" || cfg.BaseURL != "https://api.example.com" || cfg.APIKey != "sk-xxx" {
		t.Errorf("EffectiveLLM = %+v", cfg)
	}
	if displayName != "My GLM" {
		t.Errorf("displayName = %q, want My GLM", displayName)
	}
}

func TestEffectiveLLM_FromFile_DisplayNameFallbackToModel(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	// name is empty; display name should be model id
	json := `{"models":[{"model":"x","name":"","api_url":"https://u","api_key":"k"}]}`
	if err := os.WriteFile(path, []byte(json), 0644); err != nil {
		t.Fatal(err)
	}
	_, displayName := EffectiveLLM(path)
	if displayName != "x" {
		t.Errorf("displayName = %q, want x", displayName)
	}
}

func TestEffectiveLLM_FallbackWhenNoSettings(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxHome, tmp)
	t.Setenv(EnvKeyBuildmaxAPIKey, "env-key")
	t.Setenv(EnvKeyBuildmaxBaseURL, "https://env.url")
	t.Setenv(EnvKeyBuildmaxModel, "env-model")
	// Use default path (empty string): auto-creates empty settings.json, then falls back to env.
	cfg, displayName := EffectiveLLM("")
	if cfg.APIKey != "env-key" || cfg.BaseURL != "https://env.url" || cfg.Model != "env-model" {
		t.Errorf("EffectiveLLM(\"\") should fall back to LoadLLM(), got %+v", cfg)
	}
	if displayName != "env-model" {
		t.Errorf("displayName = %q, want env-model", displayName)
	}
}

func TestEffectiveLLM_FallbackWhenEmptyModels(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte(settingsEmptyModels), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvKeyBuildmaxAPIKey, "fallback-key")
	t.Setenv(EnvKeyBuildmaxBaseURL, "https://fallback.url")
	t.Setenv(EnvKeyBuildmaxModel, "fallback-model")
	cfg, displayName := EffectiveLLM(path)
	if cfg.APIKey != "fallback-key" || cfg.Model != "fallback-model" {
		t.Errorf("EffectiveLLM(empty models) should fall back to LoadLLM(), got %+v", cfg)
	}
	if displayName != "fallback-model" {
		t.Errorf("displayName = %q, want fallback-model", displayName)
	}
}

func TestEffectiveLLMWithSelector_EmptySelectorSameAsEffectiveLLM(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte(settingsValidOne), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, name, err := EffectiveLLMWithSelector(path, "")
	if err != nil {
		t.Fatalf("EffectiveLLMWithSelector(path, \"\") should not error: %v", err)
	}
	if cfg.Model != "glm4.5" || name != "My GLM" {
		t.Errorf("EffectiveLLMWithSelector(path, \"\") = %+v, name %q; want first model", cfg, name)
	}
}

func TestEffectiveLLMWithSelector_MatchByModel(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte(settingsValidTwo), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, name, err := EffectiveLLMWithSelector(path, "b")
	if err != nil {
		t.Fatalf("EffectiveLLMWithSelector(..., \"b\") err: %v", err)
	}
	if cfg.Model != "b" || cfg.BaseURL != "https://b" || cfg.APIKey != "k2" {
		t.Errorf("EffectiveLLMWithSelector(..., \"b\") = %+v", cfg)
	}
	if name != "B" {
		t.Errorf("displayName = %q, want B", name)
	}
}

func TestEffectiveLLMWithSelector_MatchByName(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte(settingsValidOne), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, name, err := EffectiveLLMWithSelector(path, "My GLM")
	if err != nil {
		t.Fatalf("EffectiveLLMWithSelector(..., \"My GLM\") err: %v", err)
	}
	if cfg.Model != "glm4.5" {
		t.Errorf("EffectiveLLMWithSelector(..., \"My GLM\") = %+v", cfg)
	}
	if name != "My GLM" {
		t.Errorf("displayName = %q, want My GLM", name)
	}
}

func TestEffectiveLLMWithSelector_NoMatchReturnsError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte(settingsValidTwo), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := EffectiveLLMWithSelector(path, "nonexistent")
	if err == nil {
		t.Error("EffectiveLLMWithSelector(..., \"nonexistent\") should return error")
	}
	if err != nil && !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error message should contain selector: %v", err)
	}
}

func TestEffectiveLLMWithSelector_NoModelsAndSelectorSetReturnsError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "settings.json")
	if err := os.WriteFile(path, []byte(settingsEmptyModels), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := EffectiveLLMWithSelector(path, "any")
	if err == nil {
		t.Error("EffectiveLLMWithSelector(empty models, \"any\") should return error")
	}
	if err != nil && !strings.Contains(err.Error(), "no models in settings") {
		t.Errorf("error message should mention no models: %v", err)
	}
}

func TestEffectiveLLMWithSelector_MissingFileAndSelectorSetReturnsError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nonexistent.json")
	_, _, err := EffectiveLLMWithSelector(path, "any")
	if err == nil {
		t.Error("EffectiveLLMWithSelector(missing file, \"any\") should return error")
	}
}

func TestWorkspacesDir_Default(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxWorkspacesDir, "")
	dir := WorkspacesDir()
	if dir != "" {
		t.Errorf("WorkspacesDir() = %q, want %q", dir, "")
	}
}

func TestWorkspacesDir_Override(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxWorkspacesDir, tmp)
	dir := WorkspacesDir()
	want := filepath.Clean(tmp)
	if dir != want {
		t.Errorf("WorkspacesDir() = %q, want %q", dir, want)
	}
}

func TestPersistentWorkspaceDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxWorkspacesDir, tmp)
	got := PersistentWorkspaceDir("ws-123")
	want := filepath.Join(filepath.Clean(tmp), "ws-123", "home")
	if got != want {
		t.Errorf("PersistentWorkspaceDir(\"ws-123\") = %q, want %q", got, want)
	}
}

func TestRuntimeWorkspaceDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxWorkspacesDir, tmp)
	got := RuntimeWorkspaceDir("ws-1", "chat-456")
	want := filepath.Join(filepath.Clean(tmp), "ws-1", "tasks", "chat-456")
	if got != want {
		t.Errorf("RuntimeWorkspaceDir(\"ws-1\", \"chat-456\") = %q, want %q", got, want)
	}
}

func TestRuntimeChatBuildmaxDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxWorkspacesDir, tmp)
	got := RuntimeChatBuildmaxDir("ws-1", "chat-456")
	want := filepath.Join(filepath.Clean(tmp), "ws-1", "tasks", "chat-456", "buildmax")
	if got != want {
		t.Errorf("RuntimeChatBuildmaxDir(\"ws-1\", \"chat-456\") = %q, want %q", got, want)
	}
}

func TestRuntimeChatWSDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxWorkspacesDir, tmp)
	got := RuntimeChatWSDir("ws-1", "chat-456")
	want := filepath.Join(filepath.Clean(tmp), "ws-1", "tasks", "chat-456", "ws")
	if got != want {
		t.Errorf("RuntimeChatWSDir(\"ws-1\", \"chat-456\") = %q, want %q", got, want)
	}
}

func TestRuntimeTaskRunHomeDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxWorkspacesDir, tmp)
	got := RuntimeTaskRunHomeDir("ws-1", "chat-456", "run-789")
	want := filepath.Join(filepath.Clean(tmp), "ws-1", "tasks", "chat-456", "run-789", "home")
	if got != want {
		t.Errorf("RuntimeTaskRunHomeDir(...) = %q, want %q", got, want)
	}
}

func TestRuntimeTaskRunArtifactsDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxWorkspacesDir, tmp)
	got := RuntimeTaskRunArtifactsDir("ws-1", "chat-456", "run-789")
	want := filepath.Join(filepath.Clean(tmp), "ws-1", "tasks", "chat-456", "run-789", "artifacts")
	if got != want {
		t.Errorf("RuntimeTaskRunArtifactsDir(...) = %q, want %q", got, want)
	}
}

func TestRuntimeTaskRunGlobalDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxWorkspacesDir, tmp)
	got := RuntimeTaskRunGlobalDir("ws-1", "chat-456", "run-789")
	want := filepath.Join(filepath.Clean(tmp), "ws-1", "tasks", "chat-456", "run-789", "global")
	if got != want {
		t.Errorf("RuntimeTaskRunGlobalDir(...) = %q, want %q", got, want)
	}
}

func TestRunOutputDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvKeyBuildmaxWorkspacesDir, tmp)
	got := RunOutputDir("ws-1", "chat-456", "run-1")
	want := filepath.Join(filepath.Clean(tmp), "ws-1", "artifacts", "chat-456", "run-1")
	if got != want {
		t.Errorf("RunOutputDir(\"ws-1\", \"chat-456\", \"run-1\") = %q, want %q", got, want)
	}
}

func TestResolveServerPort(t *testing.T) {
	tests := []struct {
		name         string
		portFromFlag int
		env          string // BUILDMAX_SERVER_PORT, empty means unset
		wantPort     int
		wantErr      bool
	}{
		{"flag overrides env", 9999, "8888", 9999, false},
		{"env when flag zero", 0, "8888", 8888, false},
		{"default when no flag and no env", 0, "", DefaultServerPort, false},
		{"invalid env", 0, "bad", 0, true},
		{"env zero is invalid", 0, "0", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv(EnvKeyBuildmaxServerPort, tt.env)
			} else {
				t.Setenv(EnvKeyBuildmaxServerPort, "")
			}
			port, err := ResolveServerPort(tt.portFromFlag)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveServerPort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && port != tt.wantPort {
				t.Errorf("ResolveServerPort() = %d, want %d", port, tt.wantPort)
			}
		})
	}
}

func TestLogLevel(t *testing.T) {
	t.Setenv(EnvKeyBuildmaxLogLevel, "debug")
	if got := LogLevel(); got != "debug" {
		t.Errorf("LogLevel() = %q, want debug", got)
	}
	t.Setenv(EnvKeyBuildmaxLogLevel, "")
	if got := LogLevel(); got != "" {
		t.Errorf("LogLevel() empty = %q", got)
	}
}
