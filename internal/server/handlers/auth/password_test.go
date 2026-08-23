package auth

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
)

const testPassword = "correct horse battery staple"

func hashFor(t *testing.T, plaintext string) string {
	t.Helper()
	hash, err := model.HashPassword(plaintext)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	return hash
}

// newPasswordMux wires a handler where u1 signs in with testPassword.
func newPasswordMux(t *testing.T, cfg Config) (*http.ServeMux, *mock.MockPasswordStore) {
	t.Helper()
	user := &model.User{ID: "u1", Email: "a@b.c", Name: "Alice"}
	passwords := &mock.MockPasswordStore{Hashes: map[string]string{"u1": hashFor(t, testPassword)}}
	if cfg.Users == nil {
		cfg.Users = &mock.MockUserStore{
			ByEmail: map[string]*model.User{"a@b.c": user},
			ByID:    map[string]*model.User{"u1": user},
		}
	}
	if cfg.Passwords == nil {
		cfg.Passwords = passwords
	} else if s, ok := cfg.Passwords.(*mock.MockPasswordStore); ok {
		passwords = s
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "test-jwt-secret"
	}
	if cfg.RefreshTokens == nil {
		cfg.RefreshTokens = &mock.MockRefreshTokenStore{}
	}
	mux := http.NewServeMux()
	New(cfg).Register(mux)
	return mux, passwords
}

func TestPasswordLoginIssuesASession(t *testing.T) {
	mux, _ := newPasswordMux(t, Config{})
	rec := postJSON(t, mux, "/api/login",
		`{"email":"a@b.c","password":"`+testPassword+`","platform":"portal"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	if body["access_token"] == "" {
		t.Error("no access token issued")
	}
	// A password login is a session like any other, so it gets both halves.
	if _, ok := body["refresh_token"].(string); !ok {
		t.Error("a password login returned no refresh token")
	}
}

// Every failure answers the same way. Telling an unknown address apart from a
// wrong password turns the login form into a way to ask who has an account.
func TestPasswordLoginFailuresAreIndistinguishable(t *testing.T) {
	noPassword := &model.User{ID: "u2", Email: "nopass@b.c"}
	mux, _ := newPasswordMux(t, Config{
		Users: &mock.MockUserStore{
			ByEmail: map[string]*model.User{
				"a@b.c":      {ID: "u1", Email: "a@b.c", Name: "Alice"},
				"nopass@b.c": noPassword,
			},
			ByID: map[string]*model.User{"u1": {ID: "u1", Email: "a@b.c"}, "u2": noPassword},
		},
	})

	tests := []struct {
		name string
		body string
	}{
		{"wrong password", `{"email":"a@b.c","password":"wrong but long enough"}`},
		{"unknown address", `{"email":"nobody@example.com","password":"` + testPassword + `"}`},
		{"account with no password set", `{"email":"nopass@b.c","password":"` + testPassword + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postJSON(t, mux, "/api/login", tt.body, nil)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := decodeJSON(t, rec)["error"]; got != invalidPasswordMessage {
				t.Errorf("error = %q, want the same message every failure gives", got)
			}
		})
	}
}

// A password shorter than the minimum is refused at login too, not only when
// set — otherwise a hash written before the rule existed would still work.
func TestPasswordLoginRejectsAnEmptyPasswordWithoutFallingBackToOtp(t *testing.T) {
	mux, _ := newPasswordMux(t, Config{})
	rec := postJSON(t, mux, "/api/login", `{"email":"a@b.c","password":"","otp":""}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestSetPasswordRequiresTheCurrentOneWhenThereIsOne(t *testing.T) {
	mux, passwords := newPasswordMux(t, Config{})
	access, _ := loginWithPassword(t, mux)
	const next = "a much longer replacement passphrase"

	// Without the current password, the session alone is not enough. A stolen
	// access token cannot be revoked before it expires; letting one set a
	// password would make a temporary theft permanent.
	rec := postJSON(t, mux, "/api/password", `{"new_password":"`+next+`"}`, map[string]string{
		"Authorization": "Bearer " + access,
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if model.VerifyPassword(passwords.Hashes["u1"], next) {
		t.Fatal("the password changed without the current one")
	}

	rec = postJSON(t, mux, "/api/password",
		`{"current_password":"`+testPassword+`","new_password":"`+next+`"}`,
		map[string]string{"Authorization": "Bearer " + access})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !model.VerifyPassword(passwords.Hashes["u1"], next) {
		t.Error("the new password was not stored")
	}
}

// Claiming an account: the session came from an operator-issued login code,
// which is the strongest proof this deployment has, and there is no current
// password to present.
func TestSetFirstPasswordNeedsOnlyTheSession(t *testing.T) {
	user := &model.User{ID: "u1", Email: "a@b.c", Name: "Alice"}
	passwords := &mock.MockPasswordStore{}
	mux := http.NewServeMux()
	New(Config{
		Users: &mock.MockUserStore{
			ByEmail: map[string]*model.User{"a@b.c": user},
			ByID:    map[string]*model.User{"u1": user},
		},
		LoginCodes: &mock.MockLoginCodeStore{Codes: map[string]*mock.MockLoginCode{
			"code-1": {UserID: "u1", ExpiresAt: time.Now().Add(time.Hour).UTC()},
		}},
		Passwords:     passwords,
		RefreshTokens: &mock.MockRefreshTokenStore{},
		JWTSecret:     "test-jwt-secret",
	}).Register(mux)

	rec := postJSON(t, mux, "/api/login", `{"email":"a@b.c","otp":"code-1"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login with a code failed: %d %s", rec.Code, rec.Body.String())
	}
	access, _ := decodeJSON(t, rec)["access_token"].(string)

	const chosen = "the passphrase they chose themselves"
	rec = postJSON(t, mux, "/api/password", `{"new_password":"`+chosen+`"}`, map[string]string{
		"Authorization": "Bearer " + access,
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !model.VerifyPassword(passwords.Hashes["u1"], chosen) {
		t.Fatal("the first password was not stored")
	}

	// And it works: recovery is complete, not half-done.
	rec = postJSON(t, mux, "/api/login", `{"email":"a@b.c","password":"`+chosen+`"}`, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("the password just set does not sign in: %d", rec.Code)
	}
}

func TestSetPasswordEnforcesTheLengthMinimum(t *testing.T) {
	mux, _ := newPasswordMux(t, Config{})
	access, _ := loginWithPassword(t, mux)

	rec := postJSON(t, mux, "/api/password",
		`{"current_password":"`+testPassword+`","new_password":"short"}`,
		map[string]string{"Authorization": "Bearer " + access})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("at least")) {
		t.Errorf("body %s does not say what the rule is", rec.Body.String())
	}
}

func TestSetPasswordRequiresAuthentication(t *testing.T) {
	mux, _ := newPasswordMux(t, Config{})
	rec := postJSON(t, mux, "/api/password", `{"new_password":"a long enough passphrase"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func loginWithPassword(t *testing.T, mux *http.ServeMux) (accessToken, refreshToken string) {
	t.Helper()
	rec := postJSON(t, mux, "/api/login", `{"email":"a@b.c","password":"`+testPassword+`"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body %s", rec.Code, rec.Body.String())
	}
	body := decodeJSON(t, rec)
	access, _ := body["access_token"].(string)
	refresh, _ := body["refresh_token"].(string)
	return access, refresh
}
