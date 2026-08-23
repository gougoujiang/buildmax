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
// This is BuildMax's answer to having no mail channel: an operator issues a
// code out of band (`buildmax-server user login-code`) and delivers it however
// they already talk to the person.
//
// It is not the everyday credential — a password is. A code is what claims a
// new account and what recovers a forgotten password, which is why it is
// single-use and short-lived: it exists to be spent once, on the way to
// setting a password.
type LoginCodeStore interface {
	// CreateLoginCode issues a single-use code for userID and returns the
	// plaintext, which is never stored and cannot be recovered afterwards.
	CreateLoginCode(ctx context.Context, userID string, ttl time.Duration) (plaintext string, expiresAt time.Time, err error)

	// ConsumeLoginCode redeems a code that was issued to userID. A code that
	// is unknown, already used, expired, or issued to somebody else returns
	// (false, nil) — the caller cannot tell which, and neither can an
	// attacker. Redemption is atomic: concurrent calls with the same code
	// produce exactly one winner.
	//
	// The account is named by the caller rather than reported back, so that a
	// code submitted with the wrong address is left untouched. A redemption
	// that spent the code first and checked the account afterwards burned it
	// on a typo, and the person retrying with the right address was then
	// refused for a reason nobody could see.
	ConsumeLoginCode(ctx context.Context, plaintext, userID string, now time.Time) (redeemed bool, err error)
}
