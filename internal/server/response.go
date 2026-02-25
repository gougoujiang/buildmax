package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// writeJSON sets Content-Type application/json, writes status code, and encodes data as JSON.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// writeJSONError sets Content-Type application/json, writes status code, and a body {"error": "<message>"}.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// writeQuotaExceeded writes HTTP 429 with JSON body {"error": reason}.
func writeQuotaExceeded(w http.ResponseWriter, reason string) {
	writeJSONError(w, http.StatusTooManyRequests, reason)
}

// writeInternalError logs the error with attrs then writes 500 and {"error": "internal error"}.
// Use when returning a generic error response so the real cause is visible in logs.
func writeInternalError(w http.ResponseWriter, err error, attrs ...any) {
	slog.Error("handler internal error", append([]any{"err", err}, attrs...)...)
	writeJSONError(w, http.StatusInternalServerError, "internal error")
}
