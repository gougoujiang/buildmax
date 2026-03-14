package worker

import (
	"net/http"
	"strings"

	"buildmax/internal/server/httputil"
)

// authMiddleware returns a handler that requires Authorization: Bearer <token> or X-Worker-Token.
// If Token is empty, all requests get 401.
func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.cfg.Token == "" {
			httputil.WriteJSONError(w, http.StatusUnauthorized, "worker auth not configured")
			return
		}
		token := ""
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		} else if t := r.Header.Get("X-Worker-Token"); t != "" {
			token = t
		}
		if token != h.cfg.Token {
			httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid or missing worker token")
			return
		}
		next.ServeHTTP(w, r)
	})
}
