package llm

// knownContextWindows maps provider/model identifiers (as used by OpenRouter and similar
// OpenAI-compatible providers) to their context window size in tokens.
// The settings.yaml context_window field takes precedence; this table is the fallback
// when the user has not configured context_window explicitly.
//
// Values are top_provider.context_length from OpenRouter's /models catalog, not the
// model-level context_length: the two can differ (the routed provider sometimes caps
// lower), and top_provider is what a call actually gets. `./make models check` diffs a
// settings.yaml entry against that same field, so a value copied from its output — or
// from `./make models info <model>` — lands here already in the right unit.
//
// OpenAI and Anthropic get their full current fast/mid/premium ladder, not just one
// entry: an agent runtime meant to be broadly compatible needs to have actually run
// against the cheap tier and the premium tier of its two largest providers, not just
// whichever one the last contributor happened to configure.
var knownContextWindows = map[string]int{
	// Anthropic — current Claude 5 generation, fast to premium.
	"anthropic/claude-haiku-4.5": 200_000,
	"anthropic/claude-sonnet-5":  1_000_000,
	"anthropic/claude-opus-5":    1_000_000,
	"anthropic/claude-3-haiku":   200_000, // oldest id still live; some configs still pin it

	// OpenAI — current GPT-5.6 generation, fast to premium.
	"openai/gpt-5.6-luna":  1_050_000,
	"openai/gpt-5.6-sol":   1_050_000,
	"openai/gpt-5.6-terra": 1_050_000,
	// Still live on OpenRouter; kept for configs pinned to a specific older id.
	"openai/gpt-4o":        128_000,
	"openai/gpt-4o-mini":   128_000,
	"openai/gpt-4-turbo":   128_000,
	"openai/gpt-4":         8_191,
	"openai/gpt-3.5-turbo": 16_385,
	"openai/o1":            200_000,
	"openai/o3-mini":       200_000,

	// Google
	"google/gemini-3.7-flash":      1_048_576,
	"google/gemini-3.1-flash-lite": 1_048_576,
	"google/gemini-2.5-flash-lite": 1_048_576,

	// xAI
	"x-ai/grok-4.6": 500_000,

	// DeepSeek
	"deepseek/deepseek-v4-flash": 1_024_000,
	"deepseek/deepseek-chat":     128_000,
	"deepseek/deepseek-r1":       64_000,

	// Z.ai (GLM)
	"z-ai/glm-5.3": 1_048_576,

	// Moonshot AI (Kimi)
	"moonshotai/kimi-k3": 1_048_576,

	// Qwen (Alibaba)
	"qwen/qwen3-max": 262_144,

	// Meta / Llama (commonly hosted on OpenRouter)
	"meta-llama/llama-3.3-70b-instruct": 131_072,
	"meta-llama/llama-3.1-70b-instruct": 131_072,
	"meta-llama/llama-3.1-8b-instruct":  131_072,

	// Mistral
	"mistralai/mistral-large-2512": 262_144,
	"mistralai/codestral-2508":     256_000,
}

// lookupContextWindow returns the known context window for the given model identifier,
// or 0 if the model is not in the built-in table.
func lookupContextWindow(model string) int {
	return knownContextWindows[model]
}
