package model

import "context"

// User is the user model. JSON uses snake_case per project convention.
// Internal numeric ID is retained for compatibility but is not part of the public API.
type User struct {
	ID                uint    `json:"-"`
	UserID            string  `json:"user_id"`
	Email             string  `json:"email"`
	Name              string  `json:"name"`
	QuotaTier         string  `json:"quota_tier,omitempty"`
	LastLoginAt       *int64  `json:"last_login_at,omitempty"`
	LastLoginPlatform *string `json:"last_login_platform,omitempty"`
	CreatedAt         int64   `json:"created_at"`
}

// UserStore looks up users by email and creates new users.
type UserStore interface {
	UserByEmail(ctx context.Context, email string) (*User, error)
	// GetUser returns the user by user_id, or (nil, nil) when not found.
	GetUser(ctx context.Context, userID string) (*User, error)
	// CreateUser creates a user with the given email. defaultQuotaTier is applied when non-empty. Returns ErrEmailExists if the email is already registered.
	CreateUser(ctx context.Context, email string, defaultQuotaTier string) (*User, error)
	// UpdateLoginMeta records the last login timestamp and platform for the user.
	UpdateLoginMeta(ctx context.Context, userID string, loginAt int64, platform string) error
}
