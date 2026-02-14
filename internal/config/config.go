// Package config provides configuration loading and defaults.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LLM holds LLM provider settings (OpenRouter/OpenAI-compatible).
type LLM struct {
	APIKey  string
	BaseURL string
	Model   string
}

// Settings is the root structure for settings.json (e.g. under DataDir).
type Settings struct {
	Models []ModelEntry `json:"models"`
}

// ModelEntry is one LLM model entry in settings (snake_case on disk).
type ModelEntry struct {
	Model  string `json:"model"`
	Name   string `json:"name"`
	APIURL string `json:"api_url"`
	APIKey string `json:"api_key"`
}

// DefaultOpenRouterBaseURL is the OpenRouter OpenAI-compatible API base URL.
const DefaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

// Model names on OpenRouter (switch DefaultModel to use one).
const (
	ModelGPT35Turbo      = "openai/gpt-3.5-turbo" // NOT FREE
	ModelGemma327bItFree = "google/gemma-3-27b-it:free"
	ModelGLM45AirFree    = "z-ai/glm-4.5-air:free"
)

// DefaultModel is the model used when BUILDMAX_MODEL is not set.
const DefaultModel = ModelGLM45AirFree

// LoadLLM loads LLM config from environment.
// OPENROUTER_API_KEY or BUILDMAX_API_KEY for API key;
// BUILDMAX_BASE_URL for base URL (defaults to OpenRouter);
// BUILDMAX_MODEL for model (defaults to openai/gpt-3.5-turbo).
func LoadLLM() LLM {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("BUILDMAX_API_KEY")
	}
	baseURL := os.Getenv("BUILDMAX_BASE_URL")
	if baseURL == "" {
		baseURL = DefaultOpenRouterBaseURL
	}
	model := os.Getenv("BUILDMAX_MODEL")
	if model == "" {
		model = DefaultModel
	}
	return LLM{APIKey: apiKey, BaseURL: baseURL, Model: model}
}

// DataDir returns the application data folder path.
// If BUILDMAX_HOME is set (non-empty), returns filepath.Clean(os.Getenv("BUILDMAX_HOME")).
// Otherwise returns filepath.Join(os.UserHomeDir(), ".buildmax").
// Does not create the directory; callers must create it if needed.
func DataDir() string {
	if dir := os.Getenv("BUILDMAX_HOME"); dir != "" {
		return filepath.Clean(dir)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".buildmax")
}

// SessionsDir returns the path to the sessions directory under DataDir.
// Does not create the directory; callers must create it if needed.
func SessionsDir() string {
	return filepath.Join(DataDir(), "sessions")
}

// LogsDir returns the path to the logs directory under DataDir.
// Does not create the directory; callers must create it if needed.
func LogsDir() string {
	return filepath.Join(DataDir(), "logs")
}

// SettingsPath returns the path to the settings file under DataDir.
func SettingsPath() string {
	return filepath.Join(DataDir(), "settings.json")
}

// LoadSettings reads the settings file at path. If path is empty, SettingsPath() is used.
// On missing file (default path only): creates DataDir and the file with content "{}", then returns empty Settings.
// On other read error or invalid JSON, returns empty Settings (no error).
func LoadSettings(path string) Settings {
	if path == "" {
		path = SettingsPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		// When using the default path, create settings.json with {} if it does not exist.
		if path == SettingsPath() && os.IsNotExist(err) {
			_ = os.MkdirAll(DataDir(), 0755)
			_ = os.WriteFile(path, []byte("{}\n"), 0644)
		}
		return Settings{}
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}
	}
	return s
}

// EffectiveLLMWithSelector returns the LLM config and display name to use at startup,
// optionally selecting a model by id or name. settingsPath is passed to LoadSettings
// (empty means default SettingsPath()). When modelSelector is empty, behaviour equals
// EffectiveLLM: first model from settings or LoadLLM() fallback; no error. When
// modelSelector is non-empty, settings must have at least one model and an entry
// whose model or name equals modelSelector (first match wins); otherwise an error
// is returned.
func EffectiveLLMWithSelector(settingsPath string, modelSelector string) (LLM, string, error) {
	if settingsPath == "" {
		settingsPath = SettingsPath()
	}
	s := LoadSettings(settingsPath)
	if modelSelector == "" {
		if len(s.Models) == 0 {
			cfg := LoadLLM()
			return cfg, cfg.Model, nil
		}
		m := s.Models[0]
		displayName := m.Name
		if displayName == "" {
			displayName = m.Model
		}
		return LLM{
			APIKey:  m.APIKey,
			BaseURL: m.APIURL,
			Model:   m.Model,
		}, displayName, nil
	}
	if len(s.Models) == 0 {
		return LLM{}, "", errors.New("no models in settings; add settings or omit --model")
	}
	for _, m := range s.Models {
		if m.Model == modelSelector || m.Name == modelSelector {
			displayName := m.Name
			if displayName == "" {
				displayName = m.Model
			}
			return LLM{
				APIKey:  m.APIKey,
				BaseURL: m.APIURL,
				Model:   m.Model,
			}, displayName, nil
		}
	}
	return LLM{}, "", fmt.Errorf("model not found: %q", modelSelector)
}

// EffectiveLLM returns the LLM config and display name to use at startup.
// If path is empty, the default settings path is used. When the settings file
// has at least one model, the first entry is used; otherwise LoadLLM() is used.
// Display name is the entry's name if set, else the model id.
func EffectiveLLM(path string) (LLM, string) {
	cfg, name, _ := EffectiveLLMWithSelector(path, "")
	return cfg, name
}

// SkillSearchPaths returns the ordered list of directories to scan for skills.
// Priority (first wins on name conflict):
//  1. <workspace>/.buildmax/skills  (project-level)
//  2. <DataDir>/skills              (global-level, e.g. ~/.buildmax/skills)
//
// Missing directories are not created; callers handle absent dirs gracefully.
func SkillSearchPaths(workspace string) []string {
	return []string{
		filepath.Join(workspace, ".buildmax", "skills"),
		filepath.Join(DataDir(), "skills"),
	}
}

// AgentDefsSearchPaths returns the ordered list of directories to scan for agent definitions.
// Priority (first wins on name conflict):
//  1. <workspace>/.buildmax/agents  (project-level)
//  2. <DataDir>/agents              (global-level, e.g. ~/.buildmax/agents)
//
// Missing directories are not created; callers handle absent dirs gracefully.
func AgentDefsSearchPaths(workspace string) []string {
	return []string{
		filepath.Join(workspace, ".buildmax", "agents"),
		filepath.Join(DataDir(), "agents"),
	}
}
