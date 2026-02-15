package server

import (
	"encoding/json"
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
