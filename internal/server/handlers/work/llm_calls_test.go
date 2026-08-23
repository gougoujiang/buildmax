package work

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

func ptr[T any](v T) *T { return &v }

func llmCallsFixture(ledger *llmStubLedger) Config {
	return Config{
		JWTSecret: llmTestSecret,
		Teams:     llmTestTeamStore(),
		TaskRuns: &mock.MockTaskRunStore{
			Runs:     []model.TaskRun{{ID: "r_1", TaskID: "t_1", Status: string(model.RunStatusSucceeded), CreatedAt: time.Unix(1, 0).UTC()}},
			TaskList: []model.Task{{ID: "t_1", ConversationID: "c_1", TeamID: llmTestTeam, CreatedBy: llmTestUser, CreatedAt: time.Unix(1, 0).UTC()}},
		},
		Conversations: &mock.MockConversationStore{
			Conversations: []model.Conversation{{ID: "c_1", TeamID: llmTestTeam, CreatedBy: llmTestUser}},
		},
		LLMCalls: ledger,
	}
}

func getLLMCalls(t *testing.T, cfg Config, teamID, taskRunID string, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	New(cfg).Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/task-runs/"+taskRunID+"/llm-calls", nil)
	if auth {
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(llmTestUser, llmTestSecret))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func stagedCall() model.LLMCall {
	return model.LLMCall{
		ID:            "lc_1",
		UserID:        ptr(llmTestUser),
		TaskRunID:     ptr("r_1"),
		TaskID:        ptr("t_1"),
		Surface:       "worker",
		Model:         "default",
		TargetID:      "SECRET-CATALOG-ID",
		ProviderType:  "openai_compatible",
		UpstreamModel: "SECRET-UPSTREAM-MODEL",
		AcceptedAt:    time.Now().UTC(),
		Status:        model.LLMCallStatusSucceeded,
		PromptTokens:  ptr(10),
		UsageSource:   model.LLMUsageSourceReported,
	}
}

// TestListTaskRunLLMCalls is the reachability this route exists for: what a run
// spent was recorded from the start and could only be read with the database
// password.
func TestListTaskRunLLMCalls(t *testing.T) {
	rec := getLLMCalls(t, llmCallsFixture(&llmStubLedger{calls: []model.LLMCall{stagedCall()}}), llmTestTeam, "r_1", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var out []LLMCallSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("returned %d calls, want 1", len(out))
	}
	call := out[0]
	if call.ID != "lc_1" || call.Model != "default" || call.Surface != "worker" {
		t.Errorf("call = %+v", call)
	}
	if call.UserID == nil || *call.UserID != llmTestUser {
		t.Errorf("user = %v, want the run's owner", call.UserID)
	}
	if call.PromptTokens == nil || *call.PromptTokens != 10 {
		t.Errorf("prompt tokens = %v", call.PromptTokens)
	}
}

// The ledger recorded cache counts from the start and this view dropped them,
// which left a cache-heavy run looking identical to an uncached one to everyone
// without the database password. A reported zero must survive as a zero, not as
// an absent field: "the provider cached nothing" and "the provider said
// nothing" are different facts about what a run cost.
func TestListTaskRunLLMCallsReportsCacheTokens(t *testing.T) {
	call := stagedCall()
	call.CacheReadTokens = ptr(80)
	call.CacheWriteTokens = ptr(0)
	rec := getLLMCalls(t, llmCallsFixture(&llmStubLedger{calls: []model.LLMCall{call}}), llmTestTeam, "r_1", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var out []LLMCallSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("returned %d calls, want 1", len(out))
	}
	got := out[0]
	if got.CacheReadTokens == nil || *got.CacheReadTokens != 80 {
		t.Errorf("cache read = %v, want 80", got.CacheReadTokens)
	}
	if got.CacheWriteTokens == nil || *got.CacheWriteTokens != 0 {
		t.Errorf("cache write = %v, want a reported 0", got.CacheWriteTokens)
	}
	if !strings.Contains(rec.Body.String(), `"cache_write_tokens":0`) {
		t.Errorf("a reported zero was omitted rather than sent: %s", rec.Body)
	}
}

// A call the provider reported no cache for omits the fields entirely, so a
// reader is never handed a zero nobody measured.
func TestListTaskRunLLMCallsOmitsUnreportedCacheTokens(t *testing.T) {
	rec := getLLMCalls(t, llmCallsFixture(&llmStubLedger{calls: []model.LLMCall{stagedCall()}}), llmTestTeam, "r_1", true)
	for _, field := range []string{"cache_read_tokens", "cache_write_tokens"} {
		if strings.Contains(rec.Body.String(), field) {
			t.Errorf("unreported %s was sent as a count: %s", field, rec.Body)
		}
	}
}

// TestListTaskRunLLMCallsHidesOperatorRouting keeps the reader inside the team
// boundary. A caller names a model; which catalog entry that resolves to, and
// which upstream model it is, belong to the operator.
func TestListTaskRunLLMCallsHidesOperatorRouting(t *testing.T) {
	rec := getLLMCalls(t, llmCallsFixture(&llmStubLedger{calls: []model.LLMCall{stagedCall()}}), llmTestTeam, "r_1", true)
	for _, secret := range []string{"SECRET-CATALOG-ID", "SECRET-UPSTREAM-MODEL"} {
		if body := rec.Body.String(); strings.Contains(body, secret) {
			t.Errorf("response leaked %q: %s", secret, body)
		}
	}
}

func TestListTaskRunLLMCallsRefusals(t *testing.T) {
	staged := &llmStubLedger{calls: []model.LLMCall{stagedCall()}}

	t.Run("unauthenticated", func(t *testing.T) {
		if code := getLLMCalls(t, llmCallsFixture(staged), llmTestTeam, "r_1", false).Code; code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", code)
		}
	})

	// A member of one team must not enumerate another team's spending by naming
	// a run id, which is why the run's ownership is checked before the query.
	t.Run("a team the caller does not belong to", func(t *testing.T) {
		code := getLLMCalls(t, llmCallsFixture(staged), "tm_other", "r_1", true).Code
		if code == http.StatusOK {
			t.Error("a foreign team's run returned a ledger")
		}
	})

	t.Run("a run the server does not have", func(t *testing.T) {
		code := getLLMCalls(t, llmCallsFixture(staged), llmTestTeam, "r_missing", true).Code
		if code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", code)
		}
	})

	t.Run("no ledger configured", func(t *testing.T) {
		cfg := llmCallsFixture(staged)
		cfg.LLMCalls = nil
		if code := getLLMCalls(t, cfg, llmTestTeam, "r_1", true).Code; code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", code)
		}
	})

	t.Run("the query fails", func(t *testing.T) {
		cfg := llmCallsFixture(&llmStubLedger{listErr: errors.New("database is away")})
		if code := getLLMCalls(t, cfg, llmTestTeam, "r_1", true).Code; code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", code)
		}
	})
}

