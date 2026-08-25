package bootstrap

import (
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/llm"
	coregw "github.com/gougoujiang/buildmax/internal/core/llmgateway"
	"github.com/gougoujiang/buildmax/internal/service/llmcatalog"
)

// TestCommandErrorNamesTheFlag is the operator's half of the catalog's
// refusals. The service says which input field it refused, in its own
// vocabulary; this is where that becomes the flag the person typed.
//
// It is pinned here because the message is what an operator reads and acts on:
// "api_key is required" sends them looking for a field that has no flag.
func TestCommandErrorNamesTheFlag(t *testing.T) {
	cases := []struct {
		field   string
		message string
		want    string
	}{
		{"name", "is required", "model add: --name is required"},
		{"api_url", "is required", "model add: --api-url is required"},
		{"api_key", "is required", "model add: --api-key is required"},
		{"context_window", "cannot be negative", "model add: --context-window cannot be negative"},
		{"provider_type", `"bedrock" is not implemented`, `model add: --provider "bedrock" is not implemented`},
		// A refusal about no single field prints on its own.
		{"", `unknown capability "telepathy"`, `model add: unknown capability "telepathy"`},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			err := commandError("model add", &llmcatalog.InvalidField{Field: tc.field, Message: tc.message})
			if got := err.Error(); got != tc.want {
				t.Errorf("error = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCommandErrorPassesThroughWhatItCannotRender keeps a failure that is not a
// refusal readable rather than reshaped into one.
func TestCommandErrorPassesThroughWhatItCannotRender(t *testing.T) {
	err := commandError("model add", errors.New("database is gone"))
	if got := err.Error(); got != "model add: database is gone" {
		t.Errorf("error = %q", got)
	}
}

// TestModelAddFlagForCoversEveryFieldValidateNames fails when the catalog
// starts refusing a field this command cannot name. Without it the operator
// gets the service's vocabulary and has to guess the flag.
func TestModelAddFlagForCoversEveryFieldValidateNames(t *testing.T) {
	base := coregw.CreateModelInput{
		Name:         "Target",
		APIURL:       "https://api.example.test/v1",
		Model:        "some-model",
		APIKey:       "sk-test",
		ProviderType: llm.ProviderOpenAICompatible,
	}
	mutations := []func(*coregw.CreateModelInput){
		func(in *coregw.CreateModelInput) { in.Name = "" },
		func(in *coregw.CreateModelInput) { in.APIURL = "" },
		func(in *coregw.CreateModelInput) { in.APIKey = "" },
		func(in *coregw.CreateModelInput) { in.Model = "" },
		func(in *coregw.CreateModelInput) { in.ContextWindow = -1 },
		func(in *coregw.CreateModelInput) { in.CallTimeout = -1 },
		func(in *coregw.CreateModelInput) { in.MaxTokens = -1 },
		func(in *coregw.CreateModelInput) { in.Reasoning = "ultra" },
		func(in *coregw.CreateModelInput) { in.ProviderType = "bedrock" },
		func(in *coregw.CreateModelInput) { in.CacheMode = "sometimes" },
		func(in *coregw.CreateModelInput) { in.CacheTTL = "forever" },
	}
	for _, mutate := range mutations {
		in := base
		mutate(&in)
		err := llmcatalog.Validate(in)
		if err == nil {
			t.Fatal("a mutation this test relies on was accepted")
		}
		var invalid *llmcatalog.InvalidField
		if !errors.As(err, &invalid) || invalid.Field == "" {
			continue
		}
		if _, ok := modelAddFlagFor[invalid.Field]; !ok {
			t.Errorf("Validate refuses field %q, which model add cannot name as a flag", invalid.Field)
		}
	}
}
