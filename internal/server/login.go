package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// LoginRequest is the JSON body for POST /api/login.
type LoginRequest struct {
	Email string `json:"email"`
	Otp   string `json:"otp"`
}

// OtpCode is the hardcoded OTP for MVP (no real email sending).
const OtpCode = "123456"

// LoginResponse is the JSON body for a successful login.
type LoginResponse struct {
	Token string     `json:"token"`
	User  LoginUser  `json:"user"`
}

// LoginUser is the user subset returned in the login response (snake_case).
type LoginUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// jwtClaims are the JWT claims (sub = user id, exp, iat).
type jwtClaims struct {
	jwt.RegisteredClaims
	Sub string `json:"sub"`
}

const jwtExpiry = 24 * time.Hour

func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Stores.UserStore == nil || s.cfg.Auth.JWTSecret == "" {
		writeJSONError(w, http.StatusServiceUnavailable, "login not configured")
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		writeJSONError(w, http.StatusBadRequest, "email required")
		return
	}
	if req.Otp == "" {
		writeJSONError(w, http.StatusBadRequest, "otp required")
		return
	}
	if req.Otp != OtpCode {
		writeJSONError(w, http.StatusUnauthorized, "invalid otp")
		return
	}

	user, err := s.cfg.Stores.UserStore.UserByEmail(r.Context(), req.Email)
	if err != nil {
		writeInternalError(w, err, "handler", "login", "email", req.Email)
		return
	}
	if user == nil {
		writeJSONError(w, http.StatusUnauthorized, "user not found")
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
	tokenStr, err := token.SignedString([]byte(s.cfg.Auth.JWTSecret))
	if err != nil {
		writeInternalError(w, err, "handler", "login", "sign_token")
		return
	}

	resp := LoginResponse{
		Token: tokenStr,
		User: LoginUser{
			ID:    user.UserID,
			Email: user.Email,
			Name:  user.Name,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}
