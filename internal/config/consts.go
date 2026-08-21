package config

// DefaultContextWindow is the fallback context window token limit used when a model
// entry does not specify context_window (i.e. the value is 0).
// 32 000 tokens is a conservative default that fits most hosted models.
const DefaultContextWindow = 32_000

// DefaultCallTimeoutSecs is the per-LLM-call timeout used when a model entry does not
// specify call_timeout. 300 seconds (5 minutes) is generous enough for long reasoning
// responses while still bounding hung connections.
const DefaultCallTimeoutSecs = 300

// DefaultMaxTokens caps one response when a model entry does not set max_tokens
// and the protocol requires the field. The Anthropic Messages API rejects a
// request without it, so its adapter substitutes this value; the OpenAI
// protocols leave the cap to the provider when it is unset.
// 8 192 tokens fits a long tool-calling turn without truncating mid-thought.
const DefaultMaxTokens = 8_192
