package server

import (
	"net/http"

	"buildmax/internal/server/httputil"
)

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	httputil.WriteJSON(w, status, data)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	httputil.WriteJSONError(w, status, message)
}

func writeQuotaExceeded(w http.ResponseWriter, reason string) {
	httputil.WriteQuotaExceeded(w, reason)
}

func writeInternalError(w http.ResponseWriter, err error, attrs ...any) {
	httputil.WriteInternalError(w, err, "handler internal error", attrs...)
}