// TestListTaskRunLLMCallsEmptyIsAList records that a direct-mode run answers
// with an empty array rather than null: it made no managed calls, which is a
// fact, not a missing answer.
func TestListTaskRunLLMCallsEmptyIsAList(t *testing.T) {
	rec := getLLMCalls(t, llmCallsFixture(&llmStubLedger{}), llmTestTeam, "r_1", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if body := rec.Body.String(); body != "[]\n" && body != "[]" {
		t.Errorf("body = %q, want an empty array", body)
	}
}

// A ledger row is priced from the rates it recorded, not from whatever the
// catalog says now. That is the whole reason the rates are on the row: a model
// gets repriced, and recomputing an old call from the new rates would restate
// an invoice that has already been paid.
func TestListTaskRunLLMCallsPricesFromTheRecordedRates(t *testing.T) {
	call := stagedCall()
	call.PromptTokens = ptr(100_000)
	call.CompletionTokens = ptr(1_000)
	call.CacheReadTokens = ptr(90_000)
	call.CacheWriteTokens = ptr(0)
	call.Currency = "USD"
	call.RateInputPerMTok = ptr(int64(3_000_000_000))
	call.RateCacheReadPerMTok = ptr(int64(300_000_000))
	call.RateCacheWritePerMTok = ptr(int64(3_750_000_000))
	call.RateOutputPerMTok = ptr(int64(15_000_000_000))

	out := listCalls(t, call)
	cost := out[0].Cost
	if cost == nil {
		t.Fatal("a priced row produced no cost")
	}
	// 10k fresh at $3/M + 90k read at $0.30/M + 1k out at $15/M.
	if cost.Currency != "USD" || cost.Uncached != 30_000_000 ||
		cost.CacheRead != 27_000_000 || cost.Output != 15_000_000 {
		t.Errorf("cost = %+v", *cost)
	}
	if cost.Total != cost.Uncached+cost.CacheRead+cost.CacheWrite+cost.Output {
		t.Errorf("total %d does not match its parts: %+v", cost.Total, *cost)
	}
	// Uncached the same 100k prompt would have billed at $3/M throughout.
	if cost.Baseline != 300_000_000+15_000_000 {
		t.Errorf("baseline = %d", cost.Baseline)
	}
	if cost.Baseline <= cost.Total {
		t.Error("a read-heavy call should cost less than the same call uncached")
	}
}

// Two absences that must not become a zero cost: a model nobody priced, and a
// row written before the rate snapshot existed. Both are unknowns, and a zero
// would read as a free call.
func TestListTaskRunLLMCallsOmitsCostWhenItCannotBeKnown(t *testing.T) {
	unpriced := stagedCall()
	unpriced.PromptTokens = ptr(100_000)

	priced := stagedCall()
	priced.Currency = "USD"
	priced.RateInputPerMTok = ptr(int64(3_000_000_000))
	// No usage reported, so there is nothing to multiply.
	priced.PromptTokens = nil

	for name, call := range map[string]model.LLMCall{"unpriced model": unpriced, "unreported usage": priced} {
		t.Run(name, func(t *testing.T) {
			out := listCalls(t, call)
			if out[0].Cost != nil {
				t.Errorf("cost = %+v, want absent", *out[0].Cost)
			}
			if strings.Contains(rawBody(t, call), `"cost"`) {
				t.Error("an absent cost was serialized")
			}
		})
	}
}

func listCalls(t *testing.T, call model.LLMCall) []LLMCallSummary {
	t.Helper()
	rec := getLLMCalls(t, llmCallsFixture(&llmStubLedger{calls: []model.LLMCall{call}}), llmTestTeam, "r_1", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	var out []LLMCallSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("returned %d calls, want 1", len(out))
	}
	return out
}

func rawBody(t *testing.T, call model.LLMCall) string {
	t.Helper()
	rec := getLLMCalls(t, llmCallsFixture(&llmStubLedger{calls: []model.LLMCall{call}}), llmTestTeam, "r_1", true)
	return rec.Body.String()
}
