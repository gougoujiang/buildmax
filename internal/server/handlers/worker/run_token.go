package worker

import (
	"net/http"
	"strings"

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

// runScopedWorkerMiddleware guards the routes whose handler does not need the
// claims. The rule is the same one requireRunToken applies, because every route
// here names a run in its path and none of them may reach outside it.
func (h *Handler) runScopedWorkerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		taskRunID, ok := httputil.PathValue(w, r, "task_run_id")
		if !ok {
			return
		}
		if _, ok := h.requireRunToken(w, r, taskRunID); !ok {
			return
		}
		next.ServeHTTP(w, r)
	})
}
