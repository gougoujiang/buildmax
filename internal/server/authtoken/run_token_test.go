package authtoken

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-signing-secret"

func testClaims() RunClaims {
	return RunClaims{
		UserID:    "u_alice",
		TeamID:    "tm_example",
		TaskRunID: "r_example",
		TaskID:    "t_example",
	}
}

func TestRunTokenRoundTrip(t *testing.T) {
	token, err := MintRun(testSecret, testClaims(), time.Hour, time.Now())
	if err != nil {
		t.Fatalf("MintRun: %v", err)
	}
	got, ok := ParseRun(token, testSecret)
	if !ok {
		t.Fatal("a freshly minted token did not parse")
	}
	if got != testClaims() {
		t.Errorf("ParseRun = %+v, want %+v", got, testClaims())
	}
}

// TestRunTokenNeedsCompleteClaims records that an unusable token is never
// signed: a token missing its team would authenticate and then fail
// authorization at the first inference call, which is a worse failure than
// refusing to mint it.
func TestRunTokenNeedsCompleteClaims(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RunClaims)
	}{
		{"no user", func(c *RunClaims) { c.UserID = "" }},
		{"no team", func(c *RunClaims) { c.TeamID = "" }},
		{"no run", func(c *RunClaims) { c.TaskRunID = "" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims := testClaims()
			tc.mutate(&claims)
			if _, err := MintRun(testSecret, claims, time.Hour, time.Now()); !errors.Is(err, ErrIncompleteRunClaims) {
				t.Errorf("MintRun error = %v, want ErrIncompleteRunClaims", err)
			}
		})
	}

	// The task ID is correlation only; a run without one still gets a token.
	claims := testClaims()
	claims.TaskID = ""
	token, err := MintRun(testSecret, claims, time.Hour, time.Now())
	if err != nil {
		t.Fatalf("MintRun without a task id: %v", err)
	}
	if got, ok := ParseRun(token, testSecret); !ok || got.TaskID != "" {
		t.Errorf("ParseRun = %+v, ok = %v", got, ok)
	}
}

func TestRunTokenRefusals(t *testing.T) {
	valid, err := MintRun(testSecret, testClaims(), time.Hour, time.Now())
	if err != nil {
		t.Fatalf("MintRun: %v", err)
	}
	expired, err := MintRun(testSecret, testClaims(), time.Minute, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("MintRun expired: %v", err)
	}

	tests := []struct {
		name   string
		token  string
		secret string
	}{
		{"empty token", "", testSecret},
		{"empty secret", valid, ""},
		{"another deployment's key", valid, "some-other-secret"},
		{"expired", expired, testSecret},
		{"not a token at all", "not.a.jwt", testSecret},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := ParseRun(tc.token, tc.secret); ok {
				t.Error("the token was accepted")
			}
		})
	}
}

// TestParseRunRefusesOtherTokenTypes is the half of the credential separation
// this package owns: whatever else the deployment signs with this key, only a
// token that says it is a run token gets a run's authority. The other half —
// that a run token is not accepted as a user login — is asserted where the user
// token is parsed.
func TestParseRunRefusesOtherTokenTypes(t *testing.T) {
	for _, typ := range []string{"", "access"} {
		t.Run("typ="+typ, func(t *testing.T) {
			token := runToken{
				RegisteredClaims: jwt.RegisteredClaims{
					Subject:   "u_alice",
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				},
				Typ: typ,
				Tid: "tm_example",
				Rid: "r_example",
			}
			signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, token).SignedString([]byte(testSecret))
			if err != nil {
				t.Fatalf("sign: %v", err)
			}
			if _, ok := ParseRun(signed, testSecret); ok {
				t.Errorf("a token with typ %q was accepted as a run token", typ)
			}
		})
	}
}

func TestMintRunFallsBackToDefaultTTL(t *testing.T) {
	now := time.Now()
	token, err := MintRun(testSecret, testClaims(), 0, now)
	if err != nil {
		t.Fatalf("MintRun: %v", err)
	}
	parsed, err := jwt.ParseWithClaims(token, &runToken{}, func(*jwt.Token) (any, error) {
		return []byte(testSecret), nil
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	claims, ok := parsed.Claims.(*runToken)
	if !ok {
		t.Fatal("unexpected claims type")
	}
	want := now.Add(DefaultRunTokenTTL)
	if got := claims.ExpiresAt.Time; got.Sub(want).Abs() > time.Second {
		t.Errorf("expiry = %v, want about %v", got, want)
	}
}
