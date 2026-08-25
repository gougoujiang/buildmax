package bootstrap

import (
	"github.com/gougoujiang/buildmax/internal/core/llm"
	"strings"
	"testing"

	coregw "github.com/gougoujiang/buildmax/internal/core/llmgateway"
)

// TestValidateModelInputCredentialRule pins which catalog targets may be added
// without a credential. The catalog decides where prompts go and what pays for
// them, so the exemption has to be exactly one provider wide: the one with no
// secret to hold.
func TestValidateModelInputCredentialRule(t *testing.T) {
	base := coregw.CreateModelInput{
		Name:         "Target",
		APIURL:       "https://api.example.test/v1",
		Model:        "some-model",
		ProviderType: llm.ProviderOpenAICompatible,
	}
	cases := []struct {
		name     string
		provider string
		apiKey   string
		wantErr  string
	}{
		{name: "hosted with a key", provider: llm.ProviderOpenAICompatible, apiKey: "sk-test"},
		{name: "hosted without a key", provider: llm.ProviderOpenAICompatible, wantErr: "--api-key is required"},
		{name: "anthropic without a key", provider: llm.ProviderAnthropic, wantErr: "--api-key is required"},
		{name: "local without a key", provider: llm.ProviderOllama},
		{name: "local with a key", provider: llm.ProviderOllama, apiKey: "not-needed"},
		{name: "unimplemented provider", provider: "bedrock", apiKey: "sk-test", wantErr: "not implemented"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base
			in.ProviderType = tc.provider
			in.APIKey = tc.apiKey

			err := validateModelInput(in)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateModelInput: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// TestValidateModelInputRejectsEveryWayARowCannotServe walks the branches the
// credential test does not. A row that reaches the catalog is a row a call will
// be routed to, so each of these is the last place the operator hears about it
// instead of somebody's first prompt.
//
// It is written as a characterization: the messages are what the command says
// today, and a change to any of them is a change to what an operator reads.
func TestValidateModelInputRejectsEveryWayARowCannotServe(t *testing.T) {
	valid := coregw.CreateModelInput{
		Name:         "Target",
		APIURL:       "https://api.example.test/v1",
		Model:        "some-model",
		APIKey:       "sk-test",
		ProviderType: llm.ProviderOpenAICompatible,
	}
	if err := validateModelInput(valid); err != nil {
		t.Fatalf("the valid input was refused: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(*coregw.CreateModelInput)
		wantErr string
	}{
		{"no name", func(in *coregw.CreateModelInput) { in.Name = "" }, "--name is required"},
		{"no api url", func(in *coregw.CreateModelInput) { in.APIURL = "" }, "--api-url is required"},
		{"no model", func(in *coregw.CreateModelInput) { in.Model = "" }, "--model is required"},
		{"negative context window", func(in *coregw.CreateModelInput) { in.ContextWindow = -1 }, "--context-window cannot be negative"},
		{"negative call timeout", func(in *coregw.CreateModelInput) { in.CallTimeout = -1 }, "--call-timeout cannot be negative"},
		{"negative max tokens", func(in *coregw.CreateModelInput) { in.MaxTokens = -1 }, "--max-tokens cannot be negative"},
		{"unknown reasoning", func(in *coregw.CreateModelInput) { in.Reasoning = "ultra" }, "is not a level"},
		{"unknown cache mode", func(in *coregw.CreateModelInput) { in.CacheMode = "sometimes" }, "is not a mode"},
		{"unknown cache ttl", func(in *coregw.CreateModelInput) { in.CacheTTL = "forever" }, "is not a retention"},
		{"unknown capability", func(in *coregw.CreateModelInput) { in.Capabilities = []string{"telepathy"} }, "unknown capability"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := valid
			tc.mutate(&in)
			err := validateModelInput(in)
			if err == nil {
				t.Fatalf("%s was accepted", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// TestValidateModelInputAcceptsZeroForTheOptionalNumbers pins that unset is not
// the same as invalid: a catalog row may leave the window, the timeout, and the
// token cap to the runtime's defaults.
func TestValidateModelInputAcceptsZeroForTheOptionalNumbers(t *testing.T) {
	in := coregw.CreateModelInput{
		Name:         "Target",
		APIURL:       "https://api.example.test/v1",
		Model:        "some-model",
		APIKey:       "sk-test",
		ProviderType: llm.ProviderOpenAICompatible,
	}
	if err := validateModelInput(in); err != nil {
		t.Fatalf("zero values were refused: %v", err)
	}
}
