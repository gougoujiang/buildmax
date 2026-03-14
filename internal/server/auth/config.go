package auth

import (
	"buildmax/internal/storage/entity"
)

// Config holds dependencies for unauthenticated auth endpoints (login, OTP).
type Config struct {
	UserStore       entity.UserStore
	JWTSecret       string // Required for login
	DefaultQuotaTier string // Used when creating user on signup (OTP)
}
