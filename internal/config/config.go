// Package config provides configuration loading and defaults.
package config

import (
	"os"
	"path/filepath"
)

// LLM holds LLM provider settings (OpenRouter/OpenAI-compatible).
type LLM struct {
	APIKey  string
	BaseURL string
	Model   string
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
// If HOME_DIR is set (non-empty), returns filepath.Clean(os.Getenv("HOME_DIR")).
// Otherwise returns filepath.Join(os.UserHomeDir(), ".buildmax").
// Does not create the directory; callers must create it if needed.
func DataDir() string {
	if dir := os.Getenv("HOME_DIR"); dir != "" {
		return filepath.Clean(dir)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".buildmax")
}

// SkillSearchPaths returns the ordered list of directories to scan for skills.
// Priority (first wins on name conflict):
//  1. <workspace>/.buildmax/skills  (project-level)
//  2. <workspace>/.cursor/skills    (backward compatibility with Cursor layouts)
//  3. <DataDir>/skills              (global-level, e.g. ~/.buildmax/skills)
//
// Missing directories are not created; callers handle absent dirs gracefully.
func SkillSearchPaths(workspace string) []string {
	return []string{
		filepath.Join(workspace, ".buildmax", "skills"),
		filepath.Join(workspace, ".cursor", "skills"),
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
