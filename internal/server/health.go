package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// readinessTimeout bounds the whole probe. A readiness endpoint that can hang
// is worse than one that reports unavailable: Kubernetes waits on it, and a
// slow dependency turns into a stuck rollout rather than a failed one.
const readinessTimeout = 3 * time.Second

// ReadinessCheck is one dependency the server needs before it can serve
// traffic. The server does not know what a database or an object store is —
// bootstrap supplies the probes, which keeps infrastructure detail out of this
// layer.
type ReadinessCheck struct {
	// Name appears in the response, so it must be safe to show an
	// unauthenticated caller: "database", not a DSN.
	Name  string
	Probe func(ctx context.Context) error
}

type readinessCheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type readinessResponse struct {
	Status string                 `json:"status"`
	Checks []readinessCheckResult `json:"checks"`
}

// healthzHandler answers whether the process is alive. It deliberately checks
// nothing: a liveness probe that fails on a dependency outage restarts a
// healthy server and turns a recoverable outage into a crash loop. Readiness is
// what reflects dependencies.
func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyzHandler answers whether the server can serve traffic, by probing every
// dependency bootstrap registered.
//
// The response names which check failed but never why. Connection errors carry
// DSNs, endpoints, and bucket names, and this endpoint is unauthenticated —
// the reason goes to the server log, where an operator already has to be.
//
// An empty check list answers ready with an empty list rather than inventing
// confidence: a caller can see that nothing was verified.
func (s *Server) readyzHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	results := make([]readinessCheckResult, 0, len(s.cfg.Readiness))
	ready := true
	for _, check := range s.cfg.Readiness {
		if check.Probe == nil {
			continue
		}
		status := "ok"
		if err := check.Probe(ctx); err != nil {
			ready = false
			status = "failed"
			slog.Warn("readiness check failed", "check", check.Name, "err", err)
		}
		results = append(results, readinessCheckResult{Name: check.Name, Status: status})
	}

	body := readinessResponse{Status: "ready", Checks: results}
	code := http.StatusOK
	if !ready {
		body.Status = "unavailable"
		code = http.StatusServiceUnavailable
	}
	httputil.WriteJSON(w, code, body)
}
