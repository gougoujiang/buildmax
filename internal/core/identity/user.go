package identity

import (
	"context"
	"time"
)

// User is the user model. JSON uses snake_case per project convention.
// Internal numeric ID is retained for compatibility but is not part of the public API.
type User struct {
	ID                string     `json:"id"`
	Email             string     `json:"email"`
	Name              string     `json:"name"`
	QuotaTier         string     `json:"quota_tier,omitempty"`
	LastLoginAt       *time.Time `json:"last_login_at,omitempty"`
	LastLoginPlatform *string    `json:"last_login_platform,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	// HasPassword reports whether this account can sign in with a password. The
	// hash itself never travels on this struct — see PasswordStore — so that no
	// handler can serialize it into a response by accident.
	HasPassword bool `json:"has_password"`
	// DisabledAt is nil for an ordinary account. Non-nil means every credential
	// this account holds is refused: password, login code, refresh token, the
	// access token it already has, and its webhook keys. Disabling is not
	// deletion — nothing is removed, and enabling reverses the state and
	// nothing else. See docs/design/system-administration.md section 8.
	DisabledAt *time.Time `json:"disabled_at,omitempty"`
}

// Disabled reports whether the account is currently refused.
func (u User) Disabled() bool { return u.DisabledAt != nil }

// UserStore looks up users by email and creates new users.
type UserStore interface {
	// UserByEmail matches the address without regard to case, and returns
	// (nil, nil) when nobody has it. Login resolves the account this way and
	// compares nothing afterwards, so a case-sensitive implementation would
	// refuse people whose address is stored in another case.
	UserByEmail(ctx context.Context, email string) (*User, error)
	// GetUser returns the user by user_id, or (nil, nil) when not found.
	GetUser(ctx context.Context, userID string) (*User, error)
	// CreateUser creates a user with the given email. defaultQuotaTier is applied when non-empty. Returns ErrEmailExists if the email is already registered.
	CreateUser(ctx context.Context, email string, defaultQuotaTier string) (*User, error)
	// UpdateLoginMeta records the last login timestamp and platform for the user.
	UpdateLoginMeta(ctx context.Context, userID string, loginAt time.Time, platform string) error
	// ListUsers returns accounts newest first with the total count. A non-empty
	// query filters on email as a substring.
	ListUsers(ctx context.Context, query string, limit, offset int) ([]User, int, error)
	// SetUserDisabled disables the account at the given time, or enables it
	// when disabledAt is nil. Returns ErrUserNotFound when there is no such
	// account.
	SetUserDisabled(ctx context.Context, userID string, disabledAt *time.Time) error
}

// PasswordStore reads and writes the one credential a person chose themselves.
//
// It is deliberately separate from UserStore. A password hash is the only value
// in the system whose exposure would reach beyond BuildMax — people reuse
// passwords — so it is fetched only by the code that verifies a login, and
// never rides along on a User that some handler might serialize.
type PasswordStore interface {
	// PasswordHash returns the stored hash for userID, or "" when the account
	// has no password and can only sign in with a login code.
	PasswordHash(ctx context.Context, userID string) (string, error)
	// SetPassword stores an already-hashed password. Hashing belongs to the
	// caller — this interface must not be a place where a plaintext password
	// can be passed by mistake.
	SetPassword(ctx context.Context, userID, encodedHash string, setAt time.Time) error
}
