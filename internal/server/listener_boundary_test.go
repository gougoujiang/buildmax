package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestListenerBoundary is the property the two-listener design turns on: the
// public socket cannot dispatch a worker route, and the worker socket cannot
// dispatch a public one. The routes are absent from the other mux, so the miss
// is a 404 before any authentication runs — which is why no run token, valid or
// not, reaches a worker handler through the public listener, and no user token
// reaches a public handler through the worker listener. See
// docs/design/worker-api-network-boundary.md §4.1 and §5.1.
func TestListenerBoundary(t *testing.T) {
	// A JWT secret is set so a worker route that IS present rejects with 401
	// rather than "not configured"; either way it is not the 404 that a missing
	// route gives, which is what these cases distinguish.
	s := New(Config{Addr: ":0", Auth: AuthConfig{JWTSecret: "test-secret"}})
	public := s.Handler()
	worker := s.WorkerHandler()

	if worker == nil {
		t.Fatal("WorkerHandler is nil; the worker route set must be built even with no worker socket")
	}

	const workerRoute = "/api/worker/task-runs/run_abc"

	t.Run("worker route is absent from the public listener", func(t *testing.T) {
		if code := status(t, public, http.MethodGet, workerRoute); code != http.StatusNotFound {
			t.Errorf("GET %s on the public listener = %d; want 404 (route must not be registered there)", workerRoute, code)
		}
	})

	t.Run("worker route is present on the worker listener", func(t *testing.T) {
		// 401 (no run token), not 404: the route is registered and its auth runs.
		if code := status(t, worker, http.MethodGet, workerRoute); code != http.StatusUnauthorized {
			t.Errorf("GET %s on the worker listener = %d; want 401 (route present, run token required)", workerRoute, code)
		}
	})

	t.Run("public routes are absent from the worker listener", func(t *testing.T) {
		for _, path := range []string{"/healthz", "/readyz", "/openapi.json", "/api/plugins"} {
			if code := status(t, worker, http.MethodGet, path); code != http.StatusNotFound {
				t.Errorf("GET %s on the worker listener = %d; want 404 (public routes must not be registered there)", path, code)
			}
		}
	})

	t.Run("the public listener still serves its own routes", func(t *testing.T) {
		if code := status(t, public, http.MethodGet, "/healthz"); code != http.StatusOK {
			t.Errorf("GET /healthz on the public listener = %d; want 200", code)
		}
	})
}

// TestWorkerSocketFollowsConfig covers the wiring the boundary test does not:
// the worker http.Server exists only when an address is configured, while the
// handler is always built so the boundary is testable without a socket.
func TestWorkerSocketFollowsConfig(t *testing.T) {
	off := New(Config{Addr: ":0"})
	if off.workerSrv != nil {
		t.Error("worker server was built with no WorkerAddr")
	}
	if off.WorkerHandler() == nil {
		t.Error("worker handler must be built even when the socket is off")
	}

	on := New(Config{Addr: ":0", WorkerAddr: "127.0.0.1:0"})
	if on.workerSrv == nil {
		t.Error("worker server was not built despite a WorkerAddr")
	}
	if on.workerSrv.Addr != "127.0.0.1:0" {
		t.Errorf("worker server Addr = %q; want the configured address", on.workerSrv.Addr)
	}
}

// status serves one request against a handler in memory and returns its code.
func status(t *testing.T, h http.Handler, method, target string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec.Code
}
