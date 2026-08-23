// Package config provides configuration loading and defaults.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	corehook "github.com/gougoujiang/buildmax/internal/core/hook"
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
	LogLevel  string          `mapstructure:"log_level"`
	ServerURL string          `mapstructure:"server_url"`
	Models    []ModelEntry    `mapstructure:"models"`
	Hooks     corehook.Config `mapstructure:"hooks"`
	Sandbox   SandboxConfig   `mapstructure:"sandbox"`
	Tools     ToolsConfig     `mapstructure:"tools"`
	Agent     AgentConfig     `mapstructure:"agent"`
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
	// MaxTokens caps one response; 0 = uses the adapter's own default. The
	// Anthropic Messages protocol requires the field, so its adapter substitutes
	// DefaultMaxTokens; the OpenAI protocols send it only when set.
	MaxTokens int `mapstructure:"max_tokens"`
	// Reasoning is how much the model should reason before answering:
	// ReasoningOff (the default), ReasoningLow, ReasoningMedium, or
	// ReasoningHigh. Any level other than off also replays the reasoning on
	// later turns. It has no effect on a protocol that carries none.
	Reasoning string `mapstructure:"reasoning"`
	// Vision says this model accepts image input. When false, an image a tool
	// returns is described in text rather than sent, because a model that
	// cannot read images rejects the request rather than ignoring the image.
	Vision bool `mapstructure:"vision"`
	// PromptCache is the deprecated shorthand for CacheControl. It is a pointer
	// because absent and false ask for different things: absent means nobody
	// chose and takes the default, while false is an opt-out. Use CacheControl
	// in new configuration; ResolveCacheControl folds the two.
	PromptCache *bool `mapstructure:"prompt_cache"`
	// CacheControl is this model's prompt-cache policy: which calls ask the
	// provider to cache the stable prefix of a request — the tool definitions
	// and system prompt — and for how long.
	CacheControl *CacheControl `mapstructure:"cache_control"`
	// KeepAlive is how long a local runtime keeps the model loaded after a
	// call — a duration string, "0" to unload at once, "-1" to stay resident.
	// Only LLMProviderOllama reads it; on a hosted provider there is no model
	// to keep loaded. Empty means the runtime's own default.
	KeepAlive string `mapstructure:"keep_alive"`
	// Provider is the wire protocol a direct entry speaks: LLMProviderOpenAICompatible
	// (the default), LLMProviderOpenAI, LLMProviderAnthropic, or LLMProviderOllama.
	// It is ignored by a "buildmax" entry, where the operator's catalog decides.
	Provider string `mapstructure:"provider"`
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

// LLM wire protocols a direct model entry can speak. The value names a protocol
// family, not a vendor: Claude served through an OpenAI-compatible gateway is
// LLMProviderOpenAICompatible, and Claude served from Anthropic's own endpoint is
// LLMProviderAnthropic.
const (
	// LLMProviderOpenAICompatible is OpenAI Chat Completions, spoken by OpenRouter,
	// LiteLLM, vLLM, and local inference servers. It is the default.
	LLMProviderOpenAICompatible = "openai_compatible"
	// LLMProviderOpenAI is OpenAI's own Responses API.
	LLMProviderOpenAI = "openai"
	// LLMProviderAnthropic is the Anthropic Messages API.
	LLMProviderAnthropic = "anthropic"
	// LLMProviderOllama is Ollama's own /api/chat, spoken by a local daemon.
	// Its compatibility endpoint would answer LLMProviderOpenAICompatible, but
	// that path cannot set the context window the runtime otherwise defaults
	// and silently truncates to.
	LLMProviderOllama = "ollama"
)

// LLMProviders returns every wire protocol a direct model entry may name, for
// help text and error messages that must not drift from the list above.
func LLMProviders() []string {
	return []string{
		LLMProviderOpenAICompatible,
		LLMProviderOpenAI,
		LLMProviderAnthropic,
		LLMProviderOllama,
	}
}

// LLMProviderNeedsAPIKey reports whether an entry naming this provider must
// carry a credential. A local runtime has none, and demanding a placeholder for
// it turns a working setup into a diagnostic failure.
func LLMProviderNeedsAPIKey(provider string) bool {
	return provider != LLMProviderOllama
}

// LLMProvider returns the wire protocol this entry speaks, defaulting to
// LLMProviderOpenAICompatible so a configuration written before providers
// existed keeps calling what it always called.
func (m ModelEntry) LLMProvider() string {
	if m.Provider == "" {
		return LLMProviderOpenAICompatible
	}
	return m.Provider
}

// Reasoning effort levels. They are a neutral scale: each protocol maps them to
// its own vocabulary, and a level a model does not support fails that model's
// call rather than being silently downgraded.
const (
	// ReasoningOff does no extra reasoning. It is the default.
	ReasoningOff    = "off"
	ReasoningLow    = "low"
	ReasoningMedium = "medium"
	ReasoningHigh   = "high"
)

// ReasoningEnabled reports whether a configured level asks for reasoning.
func ReasoningEnabled(level string) bool {
	return level != "" && level != ReasoningOff
}

// KnownReasoningEffort reports whether level is one BuildMax implements. The
// empty string is known: it means off.
func KnownReasoningEffort(level string) bool {
	switch level {
	case "", ReasoningOff, ReasoningLow, ReasoningMedium, ReasoningHigh:
		return true
	}
	return false
}

// ReasoningEfforts returns every level an operator may set, for help text and
// error messages that must not drift from the list above.
func ReasoningEfforts() []string {
	return []string{ReasoningOff, ReasoningLow, ReasoningMedium, ReasoningHigh}
}

// KnownLLMProvider reports whether name is a wire protocol BuildMax implements.
// The empty string is known: it means the default.
func KnownLLMProvider(name string) bool {
	switch name {
	case "", LLMProviderOpenAICompatible, LLMProviderOpenAI, LLMProviderAnthropic, LLMProviderOllama:
		return true
	}
	return false
}

// DefaultOpenRouterBaseURL is the OpenRouter OpenAI-compatible API base URL.
const DefaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

// DefaultOllamaBaseURL is where a local Ollama daemon listens. It is the daemon
// root, not the /v1 compatibility endpoint, because LLMProviderOllama speaks the
// native API under /api.
const DefaultOllamaBaseURL = "http://localhost:11434"

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
	// A test that reaches the real data directory reads and writes the
	// contributor's own sessions, settings, and credentials — and its result
	// depends on what happens to be in there. `./make test` sets BUILDMAX_HOME,
	// so the invariant held there and nowhere else: a bare `go test ./internal/x`,
	// the narrow inner loop the testing guide asks for, fell straight through.
	if testing.Testing() {
		panic("config.DataDir: BUILDMAX_HOME is unset under `go test`, which would use the real ~/.buildmax. " +
			"Give the package a TestMain calling testsupport.RunWithIsolatedHome, or set it per test with " +
			"t.Setenv(config.EnvKeyBuildmaxHome, t.TempDir()).")
	}
	return defaultDataDir()
}

// defaultDataDir is where DataDir lands with no override. It is separate so the
// test that pins the default path can assert it without tripping the guard.
func defaultDataDir() string {
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
