package worker

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/server/access"

	coretask "github.com/gougoujiang/buildmax/internal/core/task"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/server/authtoken"
)

// TestRunTokenIsNotAUserLogin is the half of the credential separation this
// package owns. A worker holds a run token so it can reach the managed gateway
// for one run; if that token also opened the user API, the worker — which runs
// model-chosen shell commands — could act as the task's owner everywhere, which
// is the whole reason the run token exists instead of a borrowed access token.
//
// The other half, that a user access token is not accepted as a run token, is
// asserted in internal/server/authtoken.
func TestRunTokenIsNotAUserLogin(t *testing.T) {
	const secret = "test-jwt-secret"
	token, err := authtoken.MintRun(secret, authtoken.RunClaims{
		UserID:    "u1",
		TeamID:    "tm1",
		TaskRunID: "r1",
		TaskID:    "t1",
	}, time.Hour, time.Now())
	if err != nil {
		t.Fatalf("MintRun: %v", err)
	}

	if claims, ok := access.Verify(token, secret); ok {
		t.Errorf("a run token was accepted as an access token for %q", claims.Sub)
	}
	if userID, ok := access.UserIDFromToken(token, secret); ok {
		t.Errorf("a run token resolved to user %q", userID)
	}
}

// workerRouteRequest drives one worker route with the given credential.
func workerRouteRequest(t *testing.T, cfg Config, method, path, token string) int {
	t.Helper()
	mux := http.NewServeMux()
	New(cfg).Register(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

func workerRouteConfig() Config {
	return Config{
		JWTSecret: workerTestSecret,
		TaskRuns: &mock.MockTaskRunStore{
			Runs:     []coretask.Run{{ID: "r_1", TaskID: "t_1", Status: string(coretask.RunStatusScheduled), CreatedAt: time.Unix(1, 0).UTC()}},
			TaskList: []coretask.Task{{ID: "t_1", ConversationID: "c_1", TeamID: llmTestTeam, CreatedBy: llmTestUser, CreatedAt: time.Unix(1, 0).UTC()}},
		},
	}
}

// TestWorkerRoutesAreRunScoped is what retiring the shared secret buys. Every
// worker route names a run in its path, and each one checks that the caller
// holds that run's token — so a compromised run can no longer read another
// team's task input, forge another run's result, or write into another run's
// live stream.
//
// The list is every route the package registers. A route added without a
// credential would be exactly the hole the run token exists to close, so the
// table is the place that notices.
func TestWorkerRoutesAreRunScoped(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/worker/task-runs/r_1"},
		{http.MethodPatch, "/api/worker/task-runs/r_1"},
		{http.MethodPost, "/api/worker/task-runs/r_1/stream"},
		{http.MethodPost, "/api/worker/task-runs/r_1/artifacts"},
		{http.MethodPost, "/api/worker/task-runs/r_1/llm/completions"},
	}
	// A run token is the only credential these routes take, so everything a
	// worker could otherwise present has to be refused by name.
	otherDeployment, err := authtoken.MintRun("some-other-deployments-secret", authtoken.RunClaims{
		UserID: "u_1", TeamID: "tm_1", TaskRunID: "r_1", TaskID: "t_1",
	}, time.Hour, time.Now())
	if err != nil {
		t.Fatalf("MintRun: %v", err)
	}
	refused := []struct {
		name  string
		token string
	}{
		{"no credential", ""},
		{"a junk credential", "not-a-token"},
		{"a static shared secret", "shared-worker-token"},
		{"a token signed by another deployment", otherDeployment},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			cfg := workerRouteConfig()

			if code := workerRouteRequest(t, cfg, route.method, route.path, runTokenFor(t, "r_1", "t_1")); code == http.StatusUnauthorized || code == http.StatusForbidden {
				t.Errorf("this run's own token was refused: %d", code)
			}
			if code := workerRouteRequest(t, cfg, route.method, route.path, runTokenFor(t, "r_somebody_else", "t_1")); code != http.StatusForbidden {
				t.Errorf("another run's token got %d, want 403", code)
			}
			for _, tc := range refused {
				if code := workerRouteRequest(t, cfg, route.method, route.path, tc.token); code != http.StatusUnauthorized {
					t.Errorf("%s got %d, want 401", tc.name, code)
				}
			}
		})
	}
}
