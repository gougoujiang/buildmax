// Package authtoken signs and verifies the run token a worker presents to the
// managed LLM gateway.
//
// It sits beside the scheduler that mints one and the handlers that verify it,
// because those two are in different packages and neither should own the
// other's half of the credential.
//
// Mirrors the design in docs/design/worker-run-token.md.
package authtoken

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// TokenTypeRun marks a run token.
	//
	// It is what keeps the two credentials this deployment signs with one key
	// from substituting for each other: the user-token parser refuses any typ
	// other than "access", and ParseRun refuses anything other than this.
	TokenTypeRun = "run"

	// DefaultRunTokenTTL bounds a run token when a deployment configures none.
	//
	// A run token is not renewable, so this is also the longest run that can
	// reach the gateway for its whole duration. A day is generous for a task run
	// and still short enough that a token recovered from a Job spec afterwards is
	// not indefinitely useful.
	DefaultRunTokenTTL = 24 * time.Hour

	// leeway absorbs clock skew between the replica that signed a token and the
	// one validating it. Matches the user-token parser.
	leeway = 30 * time.Second
)

// ErrIncompleteRunClaims reports a mint call that would produce a token
// ParseRun is bound to reject.
var ErrIncompleteRunClaims = errors.New("run token needs a user, a team, and a task run")

// RunClaims is the identity a run token carries.
//
// Every field is derived from server state at dispatch. Nothing here is ever
// taken from a worker request: the point of the token is that a worker states
// which run it is executing and can state nothing else.
type RunClaims struct {
	// UserID owns the task. It is not always a human login — webhook and system
	// runs carry their configured identity — but it is always the task's owner.
	UserID string
	// TeamID is the authorization and accounting boundary the run spends against.
	TeamID string
	// TaskRunID is the one run this token authorizes.
	TaskRunID string
	// TaskID is correlation context for the call ledger. Optional.
	TaskID string
}

// runToken is the wire form. The registered claims carry subject and expiry;
// the rest are BuildMax scope.
type runToken struct {
	jwt.RegisteredClaims
	Typ string `json:"typ"`
	Tid string `json:"tid"`
	Rid string `json:"rid"`
	Kid string `json:"kid,omitempty"`
}

// MintRun signs a run token valid for ttl from now.
//
// Incomplete claims fail here rather than producing a token that authenticates
// and then fails authorization at the first inference call.
func MintRun(secret string, claims RunClaims, ttl time.Duration, now time.Time) (string, error) {
	if secret == "" {
		return "", errors.New("run token needs a signing secret")
	}
	if claims.UserID == "" || claims.TeamID == "" || claims.TaskRunID == "" {
		return "", ErrIncompleteRunClaims
	}
	if ttl <= 0 {
		ttl = DefaultRunTokenTTL
	}
	token := runToken{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   claims.UserID,
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Typ: TokenTypeRun,
		Tid: claims.TeamID,
		Rid: claims.TaskRunID,
		Kid: claims.TaskID,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, token).SignedString([]byte(secret))
}

// ParseRun verifies a run token and returns the run it authorizes.
//
// Unlike the user-token parser it does not accept an absent typ. That parser
// tolerates one so tokens signed before the claim existed stay valid; no run
// token has ever been signed without it, so here an absent typ means the caller
// presented some other credential.
func ParseRun(tokenStr, secret string) (RunClaims, bool) {
	if secret == "" || tokenStr == "" {
		return RunClaims{}, false
	}
	parsed, err := jwt.ParseWithClaims(tokenStr, &runToken{}, func(*jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithLeeway(leeway))
	if err != nil || !parsed.Valid {
		return RunClaims{}, false
	}
	claims, ok := parsed.Claims.(*runToken)
	if !ok || claims.Typ != TokenTypeRun {
		return RunClaims{}, false
	}
	if claims.Subject == "" || claims.Tid == "" || claims.Rid == "" {
		return RunClaims{}, false
	}
	return RunClaims{
		UserID:    claims.Subject,
		TeamID:    claims.Tid,
		TaskRunID: claims.Rid,
		TaskID:    claims.Kid,
	}, true
}
