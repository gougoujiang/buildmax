package server

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// requireAuth extracts the user id from the request and writes 401 if missing or invalid.
// Returns (userID, true) on success; on failure writes JSON error and returns ("", false).
func requireAuth(w http.ResponseWriter, r *http.Request, jwtSecret string) (string, bool) {
	userID, ok := userIDFromRequest(r, jwtSecret)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return userID, true
}

// userIDFromRequest extracts the user id from the Authorization: Bearer <token> header.
// Returns (userID, true) if the JWT is valid and contains a sub claim, or ("", false) otherwise.
func userIDFromRequest(r *http.Request, jwtSecret string) (string, bool) {
	if jwtSecret == "" {
		return "", false
	}
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return "", false
	}
	tokenStr := strings.TrimSpace(auth[len(prefix):])
	if tokenStr == "" {
		return "", false
	}
	token, err := jwt.ParseWithClaims(tokenStr, &jwtClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(jwtSecret), nil
	})
	if err != nil || !token.Valid {
		return "", false
	}
	claims, ok := token.Claims.(*jwtClaims)
	if !ok || claims.Sub == "" {
		return "", false
	}
	return claims.Sub, true
}
