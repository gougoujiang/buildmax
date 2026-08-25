package llm

import "testing"

// TestProviderNeedsCredential pins which protocols a credential is demanded
// for. The exemption is what makes a local entry complete rather than
// half-written, and widening it by accident would let a hosted upstream fail at
// the first call instead of at the diagnostic that was supposed to catch it.
func TestProviderNeedsCredential(t *testing.T) {
	for _, provider := range []string{"", ProviderOpenAICompatible, ProviderOpenAI, ProviderAnthropic} {
		if !ProviderNeedsCredential(provider) {
			t.Errorf("provider %q should still need a credential", provider)
		}
	}
	if ProviderNeedsCredential(ProviderOllama) {
		t.Errorf("provider %q runs locally and has no credential to hold", ProviderOllama)
	}
}

// TestKnownProviderCoversEveryListedProvider keeps the list and the predicate
// from drifting apart, in both directions.
func TestKnownProviderCoversEveryListedProvider(t *testing.T) {
	for _, provider := range Providers() {
		if !KnownProvider(provider) {
			t.Errorf("Providers lists %q, which KnownProvider rejects", provider)
		}
	}
	if KnownProvider("bedrock") {
		t.Error("KnownProvider accepted a protocol with no adapter")
	}
	// Unlike config.KnownLLMProvider, which reads an unset value as "the
	// default", this package answers about a stated protocol.
	if KnownProvider("") {
		t.Error("KnownProvider accepted an unstated protocol")
	}
}

// TestProvidersIsStable guards the wire values themselves. They are written in
// settings.yaml, stored in the model catalog, and recorded in the call ledger,
// so renaming one silently would strand every deployment that named it.
func TestProvidersIsStable(t *testing.T) {
	want := []string{"openai_compatible", "openai", "anthropic", "ollama"}
	got := Providers()
	if len(got) != len(want) {
		t.Fatalf("Providers() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Providers()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
