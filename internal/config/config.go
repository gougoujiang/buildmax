// Package config provides configuration loading and defaults.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// LLM holds resolved LLM provider settings.
type LLM struct {
	APIKey  string
	BaseURL string
	Model   string
}

// Settings is the root structure for settings.yaml (BUILDMAX_HOME/settings.yaml).
// Used by the CLI and desktop app.
type Settings struct {
	LogLevel  string        `mapstructure:"log_level"`
	ServerURL string        `mapstructure:"server_url"`
	Models    []ModelEntry  `mapstructure:"models"`
	Hooks     HooksConfig   `mapstructure:"hooks"`
	Sandbox   SandboxConfig `mapstructure:"sandbox"`
}

// LLM connection modes for a model entry.
//
// The two are never mixed and never fall back to one another: a run either
// calls a provider from this machine or calls a BuildMax deployment, and the
// user can tell which from the entry. See docs/design/llm-gateway.md.
const (
	// TransportDirect calls an OpenAI-compatible provider from this machine
	// using the entry's own credential. It is the default.
	TransportDirect = "direct"
	// TransportBuildMax calls a BuildMax server's managed gateway, which holds
	// the provider credential and decides which model the team may use.
	TransportBuildMax = "buildmax"
)

// ModelEntry is one LLM model entry in settings.yaml (snake_case on disk).
type ModelEntry struct {
	Model         string `mapstructure:"model"`
	Name          string `mapstructure:"name"`
	APIURL        string `mapstructure:"api_url"`
	APIKey        string `mapstructure:"api_key"`
	ContextWindow int    `mapstructure:"context_window"` // 0 = uses DefaultContextWindow
	CallTimeout   int    `mapstructure:"call_timeout"`   // seconds; 0 = uses DefaultCallTimeoutSecs
	// Transport is "direct" (the default) or "buildmax".
	Transport string `mapstructure:"transport"`
	// ServerURL and TeamID apply to a "buildmax" entry. The credential is not
	// copied here: the remote client reads it from the login state in
	// auth.json, and only when it belongs to this server.
	ServerURL string `mapstructure:"server_url"`
	TeamID    string `mapstructure:"team_id"`
}

// IsManaged reports whether this entry calls a BuildMax gateway.
func (m ModelEntry) IsManaged() bool { return m.Transport == TransportBuildMax }

// DefaultOpenRouterBaseURL is the OpenRouter OpenAI-compatible API base URL.
const DefaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

// Model names on OpenRouter (switch DefaultModel to use one).
const (
	ModelGPT35Turbo        = "openai/gpt-3.5-turbo"
	ModelGemma327bItFree   = "google/gemma-3-27b-it:free"
	ModelGLM45AirFree      = "z-ai/glm-4.5-air:free"
	ModelGPT4oMini         = "openai/gpt-4o-mini"
	ModelGemini25FlashLite = "google/gemini-2.5-flash-lite"
	ModelGemini31FlashLite = "google/gemini-3.1-flash-lite"
)

// DefaultModel is the model used when no model is configured.
const DefaultModel = ModelGemini25FlashLite

// LogLevel returns the effective log level from the config file, defaulting to "info".
func LogLevel(fromSettings string) string {
	if fromSettings != "" {
		return fromSettings
	}
	return "info"
}

// ---------------------------------------------------------------------------
// Paths (BUILDMAX_HOME-relative)
// ---------------------------------------------------------------------------

// DataDir returns the application data folder path.
// BUILDMAX_HOME overrides; default is ~/.buildmax.
// Does not create the directory; callers must create it if needed.
func DataDir() string {
	if dir := os.Getenv(EnvKeyBuildmaxHome); dir != "" {
		return filepath.Clean(dir)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".buildmax")
}

// SettingsPath returns the path to the local settings file (BUILDMAX_HOME/settings.yaml).
func SettingsPath() string {
	return filepath.Join(DataDir(), "settings.yaml")
}

// SessionsDir returns the path to the sessions directory under DataDir.
func SessionsDir() string {
	return filepath.Join(DataDir(), "sessions")
}

// LogsDir returns the path to the logs directory under DataDir.
func LogsDir() string {
	return filepath.Join(DataDir(), "logs")
}

// AuthPath returns the path to the auth credentials file under DataDir.
func AuthPath() string {
	return filepath.Join(DataDir(), "auth.json")
}

