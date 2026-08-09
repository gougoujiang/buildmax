package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"

	"github.com/golang-jwt/jwt/v5"
)

// LoginRequest is the JSON body for POST /api/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Otp      string `json:"otp"`
	Platform string `json:"platform"`
}

// Login OTP verification is opt-in. BuildMax does not ship an OTP delivery
// channel yet, so the only supported verifier is the development fixed code in
// Config.DevLoginOTP (server.yaml `dev_login_otp` / BUILDMAX_DEV_LOGIN_OTP).
//
// When that value is empty — the default — POST /api/login is disabled and
// returns 503. This is deliberate: a fixed code that is enabled by default is a
// full authentication bypass for anyone who knows a registered email address.
// Operators must opt in explicitly, and should only do so on deployments that
// are not reachable by untrusted users.

// LoginResponse is the JSON body for a successful login.
type LoginResponse struct {
	Token string    `json:"token"`
	User  LoginUser `json:"user"`
}

// LoginUser is the user subset returned in the login response.
type LoginUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// OtpRequestRequest is the JSON body for POST /api/otp/request.
type OtpRequestRequest struct {
	Email  string `json:"email"`
	Intent string `json:"intent"` // "signup" or "login"; default "signup"
}

// OtpRequestResponse is the JSON body for a successful OTP request.
type OtpRequestResponse struct {
	Message string `json:"message"`
}

type jwtClaims struct {
	jwt.RegisteredClaims
	Sub string `json:"sub"`
}

const jwtExpiry = 24 * time.Hour

func (h *Handler) loginHandler(w http.ResponseWriter, r *http.Request) {
	if h.cfg.UserStore == nil || h.cfg.JWTSecret == "" {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "login not configured")
		return
	}
	if h.cfg.DevLoginOTP == "" {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "login not configured: no otp verifier")
		return
	}
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "email required")
		return
	}
	if req.Otp == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "otp required")
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Otp), []byte(h.cfg.DevLoginOTP)) != 1 {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "invalid otp")
		return
	}
	user, err := h.cfg.UserStore.UserByEmail(r.Context(), req.Email)
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "login", "email", req.Email)
		return
	}
	if user == nil {
		httputil.WriteJSONError(w, http.StatusUnauthorized, "user not found")
		return
	}
	now := time.Now()
	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(jwtExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Sub: user.UserID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(h.cfg.JWTSecret))
	if err != nil {
		httputil.WriteInternalError(w, err, "auth handler error", "handler", "login", "sign_token")
		return
	}
	platform := req.Platform
	if platform == "" {
		platform = "unknown"
	}
	if err := h.cfg.UserStore.UpdateLoginMeta(r.Context(), user.UserID, now.Unix(), platform); err != nil {
		slog.Error("update login meta failed", "err", err, "handler", "login", "user_id", user.UserID)
	}
	httputil.WriteJSON(w, http.StatusOK, LoginResponse{
		Token: tokenStr,
		User:  LoginUser{ID: user.UserID, Email: user.Email, Name: user.Name},
	})
}

func (h *Handler) otpRequestHandler(w http.ResponseWriter, r *http.Request) {
	if h.cfg.UserStore == nil {
		httputil.WriteJSONError(w, http.StatusServiceUnavailable, "otp not configured")
		return
	}
	var req OtpRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "email required")
		return
	}
	intent := req.Intent
	if intent == "" {
		intent = "signup"
	}
	if intent != "signup" && intent != "login" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "intent must be signup or login")
		return
	}
	user, err := h.cfg.UserStore.UserByEmail(r.Context(), req.Email)
	if err != nil {
		httputil.WriteJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if intent == "login" {
		if user == nil {
			httputil.WriteJSONError(w, http.StatusNotFound, "user not found")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, OtpRequestResponse{Message: "otp_sent"})
		return
	}
	// intent == "signup"
	if user != nil {
		httputil.WriteJSONError(w, http.StatusConflict, "email already registered")
		return
	}
	_, err = h.cfg.UserStore.CreateUser(r.Context(), req.Email, h.cfg.DefaultQuotaTier)
	if err != nil {
		if errors.Is(err, model.ErrEmailExists) {
			httputil.WriteJSONError(w, http.StatusConflict, "email already registered")
			return
		}
		httputil.WriteJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, OtpRequestResponse{Message: "otp_sent"})
}

// --- JWT auth middleware helpers ---

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

func (h *Handler) withUserPathTeamAndStore(w http.ResponseWriter, r *http.Request, store interface{}, unavailableMsg string) (userID, teamID string, ok bool) {
	userID, ok = h.withUserAndStore(w, r, store, unavailableMsg)
	if !ok {
		return "", "", false
	}
	if !h.requireStore(w, h.cfg.TeamStore, "teams not configured") {
		return "", "", false
	}
	teamID, ok = pathValueRequired(w, r, "team_id")
	if !ok {
		return "", "", false
	}
	_, resolvedTeamID, ok := h.withExplicitTeam(w, r, userID, teamID)
	if !ok {
		return "", "", false
	}
	return userID, resolvedTeamID, true
}

func (h *Handler) withExplicitTeam(w http.ResponseWriter, r *http.Request, userID, teamID string) (string, string, bool) {
	if !h.requireStore(w, h.cfg.TeamStore, "teams not configured") {
		return "", "", false
	}
	resolvedTeamID, ok := h.resolveTeamID(w, r, userID, teamID)
	if !ok {
		return "", "", false
	}
	return userID, resolvedTeamID, true
}

func (h *Handler) resolveTeamID(w http.ResponseWriter, r *http.Request, userID, explicitTeamID string) (string, bool) {
	if explicitTeamID == "" {
		team, err := h.cfg.TeamStore.GetPersonalTeamByUser(r.Context(), userID)
		if err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "resolve_current_team", "user_id", userID)
			return "", false
		}
		if team == nil {
			httputil.WriteJSONError(w, http.StatusForbidden, "team not found")
			return "", false
		}
		return team.TeamID, true
	}
	teams, err := h.cfg.TeamStore.ListTeamsByUser(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "resolve_current_team", "user_id", userID)
		return "", false
	}
	for _, team := range teams {
		if team.TeamID == explicitTeamID {
			return team.TeamID, true
		}
	}
	httputil.WriteJSONError(w, http.StatusForbidden, "forbidden")
	return "", false
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

func userIDFromToken(tokenStr string, jwtSecret string) (string, bool) {
	if jwtSecret == "" || tokenStr == "" {
		return "", false
	}
	return parseJWTSub(strings.TrimSpace(tokenStr), jwtSecret)
}

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
