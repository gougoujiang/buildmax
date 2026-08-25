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
	coregw "github.com/gougoujiang/buildmax/internal/core/llmgateway"
	"github.com/gougoujiang/buildmax/internal/util"
)

func ptrString(s string) *string { return &s }

// ledgerUser is the person a ledger fixture attributes to. A call references a
// user row, so the test creates one rather than naming a handle.
func ledgerUser(t *testing.T, s *Store) string {
	t.Helper()
	return newTestUser(t, s, "ledger")
}

func sampleLLMCall() *coregw.Call {
	return &coregw.Call{
		UserID:        nil,
		Surface:       coregw.CallSurfaceCLI,
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
	call.UserID = nil
	call.ClientCallID = ptrString("client-key-1")
	upstreamStarted := time.Unix(1_700_000_001, 0).UTC()
	firstDelta := time.Unix(1_700_000_002, 0).UTC()
	completed := time.Unix(1_700_000_003, 0).UTC()
	call.UpstreamStartedAt = &upstreamStarted
	call.FirstDeltaAt = &firstDelta
	call.CompletedAt = &completed
	call.Status = coregw.CallStatusSucceeded
	call.ErrorClass = ptrString("upstream_unavailable")
	call.Attempts = 2
	prompt, completion, total := 100, 20, 120
	call.PromptTokens = &prompt
	call.CompletionTokens = &completion
	call.TotalTokens = &total
	call.UsageSource = coregw.UsageSourceReported

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
	call.UserID = ptrString(ledgerUser(t, s))
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
	if opened.Status != coregw.CallStatusAccepted {
		t.Errorf("Status = %q, want %q", opened.Status, coregw.CallStatusAccepted)
	}
	if opened.UsageSource != coregw.UsageSourceUnavailable {
		t.Errorf("UsageSource = %q, want %q", opened.UsageSource, coregw.UsageSourceUnavailable)
	}
	if opened.PromptTokens != nil {
		t.Error("an accepted call must not carry token counts yet")
	}

	completedAt := opened.AcceptedAt.Add(3 * time.Second)
	upstreamStarted := opened.AcceptedAt.Add(1 * time.Second)
	err = s.CompleteLLMCall(ctx, opened.ID, coregw.CallOutcome{
		Status:            coregw.CallStatusSucceeded,
		Attempts:          1,
		UpstreamStartedAt: &upstreamStarted,
		CompletedAt:       completedAt,
		Usage: &coregw.CallUsage{
			PromptTokens:     100,
			CompletionTokens: 20,
			TotalTokens:      120,
			Source:           coregw.UsageSourceReported,
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
	if got.Status != coregw.CallStatusSucceeded {
		t.Errorf("Status = %q, want %q", got.Status, coregw.CallStatusSucceeded)
	}
	if got.TotalTokens == nil || *got.TotalTokens != 120 {
		t.Errorf("TotalTokens = %v, want 120", got.TotalTokens)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(completedAt) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, completedAt)
	}

	byClient, err := s.GetLLMCallByClientID(ctx, *call.UserID, *call.ClientCallID)
	if err != nil {
		t.Fatalf("GetLLMCallByClientID: %v", err)
	}
	if byClient == nil || byClient.ID != opened.ID {
		t.Errorf("GetLLMCallByClientID returned %v, want %q", byClient, opened.ID)
	}
	// Another person's identical key must not resolve this call.
	other, err := s.GetLLMCallByClientID(ctx, ledgerUser(t, s), *call.ClientCallID)
	if err != nil {
		t.Fatalf("GetLLMCallByClientID for another user: %v", err)
	}
	if other != nil {
		t.Error("a client call ID resolved across users")
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
	call.UserID = ptrString(ledgerUser(t, s))
	opened, err := s.OpenLLMCall(ctx, call)
	if err != nil {
		t.Fatalf("OpenLLMCall: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&llmCallRow{}, "public_id = ?", canonicalPublicID(opened.ID))
	}()

	errorClass := "upstream_unavailable"
	err = s.CompleteLLMCall(ctx, opened.ID, coregw.CallOutcome{
		Status:      coregw.CallStatusFailed,
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
	if got.UsageSource != coregw.UsageSourceUnavailable {
		t.Errorf("UsageSource = %q, want %q", got.UsageSource, coregw.UsageSourceUnavailable)
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
	user := ledgerUser(t, s)
	first := sampleLLMCall()
	first.UserID = &user
	first.ClientCallID = &key
	opened, err := s.OpenLLMCall(ctx, first)
	if err != nil {
		t.Fatalf("OpenLLMCall: %v", err)
	}
	defer func() {
		_ = s.db.WithContext(ctx).Delete(&llmCallRow{}, "client_call_id = ?", key)
	}()

	second := sampleLLMCall()
	second.UserID = &user
	second.ClientCallID = &key
	if _, err := s.OpenLLMCall(ctx, second); !errors.Is(err, coregw.ErrDuplicateCall) {
		t.Fatalf("want ErrDuplicateLLMCall, got %v", err)
	}

	// Another person may use the same key: the constraint is user-scoped.
	otherUser := ledgerUser(t, s)
	other := sampleLLMCall()
	other.UserID = &otherUser
	other.ClientCallID = &key
	otherOpened, err := s.OpenLLMCall(ctx, other)
	if err != nil {
		t.Fatalf("another user was blocked by the key: %v", err)
	}
	if otherOpened.ID == opened.ID {
		t.Error("two calls share one id")
	}

	// A call with no client key is never a duplicate.
	for range 2 {
		anonymous := sampleLLMCall()
		anonymous.UserID = &user
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