// SkillSearchPaths returns the ordered list of directories to scan for skills.
// Priority: workspace-local first, then global DataDir.
func SkillSearchPaths(workspace string) []string {
	return []string{
		filepath.Join(workspace, ".buildmax", "skills"),
		filepath.Join(DataDir(), "skills"),
	}
}

// AgentDefsSearchPaths returns the ordered list of directories to scan for agent definitions.
// Priority: workspace-local first, then global DataDir.
func AgentDefsSearchPaths(workspace string) []string {
	return []string{
		filepath.Join(workspace, ".buildmax", "agents"),
		filepath.Join(DataDir(), "agents"),
	}
}

// ---------------------------------------------------------------------------
// Workspace path helpers — take workspacesDir explicitly so callers are
// not coupled to a global env var.
// ---------------------------------------------------------------------------

// PersistentWorkspaceDir returns the persistent home directory for a team's workspace.
func PersistentWorkspaceDir(workspacesDir, workspaceID string) string {
	return filepath.Join(workspacesDir, workspaceID, "home")
}

// RuntimeTaskRunDir returns the run directory for a specific task run.
func RuntimeTaskRunDir(workspacesDir, workspaceID, taskID, taskRunID string) string {
	return filepath.Join(workspacesDir, workspaceID, "tasks", taskID, taskRunID)
}

// RuntimeTaskRunHomeDir returns the run's home dir (materialized workspace home).
func RuntimeTaskRunHomeDir(workspacesDir, workspaceID, taskID, taskRunID string) string {
	return filepath.Join(RuntimeTaskRunDir(workspacesDir, workspaceID, taskID, taskRunID), "home")
}

// RuntimeTaskRunArtifactsDir returns the run's artifacts dir.
func RuntimeTaskRunArtifactsDir(workspacesDir, workspaceID, taskID, taskRunID string) string {
	return filepath.Join(RuntimeTaskRunDir(workspacesDir, workspaceID, taskID, taskRunID), "artifacts")
}

// RuntimeTaskRunGlobalDir returns the run's global dir (BUILDMAX_HOME for that run).
func RuntimeTaskRunGlobalDir(workspacesDir, workspaceID, taskID, taskRunID string) string {
	return filepath.Join(RuntimeTaskRunDir(workspacesDir, workspaceID, taskID, taskRunID), "global")
}

// ---------------------------------------------------------------------------
// Settings loader
// ---------------------------------------------------------------------------

// LoadSettings reads BUILDMAX_HOME/settings.yaml via Viper.
// A missing file is not an error — returns (Settings{}, nil) so callers fall back gracefully.
func LoadSettings() (Settings, error) {
	v := viper.New()
	v.SetConfigFile(SettingsPath())
	if err := v.ReadInConfig(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Settings{}, nil
		}
		return Settings{}, fmt.Errorf("read settings: %w", err)
	}
	var s Settings
	if err := v.Unmarshal(&s); err != nil {
		return Settings{}, fmt.Errorf("parse settings: %w", err)
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// Effective LLM helpers (used by agentapp)
// ---------------------------------------------------------------------------

// EffectiveLLMWithModelName returns the LLM config and display name to use at startup.
// When modelName is empty: uses the first model from settings.yaml.
// When modelName is set: must match an entry by name or model id.
// Returns an error when settings.yaml has no models configured.
func EffectiveLLMWithModelName(modelName string) (LLM, string, error) {
	s, err := LoadSettings()
	if err != nil {
		return LLM{}, "", fmt.Errorf("load settings: %w", err)
	}
	if len(s.Models) == 0 {
		return LLM{}, "", errors.New("no models configured; add models to settings.yaml")
	}
	if modelName == "" {
		m := s.Models[0]
		displayName := m.Name
		if displayName == "" {
			displayName = m.Model
		}
		return LLM{APIKey: m.APIKey, BaseURL: m.APIURL, Model: m.Model}, displayName, nil
	}
	for _, m := range s.Models {
		if m.Model == modelName || m.Name == modelName {
			displayName := m.Name
			if displayName == "" {
				displayName = m.Model
			}
			return LLM{APIKey: m.APIKey, BaseURL: m.APIURL, Model: m.Model}, displayName, nil
		}
	}
	return LLM{}, "", fmt.Errorf("model not found: %q", modelName)
}

// EffectiveLLM returns the LLM config and display name using the first model from settings
// or the env-var fallback.
func EffectiveLLM() (LLM, string) {
	cfg, name, _ := EffectiveLLMWithModelName("")
	return cfg, name
}
