// Package access owns who a caller is and what they may do.
//
// It exists because those two questions were answered inside auth.go, which
// made auth.go the file every other route had to reach into: fifteen of them
// called withUserPathTeamAndStore, nineteen called pathValueRequired. Splitting
// the route packages apart is only possible once the question every route asks
// first has a home of its own.
package access

import (
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/gougoujiang/buildmax/internal/util"
)

// TypeAccess separates an access token from anything else this key might ever
// sign. Refresh tokens are opaque random strings rather than JWTs, so today
// there is nothing to confuse an access token with; the claim is here so that
// stays true if that ever changes.
const TypeAccess = "access"

// leeway absorbs clock skew between the replica that signed a token and the one
// validating it. More than one replica has no guarantee their clocks agree to
// the second.
const leeway = 30 * time.Second

// Claims is what an access token carries.
type Claims struct {
	jwt.RegisteredClaims
	Sub string `json:"sub"`
	Typ string `json:"typ,omitempty"`
	// Sid names the login chain this token belongs to, matching session_id in
	// user_refresh_token. It is what lets a logout revoke the right session.
	Sid string `json:"sid,omitempty"`
}

// Mint signs an access token for userID within sessionID.
func Mint(secret, userID, sessionID string, now time.Time, ttl time.Duration) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        util.NewPrefixedID(util.PrefixAuthSession),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Sub: userID,
		Typ: TypeAccess,
		Sid: sessionID,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// Verify checks a token and confirms it is an access token.
//
// An empty typ is accepted: tokens signed before the claim existed are still
// valid until they expire, and rejecting them would sign every user out at
// upgrade -- which in a deployment with no email means an operator issuing a
// login code to each of them by hand.
func Verify(tokenStr, secret string) (*Claims, bool) {
	if secret == "" || tokenStr == "" {
		return nil, false
	}
	token, err := jwt.ParseWithClaims(strings.TrimSpace(tokenStr), &Claims{}, func(*jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithLeeway(leeway))
	if err != nil || !token.Valid {
		return nil, false
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || (claims.Typ != "" && claims.Typ != TypeAccess) {
		return nil, false
	}
	return claims, true
}

// ClaimsFromRequest reads and verifies the bearer token on r.
func ClaimsFromRequest(r *http.Request, secret string) (*Claims, bool) {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) {
		return nil, false
	}
	return Verify(auth[len(prefix):], secret)
}

// UserIDFromRequest returns the subject of r's bearer token.
func UserIDFromRequest(r *http.Request, secret string) (string, bool) {
	claims, ok := ClaimsFromRequest(r, secret)
	if !ok {
		return "", false
	}
	return claims.Sub, true
}

// UserIDFromToken returns the subject of a token taken from somewhere other
// than the Authorization header -- the WebSocket upgrade carries it as a query
// parameter, because a browser cannot set a header on that request.
func UserIDFromToken(tokenStr, secret string) (string, bool) {
	claims, ok := Verify(tokenStr, secret)
	if !ok {
		return "", false
	}
	return claims.Sub, true
}
