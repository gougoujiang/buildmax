package auth

import "buildmax/internal/core/model"

// Config holds dependencies for unauthenticated auth endpoints (login, OTP).
type Config struct {
	UserStore        model.UserStore
	JWTSecret        string // Required for login
	DefaultQuotaTier string // Used when creating user on signup (OTP)
}
