package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"buildmax/internal/server/httputil"

	"github.com/golang-jwt/jwt/v5"
)

// LoginRequest is the JSON body for POST /api/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Otp      string `json:"otp"`
	Platform string `json:"platform"`
}

// OtpCode is the hardcoded OTP for MVP (no real email sending).
const OtpCode = "123456"

// LoginResponse is the JSON body for a successful login.
type LoginResponse struct {
	Token string    `json:"token"`
	User  LoginUser `json:"user"`
}

// LoginUser is the user subset returned in the login response (snake_case).
type LoginUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
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
	if req.Otp != OtpCode {
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

	resp := LoginResponse{
		Token: tokenStr,
		User: LoginUser{
			ID:    user.UserID,
			Email: user.Email,
			Name:  user.Name,
		},
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}
