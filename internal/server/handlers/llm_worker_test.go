package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/server/authtoken"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

const workerTestToken = "worker-token-for-tests"

// workerRunClaims is what the scheduler would have minted for the run the tests
// below execute.
func workerRunClaims() authtoken.RunClaims {
	return authtoken.RunClaims{
		UserID:    llmTestUser,
		TeamID:    llmTestTeam,
		TaskRunID: "r_1",
		TaskID:    "t_1",
	}
}

func workerRunToken(t *testing.T, claims authtoken.RunClaims, ttl time.Duration, issuedAt time.Time) string {
	t.Helper()
	token, err := authtoken.MintRun(llmTestSecret, claims, ttl, issuedAt)
	if err != nil {
		t.Fatalf("MintRun: %v", err)
	}
	return token
}

func validWorkerRunToken(t *testing.T) string {
	t.Helper()
	return workerRunToken(t, workerRunClaims(), time.Hour, time.Now())
}

func workerLLMHandler(gateway *llmgateway.Service, runStatus string) http.Handler {
	h := NewHandler(Config{
		JWTSecret:  llmTestSecret,
		TeamStore:  llmTestTeamStore(),
		LLMGateway: gateway,
		TaskRunStore: &mock.MockTaskRunStore{
			Runs:     []model.TaskRun{{TaskRunID: "r_1", TaskID: "t_1", Status: runStatus, CreatedAt: 1}},
			TaskList: []model.Task{{TaskID: "t_1", ConversationID: "c_1", TeamID: llmTestTeam, Status: runStatus, Input: "in", CreatedBy: llmTestUser, CreatedAt: 1}},
		},
		WorkerToken: workerTestToken,
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

// workerLLMRequest issues a worker-route completion for a run in the given
// status, authenticated with token.
func workerLLMRequest(t *testing.T, gateway *llmgateway.Service, runStatus, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/worker/task-runs/r_1/llm/completions", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	workerLLMHandler(gateway, runStatus).ServeHTTP(rec, req)
	return rec
}

const workerLLMBody = `{"model":"default","messages":[{"role":"user","content":"hi"}]}`

func TestWorkerLLMCompletions(t *testing.T) {
	gateway := llmTestService(t, &llmStubClient{content: "answer"}, nil)

	t.Run("answers a run that is executing", func(t *testing.T) {
		rec := workerLLMRequest(t, gateway, string(model.RunStatusRunning), workerLLMBody, validWorkerRunToken(t))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "answer") {
			t.Errorf("body %q should carry the completion", rec.Body.String())
		}
	})

	// A run token proves which run is calling, but not that the run is still
	// going. Without the status check a token would keep spending a team's quota
	// against work that finished, right up to its expiry.
	t.Run("refuses a run that is not executing", func(t *testing.T) {
		for _, status := range []model.RunStatus{
			model.RunStatusPending,
			model.RunStatusScheduled,
			model.RunStatusSucceeded,
			model.RunStatusFailed,
		} {
			rec := workerLLMRequest(t, gateway, string(status), workerLLMBody, validWorkerRunToken(t))
			if rec.Code != http.StatusConflict {
				t.Errorf("status %s: got %d, want 409", status, rec.Code)
			}
		}
	})

	// The shared worker token opens the rest of /api/worker/*. It must not open
	// this route: it names no user, no team, and no run, so a call authenticated
	// with it could not be attributed or bounded.
	t.Run("refuses the shared worker token", func(t *testing.T) {
		rec := workerLLMRequest(t, gateway, string(model.RunStatusRunning), workerLLMBody, workerTestToken)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("refuses a token it cannot verify", func(t *testing.T) {
		tests := []struct {
			name  string
			token string
		}{
			{"no credential at all", ""},
			{"not a token", "not-a-token"},
			{"expired", workerRunToken(t, workerRunClaims(), time.Minute, time.Now().Add(-time.Hour))},
			{"signed by another deployment", func() string {
				other, err := authtoken.MintRun("some-other-secret", workerRunClaims(), time.Hour, time.Now())
				if err != nil {
					t.Fatalf("MintRun: %v", err)
				}
				return other
			}()},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				rec := workerLLMRequest(t, gateway, string(model.RunStatusRunning), workerLLMBody, tc.token)
				if rec.Code != http.StatusUnauthorized {
					t.Errorf("status = %d, want 401", rec.Code)
				}
			})
		}
	})

	// The check that makes the credential per-run rather than per-worker. A
	// compromised run holding its own valid token must not be able to bill
	// another run by pointing at that run's URL.
	t.Run("refuses a token minted for another run", func(t *testing.T) {
		claims := workerRunClaims()
		claims.TaskRunID = "r_somebody_else"
		rec := workerLLMRequest(t, gateway, string(model.RunStatusRunning), workerLLMBody,
			workerRunToken(t, claims, time.Hour, time.Now()))
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})

	// Token and server state must agree. They can only diverge if a run changed
	// hands after dispatch, and the safe reading of that is refusal.
	t.Run("refuses a token whose team is not the run's", func(t *testing.T) {
		claims := workerRunClaims()
		claims.TeamID = "tm_other"
		rec := workerLLMRequest(t, gateway, string(model.RunStatusRunning), workerLLMBody,
			workerRunToken(t, claims, time.Hour, time.Now()))
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})

	// A task run belongs to whoever created it. A ledger that recorded only the
	// team could not answer whose work spent the tokens.
	t.Run("attributes the call to the run's user and team", func(t *testing.T) {
		attributed := llmTestService(t, &llmStubClient{content: "answer"}, nil)
		rec := workerLLMRequest(t, attributed, string(model.RunStatusRunning), workerLLMBody, validWorkerRunToken(t))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		ledger, ok := attributed.Ledger.(*llmStubLedger)
		if !ok {
			t.Fatalf("unexpected ledger type %T", attributed.Ledger)
		}
		call := ledger.last
		if call.TeamID != llmTestTeam {
			t.Errorf("team = %q, want %q", call.TeamID, llmTestTeam)
		}
		if call.UserID == nil || *call.UserID != llmTestUser {
			t.Errorf("user = %v, want %q", call.UserID, llmTestUser)
		}
		if call.TaskRunID == nil || *call.TaskRunID != "r_1" {
			t.Errorf("task run = %v, want r_1", call.TaskRunID)
		}
		if call.TaskID == nil || *call.TaskID != "t_1" {
			t.Errorf("task = %v, want t_1", call.TaskID)
		}
		if call.Surface != workerSurface {
			t.Errorf("surface = %q, want %q", call.Surface, workerSurface)
		}
	})

	t.Run("never reveals the upstream model, endpoint, or credential", func(t *testing.T) {
		rec := workerLLMRequest(t, gateway, string(model.RunStatusRunning), workerLLMBody, validWorkerRunToken(t))
		for _, secret := range []string{"SECRET-ENDPOINT", "SECRET-CREDENTIAL", "SECRET-UPSTREAM-MODEL"} {
			if strings.Contains(rec.Body.String(), secret) {
				t.Errorf("response leaked %q: %s", secret, rec.Body.String())
			}
		}
	})

	// A worker states what it is doing, never who for. Everything the ledger
	// attributes comes from the token, so there is no field here to forge.
	t.Run("rejects a body carrying fields it does not define", func(t *testing.T) {
		rec := workerLLMRequest(t, gateway,
			string(model.RunStatusRunning),
			`{"model":"default","messages":[{"role":"user","content":"hi"}],"team_id":"tm_other"}`,
			validWorkerRunToken(t))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 for an unknown field", rec.Code)
		}
	})

	t.Run("404s a run the server does not have", func(t *testing.T) {
		h := NewHandler(Config{
			JWTSecret:    llmTestSecret,
			TeamStore:    llmTestTeamStore(),
			LLMGateway:   gateway,
			TaskRunStore: &mock.MockTaskRunStore{},
			WorkerToken:  workerTestToken,
		})
		mux := http.NewServeMux()
		h.Register(mux)

		claims := workerRunClaims()
		claims.TaskRunID = "r_missing"
		req := httptest.NewRequest(http.MethodPost, "/api/worker/task-runs/r_missing/llm/completions", strings.NewReader(workerLLMBody))
		req.Header.Set("Authorization", "Bearer "+workerRunToken(t, claims, time.Hour, time.Now()))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}
