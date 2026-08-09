package llm

// knownContextWindows maps provider/model identifiers (as used by OpenRouter and similar
// OpenAI-compatible providers) to their context window size in tokens.
// The settings.yaml context_window field takes precedence; this table is the fallback
// when the user has not configured context_window explicitly.
var knownContextWindows = map[string]int{
	// Anthropic (via OpenRouter or direct)
	"anthropic/claude-opus-4-7":            200_000,
	"anthropic/claude-opus-4-5":            200_000,
	"anthropic/claude-sonnet-4-6":          200_000,
	"anthropic/claude-sonnet-4-5":          200_000,
	"anthropic/claude-haiku-4-5":           200_000,
	"anthropic/claude-3-5-sonnet":          200_000,
	"anthropic/claude-3-5-sonnet-20241022": 200_000,
	"anthropic/claude-3-5-haiku":           200_000,
	"anthropic/claude-3-5-haiku-20241022":  200_000,
	"anthropic/claude-3-opus":              200_000,
	"anthropic/claude-3-sonnet":            200_000,
	"anthropic/claude-3-haiku":             200_000,

	// OpenAI
	"openai/gpt-4o":        128_000,
	"openai/gpt-4o-mini":   128_000,
	"openai/gpt-4-turbo":   128_000,
	"openai/gpt-4":         8_192,
	"openai/gpt-3.5-turbo": 16_385,
	"openai/o1":            200_000,
	"openai/o1-mini":       128_000,
	"openai/o3-mini":       200_000,

	// Google
	"google/gemini-2.0-flash":      1_048_576,
	"google/gemini-2.0-flash-lite": 1_048_576,
	"google/gemini-2.5-flash-lite": 1_048_576,
	"google/gemini-3.1-flash-lite": 1_048_576,
	"google/gemini-1.5-pro":        2_097_152,
	"google/gemini-1.5-flash":      1_048_576,
	"google/gemma-3-27b-it:free":   8_192,

	// Meta / Llama (commonly hosted on OpenRouter)
	"meta-llama/llama-3.3-70b-instruct": 128_000,
	"meta-llama/llama-3.1-70b-instruct": 128_000,
	"meta-llama/llama-3.1-8b-instruct":  128_000,
	"meta-llama/llama-3-70b-instruct":   8_192,
	"meta-llama/llama-3-8b-instruct":    8_192,

	// Mistral
	"mistralai/mistral-large":       128_000,
	"mistralai/mistral-small":       128_000,
	"mistralai/codestral-mamba":     256_000,
	"mistralai/mistral-7b-instruct": 32_000,

	// DeepSeek
	"deepseek/deepseek-chat":  65_536,
	"deepseek/deepseek-coder": 65_536,
	"deepseek/deepseek-r1":    163_840,
}

// lookupContextWindow returns the known context window for the given model identifier,
// or 0 if the model is not in the built-in table.
func lookupContextWindow(model string) int {
	return knownContextWindows[model]
}
