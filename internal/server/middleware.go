package server

import (
	"log/slog"
	"net/http"
	"strings"
)

// corsMiddleware wraps h and adds CORS headers. allowedOrigin is the value for Access-Control-Allow-Origin (e.g. "http://localhost:5173").
// For OPTIONS (preflight), it responds with 204 and CORS headers without calling h.
func corsMiddleware(h http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// requestLoggingMiddleware logs each request (method, path, remote) for trace/debugging.
func requestLoggingMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		}
		if r.URL.RawQuery != "" {
			attrs = append(attrs, "query", r.URL.RawQuery)
		}
		slog.Info("request", attrs...)
		h.ServeHTTP(w, r)
	})
}

// workerAuthMiddleware wraps a handler and requires a valid worker token (Bearer or X-Worker-Token).
// If WorkerToken is empty, all requests are rejected with 401 (worker auth not configured).
func (s *Server) workerAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		next.ServeHTTP(w, r)
	})
}
