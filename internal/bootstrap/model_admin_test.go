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
