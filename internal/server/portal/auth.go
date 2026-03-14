package portal

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type jwtClaims struct {
	jwt.RegisteredClaims
	Sub string `json:"sub"`
}

func requireAuth(w http.ResponseWriter, r *http.Request, jwtSecret string) (string, bool) {
	userID, ok := userIDFromRequest(r, jwtSecret)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
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
		writeJSONError(w, http.StatusBadRequest, pathKey+" required")
		return "", "", false
	}
	owned, err := h.userOwnsWorkspace(r, userID, workspaceID)
	if err != nil {
		writeInternalError(w, err, "handler", "with_workspace_auth", pathKey, workspaceID)
		return "", "", false
	}
	if !owned {
		writeJSONError(w, http.StatusForbidden, "forbidden")
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
		writeJSONError(w, http.StatusServiceUnavailable, unavailableMessage)
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
