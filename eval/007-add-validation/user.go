package user

import "errors"

// Sentinel errors returned by Register.
var (
	ErrEmptyName    = errors.New("name is required")
	ErrInvalidEmail = errors.New("invalid email address")
	ErrWeakPassword = errors.New("password must be at least 8 characters")
)

// RegisterRequest holds data for a new user registration.
type RegisterRequest struct {
	Name     string
	Email    string
	Password string
}
