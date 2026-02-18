package server

import (
	"net/http"
	"strings"
)

// workerAuthMiddleware wraps a handler and requires a valid worker token (Bearer or X-Worker-Token).
// If WorkerToken is empty, all requests pass (no worker auth enforced).
func (s *Server) workerAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.WorkerToken == "" {
			writeJSONError(w, http.StatusUnauthorized, "worker auth not configured")
			return
		}
		token := ""
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			token = strings.TrimPrefix(h, "Bearer ")
		} else if h := r.Header.Get("X-Worker-Token"); h != "" {
			token = h
		}
		if token != s.cfg.WorkerToken {
			writeJSONError(w, http.StatusUnauthorized, "invalid or missing worker token")
			return
		}
		next(w, r)
	}
}
