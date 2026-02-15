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
}

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
	if s.cfg.UserStore == nil || s.cfg.JWTSecret == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"login not configured"}`))
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"error":"method not allowed"}`))
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid request body"}`))
		return
	}
	if req.Email == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"email required"}`))
		return
	}

	user, err := s.cfg.UserStore.UserByEmail(r.Context(), req.Email)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	if user == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"user not found"}`))
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
	tokenStr, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
