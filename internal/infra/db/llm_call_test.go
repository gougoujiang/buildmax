package db

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/config"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/util"
)

func ptrString(s string) *string { return &s }

// ledgerTeam is the team a ledger fixture bills to. A call references a team
// row now, so the test creates one rather than naming a handle.
func ledgerTeam(t *testing.T, s *Store) string {
	t.Helper()
	return newTestTeam(t, s, newTestUser(t, s, "ledger"))
}

func sampleLLMCall() *model.LLMCall {
	return &model.LLMCall{
		TeamID:        "",
		UserID:        nil,
		Surface:       model.LLMCallSurfaceCLI,
		SessionID:     ptrString("session-1"),
		Model:         "fast",
		TargetID:      "mt_fast",
		ProviderType:  "openai_compatible",
		UpstreamModel: "vendor/fast-1",
		Streaming:     true,
		AcceptedAt:    time.Unix(1_700_000_000, 0).UTC(),
	}
}

// TestLLMCallRowRoundTrip runs without a database: it is the mapping that
// silently drops a column when the model grows.
func TestLLMCallRowRoundTrip(t *testing.T) {
	// The references are left out: they are row keys now, and resolving them
	// needs a database. The store integration tests cover that direction.
	call := sampleLLMCall()
	call.ID = ""
	call.TeamID = ""
	call.UserID = nil
	call.ClientCallID = ptrString("client-key-1")
	upstreamStarted := time.Unix(1_700_000_001, 0).UTC()
	firstDelta := time.Unix(1_700_000_002, 0).UTC()
	completed := time.Unix(1_700_000_003, 0).UTC()
	call.UpstreamStartedAt = &upstreamStarted
	call.FirstDeltaAt = &firstDelta
	call.CompletedAt = &completed
	call.Status = model.LLMCallStatusSucceeded
	call.ErrorClass = ptrString("upstream_unavailable")
	call.Attempts = 2
	prompt, completion, total := 100, 20, 120
	call.PromptTokens = &prompt
	call.CompletionTokens = &completion
	call.TotalTokens = &total
	call.UsageSource = model.LLMUsageSourceReported

	got := toLLMCall(&llmCallReadRow{Row: *llmCallValues(call)})
	if got == nil {
		t.Fatal("round trip produced nil")
	}
	if *got != *call {
		t.Errorf("round trip changed the record:\n got %+v\nwant %+v", *got, *call)
	}

	if llmCallValues(nil) != nil {
		t.Error("llmCallValues(nil) must be nil")
	}
	if toLLMCall(nil) != nil {
		t.Error("toLLMCall(nil) must be nil")
	}
}

// TestLLMCallCarriesNoContent is a structural guard for the redaction rule in
// docs/design/llm-gateway.md: the ledger records metadata, never prompts, tool
// payloads, or generated content.
func TestLLMCallCarriesNoContent(t *testing.T) {
	forbidden := []string{"prompt", "message", "content", "tool", "input", "output", "text", "body"}
	// Counts and prices, not payloads. The rate fields name the token class
	// they price — input, output — which is what trips the word list; they hold
	// nano-currency-units per million tokens and no call content.
	allowed := map[string]bool{
		"PromptTokens":          true,
		"RateInputPerMTok":      true,
		"RateOutputPerMTok":     true,
		"RateCacheReadPerMTok":  true,
		"RateCacheWritePerMTok": true,
	}

	rowType := reflect.TypeOf(llmCallRow{})
	for i := range rowType.NumField() {
		field := rowType.Field(i)
		if allowed[field.Name] {
			continue
		}
		name := strings.ToLower(field.Name)
		for _, word := range forbidden {
			if strings.Contains(name, word) {
				t.Errorf("llm_call field %q looks like it carries call content", field.Name)
			}
		}
	}
}

func TestOpenAndCompleteLLMCall(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	call := sampleLLMCall()
	call.TeamID = ledgerTeam(t, s)
	call.ClientCallID = ptrString(testPublicID(t))
	opened, err := s.OpenLLMCall(ctx, call)
	if err != nil {
		t.Fatalf("OpenLLMCall: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&llmCallRow{}, "public_id = ?", canonicalPublicID(opened.ID))
	}()

	if _, ok := util.CanonicalPublicID(opened.ID); !ok {
		t.Errorf("LLMCallID = %q, want a canonical public ID", opened.ID)
	}
	if opened.Status != model.LLMCallStatusAccepted {
		t.Errorf("Status = %q, want %q", opened.Status, model.LLMCallStatusAccepted)
	}
	if opened.UsageSource != model.LLMUsageSourceUnavailable {
		t.Errorf("UsageSource = %q, want %q", opened.UsageSource, model.LLMUsageSourceUnavailable)
	}
	if opened.PromptTokens != nil {
		t.Error("an accepted call must not carry token counts yet")
	}

	completedAt := opened.AcceptedAt.Add(3 * time.Second)
	upstreamStarted := opened.AcceptedAt.Add(1 * time.Second)
	err = s.CompleteLLMCall(ctx, opened.ID, model.LLMCallOutcome{
		Status:            model.LLMCallStatusSucceeded,
		Attempts:          1,
		UpstreamStartedAt: &upstreamStarted,
		CompletedAt:       completedAt,
		Usage: &model.LLMCallUsage{
			PromptTokens:     100,
			CompletionTokens: 20,
			TotalTokens:      120,
			Source:           model.LLMUsageSourceReported,
		},
	})
	if err != nil {
		t.Fatalf("CompleteLLMCall: %v", err)
	}

	got, err := s.GetLLMCall(ctx, opened.ID)
	if err != nil {
		t.Fatalf("GetLLMCall: %v", err)
	}
	if got == nil {
		t.Fatal("GetLLMCall returned nothing for a stored call")
	}
	if got.Status != model.LLMCallStatusSucceeded {
		t.Errorf("Status = %q, want %q", got.Status, model.LLMCallStatusSucceeded)
	}
	if got.TotalTokens == nil || *got.TotalTokens != 120 {
		t.Errorf("TotalTokens = %v, want 120", got.TotalTokens)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(completedAt) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, completedAt)
	}

	byClient, err := s.GetLLMCallByClientID(ctx, call.TeamID, *call.ClientCallID)
	if err != nil {
		t.Fatalf("GetLLMCallByClientID: %v", err)
	}
	if byClient == nil || byClient.ID != opened.ID {
		t.Errorf("GetLLMCallByClientID returned %v, want %q", byClient, opened.ID)
	}
	// Another team's identical key must not resolve this call.
	other, err := s.GetLLMCallByClientID(ctx, "tm_other", *call.ClientCallID)
	if err != nil {
		t.Fatalf("GetLLMCallByClientID for another team: %v", err)
	}
	if other != nil {
		t.Error("a client call ID resolved across teams")
	}
}

