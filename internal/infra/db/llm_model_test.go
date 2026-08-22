package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
)

const catalogSecret = "sk-CATALOG-SECRET-VALUE"

func sampleModelInput(name string) model.CreateLLMModelInput {
	return model.CreateLLMModelInput{
		Name:          name,
		ProviderType:  "openai_compatible",
		APIURL:        "https://upstream.example.com/v1",
		APIKey:        catalogSecret,
		Model:         "vendor/fast-1",
		ContextWindow: 128000,
		CallTimeout:   300,
		MaxTokens:     4096,
		Capabilities:  []string{"text_chat", "tool_calls"},
	}
}

func TestCapabilityListRoundTrip(t *testing.T) {
	tests := []struct {
		in   []string
		want []string
	}{
		{in: nil, want: nil},
		{in: []string{"text_chat"}, want: []string{"text_chat"}},
		{in: []string{"text_chat", "tool_calls"}, want: []string{"text_chat", "tool_calls"}},
		{in: []string{"  text_chat  ", "", "tool_calls"}, want: []string{"text_chat", "tool_calls"}},
	}
	for _, tc := range tests {
		got := splitCapabilities(joinCapabilities(tc.in))
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("round trip of %v = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestLLMModelReadsExcludeTheCredential is the structural guard for the rule
// that a stored key reaches the provider client and nothing else: every
// general-purpose read names its columns, and the credential is not among them.
func TestLLMModelReadsExcludeTheCredential(t *testing.T) {
	for _, column := range llmModelColumns {
		if column == "api_key" {
			t.Fatal("api_key is in the general read column list")
		}
	}
	// The entity itself has no field to put it in either.
	if strings.Contains(fmt.Sprintf("%+v", model.LLMModel{}), "APIKey") {
		t.Error("model.LLMModel has an APIKey field")
	}
}

func newCatalogStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, ctx
}

func TestCreateAndReadLLMModel(t *testing.T) {
	s, ctx := newCatalogStore(t)

	name := "Catalog " + util.NewPrefixedID("test")
	created, err := s.CreateLLMModel(ctx, sampleModelInput(name))
	if err != nil {
		t.Fatalf("CreateLLMModel: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&llmModelRow{}, "llm_model_id = ?", created.ID)
	}()

	if _, ok := util.ParsePublicID(created.ID); !ok {
		t.Errorf("LLMModelID = %q, want a canonical public ID", created.ID)
	}
	if !created.Enabled {
		t.Error("a new model is not enabled")
	}
	if strings.Contains(fmt.Sprintf("%+v", *created), catalogSecret) {
		t.Error("CreateLLMModel returned the credential")
	}

	got, err := s.GetLLMModel(ctx, created.ID)
	if err != nil || got == nil {
		t.Fatalf("GetLLMModel: %v, %v", got, err)
	}
	if got.Name != name || got.Model != "vendor/fast-1" || got.ContextWindow != 128000 {
		t.Errorf("stored model = %+v", got)
	}
	if got.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", got.MaxTokens)
	}
	if strings.Join(got.Capabilities, ",") != "text_chat,tool_calls" {
		t.Errorf("Capabilities = %v", got.Capabilities)
	}
	if strings.Contains(fmt.Sprintf("%+v", *got), catalogSecret) {
		t.Error("GetLLMModel returned the credential")
	}

	listed, err := s.ListLLMModels(ctx)
	if err != nil {
		t.Fatalf("ListLLMModels: %v", err)
	}
	if strings.Contains(fmt.Sprintf("%+v", listed), catalogSecret) {
		t.Error("ListLLMModels returned the credential")
	}

	// The one read that is allowed to see it.
	key, err := s.LLMModelCredential(ctx, created.ID)
	if err != nil {
		t.Fatalf("LLMModelCredential: %v", err)
	}
	if key != catalogSecret {
		t.Errorf("LLMModelCredential = %q", key)
	}
}

func TestLLMModelNameIsUnique(t *testing.T) {
	s, ctx := newCatalogStore(t)

	name := "Catalog " + util.NewPrefixedID("test")
	created, err := s.CreateLLMModel(ctx, sampleModelInput(name))
	if err != nil {
		t.Fatalf("CreateLLMModel: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&llmModelRow{}, "llm_model_id = ?", created.ID)
	}()

	if _, err := s.CreateLLMModel(ctx, sampleModelInput(name)); !errors.Is(err, model.ErrLLMModelNameTaken) {
		t.Fatalf("want ErrLLMModelNameTaken, got %v", err)
	}
}

func TestSetLLMModelEnabled(t *testing.T) {
	s, ctx := newCatalogStore(t)

	created, err := s.CreateLLMModel(ctx, sampleModelInput("Catalog "+util.NewPrefixedID("test")))
	if err != nil {
		t.Fatalf("CreateLLMModel: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&llmModelRow{}, "llm_model_id = ?", created.ID)
	}()

	if err := s.SetLLMModelEnabled(ctx, created.ID, false); err != nil {
		t.Fatalf("SetLLMModelEnabled: %v", err)
	}
	got, err := s.GetLLMModel(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetLLMModel: %v", err)
	}
	if got.Enabled {
		t.Error("the model is still enabled after being retired")
	}

	if err := s.SetLLMModelEnabled(ctx, "lm_does_not_exist", false); err == nil {
		t.Error("retiring a model that does not exist reported success")
	}
}

func TestGetLLMModelMissing(t *testing.T) {
	s, ctx := newCatalogStore(t)

	got, err := s.GetLLMModel(ctx, "lm_does_not_exist")
	if err != nil {
		t.Fatalf("GetLLMModel: %v", err)
	}
	if got != nil {
		t.Errorf("GetLLMModel = %v, want nil", got)
	}
	if got, err := s.GetLLMModel(ctx, ""); err != nil || got != nil {
		t.Errorf("GetLLMModel(\"\") = %v, %v", got, err)
	}
	if _, err := s.LLMModelCredential(ctx, "lm_does_not_exist"); err == nil {
		t.Error("a credential was returned for a model that does not exist")
	}
}
