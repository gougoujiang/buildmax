package handlers

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/gougoujiang/buildmax/internal/server/authtoken"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// requireRunToken authenticates a worker as the executor of one specific run.
//
// The deployment-wide worker token it replaced proved the caller was *a* worker
// and nothing more, which is why every route behind it had to reconstruct who
// the call was for from whatever run ID the caller named. A run token carries
// the user, the team, and the run the server assigned at dispatch, so
// authorization and attribution both come from the credential.
//
// taskRunID is the run named in the path, and it must match the token's. A
// worker executes model-chosen code; without this check a compromised run could
// spend another team's quota by presenting its own valid token against a
// different run's URL.
//
// Mirrors the design in docs/design/worker-run-token.md.
func (h *Handler) requireRunToken(w http.ResponseWriter, r *http.Request, taskRunID string) (authtoken.RunClaims, bool) {
	if h.cfg.JWTSecret == "" {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "run token authentication is not configured")
		return authtoken.RunClaims{}, false
	}
	claims, ok := authtoken.ParseRun(bearerToken(r), h.cfg.JWTSecret)
	if !ok {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid or expired run token")
		return authtoken.RunClaims{}, false
	}
	if claims.TaskRunID != taskRunID {
		httputil.WriteJSONError(w, http.StatusForbidden, "this run token does not authorize that task run")
		return authtoken.RunClaims{}, false
	}
	return claims, true
}

// bearerToken returns the Authorization bearer value, or "" when the header is
// absent or shaped differently.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, prefix))
}

// runScopedWorkerMiddleware authenticates a worker for the run named in the
// path, accepting the deployment-wide worker token as a deprecated fallback.
//
// The fallback exists for one release, and only because of upgrade order: a
// server pod that has not restarted yet mints no run token, while the worker
// image it dispatches already expects to present one. Without the fallback that
// window fails every run.
//
// It is not a permanent second credential. A shared secret proves the caller is
// some worker, which on these routes means it can read any team's task input,
// forge any run's result, and push into any run's live stream. Removing it is
// the point of the run token.
func (h *Handler) runScopedWorkerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		taskRunID := r.PathValue("task_run_id")
		if taskRunID == "" {
			httputil.WriteJSONError(w, http.StatusBadRequest, "task_run_id required")
			return
		}
		token := bearerToken(r)
		if token == "" {
			token = r.Header.Get("X-Worker-Token")
		}

		if claims, ok := authtoken.ParseRun(token, h.cfg.JWTSecret); ok {
			if claims.TaskRunID != taskRunID {
				httputil.WriteJSONError(w, http.StatusForbidden, "this run token does not authorize that task run")
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		if h.cfg.WorkerToken != "" && token == h.cfg.WorkerToken {
			warnSharedWorkerToken()
			next.ServeHTTP(w, r)
			return
		}
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid or missing run token")
	})
}

// sharedWorkerTokenWarning fires once per process. A worker calls these routes
// several times per run, so warning per request would bury the message it is
// trying to deliver.
var sharedWorkerTokenWarning sync.Once

func warnSharedWorkerToken() {
	sharedWorkerTokenWarning.Do(func() {
		slog.Warn("a worker authenticated with the deployment-wide worker token, which is deprecated",
			"why", "it names no run, so it authorizes reading any team's task input and writing any run's result",
			"fix", "upgrade the server so it mints a run token for every dispatched run")
	})
}
