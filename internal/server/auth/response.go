package auth

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

func writeInternalError(w http.ResponseWriter, err error, attrs ...any) {
	httputil.WriteInternalError(w, err, "auth handler error", attrs...)
}
