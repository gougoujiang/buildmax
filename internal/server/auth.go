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

// requireWorkspaceStore writes 503 "workspaces not configured" and returns false if WorkspaceStore is nil.
func (s *Server) requireWorkspaceStore(w http.ResponseWriter) bool {
	if s.cfg.WorkspaceStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "workspaces not configured")
		return false
	}
	return true
}

// requireProjectStore writes 503 "projects not configured" and returns false if ProjectStore is nil.
func (s *Server) requireProjectStore(w http.ResponseWriter) bool {
	if s.cfg.ProjectStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "projects not configured")
		return false
	}
	return true
}

// requireTaskStore writes 503 "tasks not configured" and returns false if TaskStore is nil.
func (s *Server) requireTaskStore(w http.ResponseWriter) bool {
	if s.cfg.TaskStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "tasks not configured")
		return false
	}
	return true
}

// requireArtifactStore writes 503 "artifacts not configured" and returns false if ArtifactStore is nil.
func (s *Server) requireArtifactStore(w http.ResponseWriter) bool {
	if s.cfg.ArtifactStore == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "artifacts not configured")
		return false
	}
	return true
}

// requirePersistStorage writes 503 "persist storage not configured" and returns false if PersistStorage is nil.
func (s *Server) requirePersistStorage(w http.ResponseWriter) bool {
	if s.cfg.PersistStorage == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "persist storage not configured")
		return false
	}
	return true
}

// requireArtifactStorage writes 503 "artifact storage not configured" and returns false if ArtifactStorage is nil.
func (s *Server) requireArtifactStorage(w http.ResponseWriter) bool {
	if s.cfg.ArtifactStorage == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "artifact storage not configured")
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
