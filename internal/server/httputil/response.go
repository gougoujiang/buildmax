package httputil

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// WriteJSON sets Content-Type application/json, writes status code, and encodes data as JSON.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// WriteJSONError sets Content-Type application/json, writes status code, and a body {"error": "<message>"}.
func WriteJSONError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}

// WriteInternalError logs the error with logMsg and attrs, then writes 500 and {"error": "internal error"}.
// logMsg is the slog message (e.g. "auth handler error", "portal handler error").
func WriteInternalError(w http.ResponseWriter, err error, logMsg string, attrs ...any) {
	slog.Error(logMsg, append([]any{"err", err}, attrs...)...)
	WriteJSONError(w, http.StatusInternalServerError, "internal error")
}
