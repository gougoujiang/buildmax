package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/llmgateway"
)

const workerTestToken = "worker-token-for-tests"

// workerLLMRequest issues a worker-route completion for a run in the given
// status. A wrong token is what an unauthenticated caller looks like.
func workerLLMRequest(t *testing.T, gateway *llmgateway.Service, runStatus, body, token string) *httptest.ResponseRecorder {
	t.Helper()
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

	req := httptest.NewRequest(http.MethodPost, "/api/worker/task-runs/r_1/llm/completions", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

const workerLLMBody = `{"model":"default","messages":[{"role":"user","content":"hi"}]}`

func TestWorkerLLMCompletions(t *testing.T) {
	gateway := llmTestService(t, &llmStubClient{content: "answer"}, nil)

	t.Run("answers a run that is executing", func(t *testing.T) {
		rec := workerLLMRequest(t, gateway, string(model.RunStatusRunning), workerLLMBody, workerTestToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "answer") {
			t.Errorf("body %q should carry the completion", rec.Body.String())
		}
	})

	// The worker token says the caller is a worker, not which run it owns. The
	// run's status is what stops a token holder from spending a team's quota
	// against work that finished weeks ago.
	t.Run("refuses a run that is not executing", func(t *testing.T) {
		for _, status := range []model.RunStatus{
			model.RunStatusPending,
			model.RunStatusScheduled,
			model.RunStatusSucceeded,
			model.RunStatusFailed,
		} {
			rec := workerLLMRequest(t, gateway, string(status), workerLLMBody, workerTestToken)
			if rec.Code != http.StatusConflict {
				t.Errorf("status %s: got %d, want 409", status, rec.Code)
			}
		}
	})

	t.Run("requires the worker token", func(t *testing.T) {
		rec := workerLLMRequest(t, gateway, string(model.RunStatusRunning), workerLLMBody, "not-the-token")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("never reveals the upstream model, endpoint, or credential", func(t *testing.T) {
		rec := workerLLMRequest(t, gateway, string(model.RunStatusRunning), workerLLMBody, workerTestToken)
		for _, secret := range []string{"SECRET-ENDPOINT", "SECRET-CREDENTIAL", "SECRET-UPSTREAM-MODEL"} {
			if strings.Contains(rec.Body.String(), secret) {
				t.Errorf("response leaked %q: %s", secret, rec.Body.String())
			}
		}
	})

	// A worker states what it is doing, never who for. Everything the ledger
	// attributes comes from the run, so there is no field here to forge.
	t.Run("rejects a body carrying fields it does not define", func(t *testing.T) {
		rec := workerLLMRequest(t, gateway,
			string(model.RunStatusRunning),
			`{"model":"default","messages":[{"role":"user","content":"hi"}],"team_id":"tm_other"}`,
			workerTestToken)
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
		req := httptest.NewRequest(http.MethodPost, "/api/worker/task-runs/r_missing/llm/completions", strings.NewReader(workerLLMBody))
		req.Header.Set("Authorization", "Bearer "+workerTestToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}
