package llm

// Provider types name the wire protocol an upstream speaks.
//
// A provider type selects a client implementation; it is not a vendor name.
// Claude reached through an OpenAI-compatible gateway is
// ProviderOpenAICompatible, and Claude reached at Anthropic's own endpoint is
// ProviderAnthropic.
//
// This is the one definition. A configured model entry, a catalog target, and a
// recorded call all name a protocol, and an operator must read the same word in
// settings.yaml, in the catalog, and in the call ledger. It lives here because
// a protocol name is a contract, not something config loads or the gateway
// resolves.
const (
	// ProviderOpenAICompatible is OpenAI Chat Completions, spoken by OpenRouter,
	// LiteLLM, vLLM, and local inference servers. It is the default.
	ProviderOpenAICompatible = "openai_compatible"
	// ProviderOpenAI is OpenAI's own Responses API.
	ProviderOpenAI = "openai"
	// ProviderAnthropic is the Anthropic Messages API.
	ProviderAnthropic = "anthropic"
	// ProviderOllama is Ollama's own /api/chat, spoken by a local daemon. Its
	// compatibility endpoint would answer ProviderOpenAICompatible, but that
	// path cannot set the context window the runtime otherwise defaults and
	// silently truncates to.
	ProviderOllama = "ollama"
)

// Providers returns every implemented wire protocol, for help text and error
// messages that must not drift from the list above.
func Providers() []string {
	return []string{ProviderOpenAICompatible, ProviderOpenAI, ProviderAnthropic, ProviderOllama}
}

// KnownProvider reports whether name is a provider type BuildMax implements.
// An empty name is not known: a caller that has not stated a protocol has not
// finished describing its upstream. Defaulting an unset one is the
// configuration boundary's job, not this package's.
func KnownProvider(name string) bool {
	switch name {
	case ProviderOpenAICompatible, ProviderOpenAI, ProviderAnthropic, ProviderOllama:
		return true
	}
	return false
}

// ProviderNeedsCredential reports whether an upstream speaking this protocol
// must carry a secret.
//
// Every hosted protocol does, and one without a credential is a
// misconfiguration that must fail at the first call rather than send an
// unauthenticated request. A local runtime has none: what authorizes the call
// is being able to reach the daemon at all, which is a property of the
// deployment's network. Demanding a placeholder for it would turn a working
// setup into a diagnostic failure.
func ProviderNeedsCredential(name string) bool {
	return name != ProviderOllama
}
