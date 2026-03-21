package portal

import (
	"encoding/json"
	"net/http"
	"strings"

	"buildmax/internal/server/httputil"

	"github.com/golang-jwt/jwt/v5"
)

type jwtClaims struct {
	jwt.RegisteredClaims
	Sub string `json:"sub"`
}

func requireAuth(w http.ResponseWriter, r *http.Request, jwtSecret string) (string, bool) {
	userID, ok := userIDFromRequest(r, jwtSecret)
	if !ok {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return userID, true
}

func (h *Handler) requireStore(w http.ResponseWriter, store interface{}, unavailableMessage string) bool {
	if store == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, unavailableMessage)
		return false
	}
	return true
}

// pathValueRequired returns the path parameter value for key, or writes 400 and returns ("", false) if missing.
func pathValueRequired(w http.ResponseWriter, r *http.Request, key string) (value string, ok bool) {
	value = r.PathValue(key)
	if value == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, key+" required")
		return "", false
	}
	return value, true
}

func (h *Handler) withUserAndStore(w http.ResponseWriter, r *http.Request, store interface{}, unavailableMsg string) (userID string, ok bool) {
	if !h.requireStore(w, store, unavailableMsg) {
		return "", false
	}
	userID, ok = requireAuth(w, r, h.cfg.JWTSecret)
	if !ok {
		return "", false
	}
	return userID, true
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

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
	return parseJWTSub(tokenStr, jwtSecret)
}

// parseJWTSub validates a raw JWT token string and returns the sub claim.
func parseJWTSub(tokenStr, jwtSecret string) (string, bool) {
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
