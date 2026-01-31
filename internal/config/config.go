// Package config provides configuration loading and defaults.
package config

import "os"

// LLM holds LLM provider settings (OpenRouter/OpenAI-compatible).
type LLM struct {
	APIKey  string
	BaseURL string
	Model   string
}

// DefaultOpenRouterBaseURL is the OpenRouter OpenAI-compatible API base URL.
const DefaultOpenRouterBaseURL = "https://openrouter.ai/api/v1"

// DefaultModel is a common default model on OpenRouter.
const DefaultModel = "openai/gpt-3.5-turbo"

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
