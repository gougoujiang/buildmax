// Package testsupport holds helpers that exist only for tests.
//
// It is a separate package because of what it contains rather than what it
// does: signing a JWT is exactly the capability a shipped binary should not
// carry around by accident. These helpers used to live in internal/util, which
// nearly every production package imports, so a token minter compiled into
// every binary BuildMax ships. Nothing here is imported outside _test.go files,
// and this package exists to keep that true and visible.
package testsupport

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TestJWTClaims is used by SignJWT for test tokens that match the server's sub claim.
type TestJWTClaims struct {
	jwt.RegisteredClaims
	Sub string `json:"sub"`
}

// SignJWT builds a JWT with sub claim and 24h expiry for tests.
func SignJWT(sub, secret string) string {
	return SignJWTWithExp(sub, secret, 24*time.Hour)
}

// SignJWTWithExp builds a JWT with sub claim and the given expiry offset from now.
// Use a negative duration to create an already-expired token.
func SignJWTWithExp(sub, secret string, expiresIn time.Duration) string {
	now := time.Now()
	claims := TestJWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(expiresIn)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Sub: sub,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := token.SignedString([]byte(secret))
	return s
}
