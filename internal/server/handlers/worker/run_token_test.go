package worker

import (
	"github.com/gougoujiang/buildmax/internal/server/access"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
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
		JWTSecret:   runRouteSecret,
		WorkerToken: "shared-worker-token",
		TaskRuns: &mock.MockTaskRunStore{
			Runs:     []model.TaskRun{{ID: "r_1", TaskID: "t_1", Status: string(model.RunStatusScheduled), CreatedAt: 1}},
			TaskList: []model.Task{{ID: "t_1", ConversationID: "c_1", TeamID: "tm_1", CreatedBy: "u_1", CreatedAt: 1}},
		},
	}
}

const runRouteSecret = "test-run-route-secret"

func runRouteToken(t *testing.T, taskRunID string) string {
	t.Helper()
	token, err := authtoken.MintRun(runRouteSecret, authtoken.RunClaims{
		UserID: "u_1", TeamID: "tm_1", TaskRunID: taskRunID, TaskID: "t_1",
	}, time.Hour, time.Now())
	if err != nil {
		t.Fatalf("MintRun: %v", err)
	}
	return token
}

// TestWorkerRoutesAreRunScoped is what retiring the shared secret buys. Every
// worker route names a run in its path, and each one now checks that the caller
// holds that run's token — so a compromised run can no longer read another
// team's task input, forge another run's result, or write into another run's
// live stream.
func TestWorkerRoutesAreRunScoped(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/worker/task-runs/r_1"},
		{http.MethodPatch, "/api/worker/task-runs/r_1"},
		{http.MethodPost, "/api/worker/task-runs/r_1/stream"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			cfg := workerRouteConfig()

			if code := workerRouteRequest(t, cfg, route.method, route.path, runRouteToken(t, "r_1")); code == http.StatusUnauthorized || code == http.StatusForbidden {
				t.Errorf("this run's own token was refused: %d", code)
			}
			if code := workerRouteRequest(t, cfg, route.method, route.path, runRouteToken(t, "r_somebody_else")); code != http.StatusForbidden {
				t.Errorf("another run's token got %d, want 403", code)
			}
			if code := workerRouteRequest(t, cfg, route.method, route.path, "not-a-token"); code != http.StatusUnauthorized {
				t.Errorf("a junk credential got %d, want 401", code)
			}
			if code := workerRouteRequest(t, cfg, route.method, route.path, ""); code != http.StatusUnauthorized {
				t.Errorf("no credential got %d, want 401", code)
			}
		})
	}
}

// TestSharedWorkerTokenStillWorksForOneRelease records the transition. An older
// server mints no run token while the worker image it dispatches already expects
// one, so removing the fallback in the same release would fail every run in that
// upgrade window.
func TestSharedWorkerTokenStillWorksForOneRelease(t *testing.T) {
	cfg := workerRouteConfig()
	if code := workerRouteRequest(t, cfg, http.MethodGet, "/api/worker/task-runs/r_1", "shared-worker-token"); code != http.StatusOK {
		t.Errorf("the shared worker token got %d, want 200 during the transition", code)
	}

	// It is a fallback, not a second credential: a deployment that no longer
	// configures one has nothing to fall back to.
	cfg.WorkerToken = ""
	if code := workerRouteRequest(t, cfg, http.MethodGet, "/api/worker/task-runs/r_1", "shared-worker-token"); code != http.StatusUnauthorized {
		t.Errorf("a stale shared token got %d after the deployment stopped configuring one, want 401", code)
	}
}
