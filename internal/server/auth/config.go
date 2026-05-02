package auth

import (
	"buildmax/internal/infra/db"
)

// Config holds dependencies for unauthenticated auth endpoints (login, OTP).
type Config struct {
	UserStore        db.UserStore
	JWTSecret        string // Required for login
	DefaultQuotaTier string // Used when creating user on signup (OTP)
}
