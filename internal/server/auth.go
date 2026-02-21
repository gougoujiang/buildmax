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

// withWorkspaceAuth performs requireAuth, extracts workspace_id from the path, and verifies ownership.
// pathKey is the path variable name (e.g. "workspace_id"). Returns (userID, workspaceID, true) on success;
// on failure writes the appropriate JSON error and returns ("", "", false).
func (s *Server) withWorkspaceAuth(w http.ResponseWriter, r *http.Request, pathKey string) (userID, workspaceID string, ok bool) {
	userID, ok = requireAuth(w, r, s.cfg.JWTSecret)
	if !ok {
		return "", "", false
	}
	workspaceID = r.PathValue(pathKey)
	if workspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, pathKey+" required")
		return "", "", false
	}
	owned, err := s.userOwnsWorkspace(r, userID, workspaceID)
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

// userOwnsWorkspace returns whether the user owns the workspace. Returns (false, nil) if WorkspaceStore is nil.
func (s *Server) userOwnsWorkspace(r *http.Request, userID, workspaceID string) (bool, error) {
	if s.cfg.WorkspaceStore == nil {
		return false, nil
	}
	return s.cfg.WorkspaceStore.WorkspaceBelongsToUser(r.Context(), workspaceID, userID)
}

// requireStore writes 503 with the given message and returns false if store is nil; otherwise returns true.
func (s *Server) requireStore(w http.ResponseWriter, store interface{}, unavailableMessage string) bool {
	if store == nil {
		writeJSONError(w, http.StatusServiceUnavailable, unavailableMessage)
		return false
	}
	return true
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