func TestCompleteLLMCallKeepsUnavailableUsage(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	call := sampleLLMCall()
	call.TeamID = ledgerTeam(t, s)
	opened, err := s.OpenLLMCall(ctx, call)
	if err != nil {
		t.Fatalf("OpenLLMCall: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&llmCallRow{}, "public_id = ?", canonicalPublicID(opened.ID))
	}()

	errorClass := "upstream_unavailable"
	err = s.CompleteLLMCall(ctx, opened.ID, model.LLMCallOutcome{
		Status:      model.LLMCallStatusFailed,
		ErrorClass:  &errorClass,
		Attempts:    3,
		CompletedAt: opened.AcceptedAt.Add(5 * time.Second),
	})
	if err != nil {
		t.Fatalf("CompleteLLMCall: %v", err)
	}

	got, err := s.GetLLMCall(ctx, opened.ID)
	if err != nil {
		t.Fatalf("GetLLMCall: %v", err)
	}
	// A failed call reports no tokens rather than zero tokens: they are
	// different facts and only one of them may be billed.
	if got.TotalTokens != nil {
		t.Errorf("TotalTokens = %v, want nil", got.TotalTokens)
	}
	if got.UsageSource != model.LLMUsageSourceUnavailable {
		t.Errorf("UsageSource = %q, want %q", got.UsageSource, model.LLMUsageSourceUnavailable)
	}
	if got.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", got.Attempts)
	}
}

// TestOpenLLMCallRejectsADuplicateClientID proves the unique index, not a
// look-before-insert, is what stops one client call ID running twice.
func TestOpenLLMCallRejectsADuplicateClientID(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	key := testPublicID(t)
	team := ledgerTeam(t, s)
	first := sampleLLMCall()
	first.TeamID = team
	first.ClientCallID = &key
	opened, err := s.OpenLLMCall(ctx, first)
	if err != nil {
		t.Fatalf("OpenLLMCall: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&llmCallRow{}, "client_call_id = ?", key)
	}()

	second := sampleLLMCall()
	second.TeamID = team
	second.ClientCallID = &key
	if _, err := s.OpenLLMCall(ctx, second); !errors.Is(err, model.ErrDuplicateLLMCall) {
		t.Fatalf("want ErrDuplicateLLMCall, got %v", err)
	}

	// Another team may use the same key: the constraint is team-scoped.
	other := sampleLLMCall()
	other.TeamID = ledgerTeam(t, s)
	other.ClientCallID = &key
	otherOpened, err := s.OpenLLMCall(ctx, other)
	if err != nil {
		t.Fatalf("another team was blocked by the key: %v", err)
	}
	if otherOpened.ID == opened.ID {
		t.Error("two calls share one id")
	}

	// A call with no client key is never a duplicate.
	for range 2 {
		anonymous := sampleLLMCall()
		anonymous.TeamID = team
		opened, err := s.OpenLLMCall(ctx, anonymous)
		if err != nil {
			t.Fatalf("a call with no client key was rejected: %v", err)
		}
		defer func() {
			_ = s.db.WithContext(ctx).Delete(&llmCallRow{}, "public_id = ?", canonicalPublicID(opened.ID))
		}()
	}
}

func TestGetLLMCallMissing(t *testing.T) {
	dsn := os.Getenv(config.EnvKeyBuildmaxTestDSN)
	if dsn == "" {
		t.Skip(config.EnvKeyBuildmaxTestDSN + " not set, skipping store integration test")
	}
	ctx := context.Background()
	s, err := New(ctx, dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := s.GetLLMCall(ctx, "lc_does_not_exist")
	if err != nil {
		t.Fatalf("GetLLMCall: %v", err)
	}
	if got != nil {
		t.Errorf("GetLLMCall = %v, want nil for a missing call", got)
	}

	// An empty key is a lookup with nothing to find, not an error.
	if got, err := s.GetLLMCallByClientID(ctx, "tm_ledger", ""); err != nil || got != nil {
		t.Errorf("GetLLMCallByClientID with an empty key = %v, %v", got, err)
	}
}
