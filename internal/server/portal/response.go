package portal

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeQuotaExceeded(w http.ResponseWriter, reason string) {
	writeJSONError(w, http.StatusTooManyRequests, reason)
}

func writeInternalError(w http.ResponseWriter, err error, attrs ...any) {
	slog.Error("portal handler error", append([]any{"err", err}, attrs...)...)
	writeJSONError(w, http.StatusInternalServerError, "internal error")
}
