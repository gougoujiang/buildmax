package model

import (
	"context"
	"time"
)

// LoginCodeTTLDefault is how long an issued code stays valid when the caller
// does not choose. Long enough to hand a code to someone over a chat message,
// short enough that a leaked one expires before it is useful.
const LoginCodeTTLDefault = time.Hour

// LoginCodeStore issues and redeems single-use login codes.
//
// This is BuildMax's answer to having no OTP delivery channel: an operator
// issues a code out of band (`buildmax-server user login-code`) and delivers it
// however they already talk to the person. It replaces the shared
// `dev_login_otp`, which authenticates any registered email and is therefore a
// standing bypass rather than a credential.
type LoginCodeStore interface {
	// CreateLoginCode issues a single-use code for userID and returns the
	// plaintext, which is never stored and cannot be recovered afterwards.
	CreateLoginCode(ctx context.Context, userID string, ttl time.Duration) (plaintext string, expiresAt int64, err error)

	// ConsumeLoginCode redeems a code and returns the user it belongs to.
	// A code that is unknown, already used, or expired returns ("", nil) —
	// the caller cannot tell which, and neither can an attacker. Redemption is
	// atomic: concurrent calls with the same code produce exactly one winner.
	ConsumeLoginCode(ctx context.Context, plaintext string, now int64) (userID string, err error)
}
