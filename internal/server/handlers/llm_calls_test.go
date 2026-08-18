package handlers

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
	"github.com/gougoujiang/buildmax/internal/util"
)

func ptr[T any](v T) *T { return &v }

func llmCallsFixture(ledger *llmStubLedger) Config {
	return Config{
		JWTSecret: llmTestSecret,
		TeamStore: llmTestTeamStore(),
		TaskRunStore: &mock.MockTaskRunStore{
			Runs:     []model.TaskRun{{TaskRunID: "r_1", TaskID: "t_1", Status: string(model.RunStatusSucceeded), CreatedAt: 1}},
			TaskList: []model.Task{{TaskID: "t_1", ConversationID: "c_1", TeamID: llmTestTeam, CreatedBy: llmTestUser, CreatedAt: 1}},
		},
		ConversationStore: &mock.MockConversationStore{
			Conversations: []model.Conversation{{ConversationID: "c_1", TeamID: llmTestTeam, CreatedBy: llmTestUser}},
		},
		LLMCallStore: ledger,
	}
}

func getLLMCalls(t *testing.T, cfg Config, teamID, taskRunID string, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	NewHandler(cfg).Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/teams/"+teamID+"/task-runs/"+taskRunID+"/llm-calls", nil)
	if auth {
		req.Header.Set("Authorization", "Bearer "+util.SignJWT(llmTestUser, llmTestSecret))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func stagedCall() model.LLMCall {
	return model.LLMCall{
		LLMCallID:     "lc_1",
		TeamID:        llmTestTeam,
		UserID:        ptr(llmTestUser),
		TaskRunID:     ptr("r_1"),
		TaskID:        ptr("t_1"),
		Surface:       "worker",
		Alias:         "default",
		TargetID:      "SECRET-CATALOG-ID",
		ProviderType:  "openai_compatible",
		UpstreamModel: "SECRET-UPSTREAM-MODEL",
		AcceptedAt:    time.Now().Unix(),
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
	if call.LLMCallID != "lc_1" || call.Alias != "default" || call.Surface != "worker" {
		t.Errorf("call = %+v", call)
	}
	if call.UserID == nil || *call.UserID != llmTestUser {
		t.Errorf("user = %v, want the run's owner", call.UserID)
	}
	if call.PromptTokens == nil || *call.PromptTokens != 10 {
		t.Errorf("prompt tokens = %v", call.PromptTokens)
	}
}

// TestListTaskRunLLMCallsHidesOperatorRouting keeps the reader inside the team
// boundary. A team is granted aliases; which catalog entry an alias resolves to,
// and which upstream model that is, belong to the operator.
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
		cfg.LLMCallStore = nil
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
