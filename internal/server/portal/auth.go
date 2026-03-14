package portal

import (
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

func (h *Handler) withWorkspaceAuth(w http.ResponseWriter, r *http.Request, pathKey string) (userID, workspaceID string, ok bool) {
	userID, ok = requireAuth(w, r, h.cfg.JWTSecret)
	if !ok {
		return "", "", false
	}
	workspaceID = r.PathValue(pathKey)
	if workspaceID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, pathKey+" required")
		return "", "", false
	}
	owned, err := h.userOwnsWorkspace(r, userID, workspaceID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "with_workspace_auth", pathKey, workspaceID)
		return "", "", false
	}
	if !owned {
		httputil.WriteJSONError(w, http.StatusForbidden, "forbidden")
		return "", "", false
	}
	return userID, workspaceID, true
}

func (h *Handler) userOwnsWorkspace(r *http.Request, userID, workspaceID string) (bool, error) {
	if h.cfg.WorkspaceStore == nil {
		return false, nil
	}
	return h.cfg.WorkspaceStore.WorkspaceBelongsToUser(r.Context(), workspaceID, userID)
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

// withWorkspaceAndStore runs withWorkspaceAuth then requireStore; returns (userID, workspaceID, true) only when both succeed.
func (h *Handler) withWorkspaceAndStore(w http.ResponseWriter, r *http.Request, pathKey string, store interface{}, unavailableMsg string) (userID, workspaceID string, ok bool) {
	userID, workspaceID, ok = h.withWorkspaceAuth(w, r, pathKey)
	if !ok {
		return "", "", false
	}
	if !h.requireStore(w, store, unavailableMsg) {
		return "", "", false
	}
	return userID, workspaceID, true
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
