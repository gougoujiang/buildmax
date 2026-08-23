package bootstrap

import (
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

// TestValidateModelInputCredentialRule pins which catalog targets may be added
// without a credential. The catalog decides where prompts go and what pays for
// them, so the exemption has to be exactly one provider wide: the one with no
// secret to hold.
func TestValidateModelInputCredentialRule(t *testing.T) {
	base := model.CreateLLMModelInput{
		Name:         "Target",
		APIURL:       "https://api.example.test/v1",
		Model:        "some-model",
		ProviderType: llmgateway.ProviderOpenAICompatible,
	}
	cases := []struct {
		name     string
		provider string
		apiKey   string
		wantErr  string
	}{
		{name: "hosted with a key", provider: llmgateway.ProviderOpenAICompatible, apiKey: "sk-test"},
		{name: "hosted without a key", provider: llmgateway.ProviderOpenAICompatible, wantErr: "--api-key is required"},
		{name: "anthropic without a key", provider: llmgateway.ProviderAnthropic, wantErr: "--api-key is required"},
		{name: "local without a key", provider: llmgateway.ProviderOllama},
		{name: "local with a key", provider: llmgateway.ProviderOllama, apiKey: "not-needed"},
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
