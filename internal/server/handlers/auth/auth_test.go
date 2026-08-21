package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
)

func TestLoginHandler(t *testing.T) {
	secret := "test-jwt-secret"
	userExists := &model.User{UserID: "u1", Email: "a@b.c", Name: "Alice"}

	// Login needs a verifier. A login code store is one, and is how a real
	// deployment recovers an account; password login has its own test file.

	// Far enough ahead that these codes never expire mid-run.
	farFuture := time.Now().Add(time.Hour).Unix()

	tests := []struct {
		name           string
		userStore      model.UserStore
		loginCodeStore model.LoginCodeStore
		jwtSecret      string
		body           string
		wantStatus     int
		wantBodyHas    string
		wantBodyNotHas string
	}{
		{
			name:      "missing credential returns 400",
			userStore: &mock.MockUserStore{ByID: map[string]*model.User{"u1": userExists}},
			loginCodeStore: &mock.MockLoginCodeStore{Codes: map[string]*mock.MockLoginCode{
				"code-1": {UserID: "u1", ExpiresAt: farFuture},
			}},
			jwtSecret:   secret,
			body:        `{"email":"a@b.c"}`,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "password or otp required",
		},
		{
			name:           "invalid body returns 400",
			userStore:      &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			loginCodeStore: &mock.MockLoginCodeStore{},
			jwtSecret:      secret,
			body:           `{`,
			wantStatus:     http.StatusBadRequest,
		},
		{
			name:           "empty email returns 400",
			userStore:      &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			loginCodeStore: &mock.MockLoginCodeStore{},
			jwtSecret:      secret,
			body:           `{"email":"","otp":"code-1"}`,
			wantStatus:     http.StatusBadRequest,
			wantBodyHas:    "email required",
		},
		{
			name:           "no UserStore returns 503",
			userStore:      nil,
			loginCodeStore: &mock.MockLoginCodeStore{},
			jwtSecret:      secret,
			body:           `{"email":"a@b.c","otp":"code-1"}`,
			wantStatus:     http.StatusServiceUnavailable,
		},
		{
			// Neither credential can be checked, so the server says so rather
			// than answering 401 as though the submission were wrong.
			name:        "no verifier configured returns 503 and issues no token",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{"a@b.c": userExists}},
			jwtSecret:   secret,
			body:        `{"email":"a@b.c","otp":"code-1"}`,
			wantStatus:  http.StatusServiceUnavailable,
			wantBodyHas: "no way to verify a credential",
		},

		// Single-use codes: the recovery path, and how a new account is claimed.
		{
			name:      "valid single-use code returns 200 and token",
			userStore: &mock.MockUserStore{ByID: map[string]*model.User{"u1": userExists}},
			loginCodeStore: &mock.MockLoginCodeStore{Codes: map[string]*mock.MockLoginCode{
				"code-1": {UserID: "u1", ExpiresAt: farFuture},
			}},
			jwtSecret:   secret,
			body:        `{"email":"a@b.c","otp":"code-1"}`,
			wantStatus:  http.StatusOK,
			wantBodyHas: "token",
		},
		{
			name:      "expired code is rejected",
			userStore: &mock.MockUserStore{ByID: map[string]*model.User{"u1": userExists}},
			loginCodeStore: &mock.MockLoginCodeStore{Codes: map[string]*mock.MockLoginCode{
				"code-1": {UserID: "u1", ExpiresAt: 1},
			}},
			jwtSecret:      secret,
			body:           `{"email":"a@b.c","otp":"code-1"}`,
			wantStatus:     http.StatusUnauthorized,
			wantBodyHas:    "invalid otp",
			wantBodyNotHas: "token",
		},
		{
			name:      "already spent code is rejected",
			userStore: &mock.MockUserStore{ByID: map[string]*model.User{"u1": userExists}},
			loginCodeStore: &mock.MockLoginCodeStore{Codes: map[string]*mock.MockLoginCode{
				"code-1": {UserID: "u1", ExpiresAt: farFuture, Used: true},
			}},
			jwtSecret:   secret,
			body:        `{"email":"a@b.c","otp":"code-1"}`,
			wantStatus:  http.StatusUnauthorized,
			wantBodyHas: "invalid otp",
		},
		{
			// The code names the user, so a mismatched email means the code was
			// pasted into the wrong browser. It must not sign anyone in.
			name:      "code submitted with someone else's email is rejected",
			userStore: &mock.MockUserStore{ByID: map[string]*model.User{"u1": userExists}},
			loginCodeStore: &mock.MockLoginCodeStore{Codes: map[string]*mock.MockLoginCode{
				"code-1": {UserID: "u1", ExpiresAt: farFuture},
			}},
			jwtSecret:      secret,
			body:           `{"email":"someone@else.com","otp":"code-1"}`,
			wantStatus:     http.StatusUnauthorized,
			wantBodyNotHas: "token",
		},
		{
			name:      "email case does not matter",
			userStore: &mock.MockUserStore{ByID: map[string]*model.User{"u1": userExists}},
			loginCodeStore: &mock.MockLoginCodeStore{Codes: map[string]*mock.MockLoginCode{
				"code-1": {UserID: "u1", ExpiresAt: farFuture},
			}},
			jwtSecret:   secret,
			body:        `{"email":"A@B.C","otp":"code-1"}`,
			wantStatus:  http.StatusOK,
			wantBodyHas: "token",
		},
		{
			// With a code store wired, login is configured — an unknown code is
			// a rejection, not "this server cannot log anyone in".
			name:           "unknown code returns 401 rather than 503",
			userStore:      &mock.MockUserStore{ByID: map[string]*model.User{"u1": userExists}},
			loginCodeStore: &mock.MockLoginCodeStore{},
			jwtSecret:      secret,
			body:           `{"email":"a@b.c","otp":"nope"}`,
			wantStatus:     http.StatusUnauthorized,
			wantBodyHas:    "invalid otp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			New(Config{
				Users:            tt.userStore,
				LoginCodes:       tt.loginCodeStore,
				JWTSecret:        tt.jwtSecret,
				DefaultQuotaTier: "",
			}).Register(mux)
			req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			body := rec.Body.String()
			if tt.wantBodyHas != "" && !bytes.Contains([]byte(body), []byte(tt.wantBodyHas)) {
				t.Errorf("body %q does not contain %q", body, tt.wantBodyHas)
			}
			if tt.wantBodyNotHas != "" && bytes.Contains([]byte(body), []byte(tt.wantBodyNotHas)) {
				t.Errorf("body %q should not contain %q", body, tt.wantBodyNotHas)
			}
			if tt.wantStatus == http.StatusOK && tt.wantBodyHas == "token" {
				var m map[string]interface{}
				if err := json.Unmarshal([]byte(body), &m); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if _, ok := m["token"]; !ok {
					t.Error("response missing token")
				}
				u, ok := m["user"].(map[string]interface{})
				if !ok {
					t.Error("response missing user object")
				} else if u["id"] != "u1" || u["email"] != "a@b.c" || u["name"] != "Alice" {
					t.Errorf("user = %v", u)
				}
			}
		})
	}
}

func TestOtpRequestHandler(t *testing.T) {
	userExists := &model.User{UserID: "u1", Email: "a@b.c", Name: "Alice"}

	tests := []struct {
		name        string
		userStore   model.UserStore
		allowSignup bool
		body        string
		wantStatus  int
		wantBodyHas string
	}{
		{
			name:        "signup new user returns 200",
			allowSignup: true,
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			body:        `{"email":"new@example.com","intent":"signup"}`,
			wantStatus:  http.StatusOK,
			wantBodyHas: "account_created",
		},
		{
			name:        "signup existing email returns 409",
			allowSignup: true,
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{"a@b.c": userExists}},
			body:        `{"email":"a@b.c","intent":"signup"}`,
			wantStatus:  http.StatusConflict,
			wantBodyHas: "email already registered",
		},
		{
			name:        "login unknown email returns 404",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			body:        `{"email":"nobody@example.com","intent":"login"}`,
			wantStatus:  http.StatusNotFound,
			wantBodyHas: "user not found",
		},
		{
			name:        "login existing user returns 200",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{"a@b.c": userExists}},
			body:        `{"email":"a@b.c","intent":"login"}`,
			wantStatus:  http.StatusOK,
			wantBodyHas: "account_exists",
		},
		{
			name:        "default intent signup creates user",
			allowSignup: true,
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			body:        `{"email":"default@example.com"}`,
			wantStatus:  http.StatusOK,
			wantBodyHas: "account_created",
		},
		{
			name:        "empty email returns 400",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			body:        `{"email":"","intent":"signup"}`,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "email required",
		},
		{
			name:       "invalid body returns 400",
			userStore:  &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			body:       `{`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "no UserStore returns 503",
			userStore:   nil,
			body:        `{"email":"a@b.c","intent":"login"}`,
			wantStatus:  http.StatusServiceUnavailable,
			wantBodyHas: "otp not configured",
		},

		// allowSignup defaults to false in this table, which is the same default
		// a server that never configured it gets.
		{
			name:        "signup is refused by default",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			body:        `{"email":"new@example.com","intent":"signup"}`,
			wantStatus:  http.StatusForbidden,
			wantBodyHas: "signup is disabled",
		},
		{
			name:        "default intent is refused by default too",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			body:        `{"email":"new@example.com"}`,
			wantStatus:  http.StatusForbidden,
			wantBodyHas: "signup is disabled",
		},
		{
			// Closing signup must not lock existing accounts out of logging in.
			name:        "login intent still works when signup is closed",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{"a@b.c": userExists}},
			body:        `{"email":"a@b.c","intent":"login"}`,
			wantStatus:  http.StatusOK,
			wantBodyHas: "account_exists",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			New(Config{
				Users:            tt.userStore,
				AllowSignup:      tt.allowSignup,
				JWTSecret:        "",
				DefaultQuotaTier: "free_trial",
			}).Register(mux)
			req := httptest.NewRequest(http.MethodPost, "/api/otp/request", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			body := rec.Body.String()
			if tt.wantBodyHas != "" && !bytes.Contains([]byte(body), []byte(tt.wantBodyHas)) {
				t.Errorf("body %q does not contain %q", body, tt.wantBodyHas)
			}
		})
	}
}

// A code has to be spent by its first successful use. The table above proves a
// pre-marked code is rejected; this proves the handler is what marks it, by
// replaying the exact request that just succeeded.
func TestLoginHandlerConsumesCodeOnUse(t *testing.T) {
	user := &model.User{UserID: "u1", Email: "a@b.c", Name: "Alice"}
	codes := &mock.MockLoginCodeStore{Codes: map[string]*mock.MockLoginCode{
		"code-1": {UserID: "u1", ExpiresAt: time.Now().Add(time.Hour).Unix()},
	}}
	mux := http.NewServeMux()
	New(Config{
		Users:      &mock.MockUserStore{ByID: map[string]*model.User{"u1": user}},
		LoginCodes: codes,
		JWTSecret:  "test-jwt-secret",
	}).Register(mux)

	post := func() int {
		req := httptest.NewRequest(http.MethodPost, "/api/login",
			bytes.NewBufferString(`{"email":"a@b.c","otp":"code-1"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := post(); got != http.StatusOK {
		t.Fatalf("first login = %d, want 200", got)
	}
	if got := post(); got != http.StatusUnauthorized {
		t.Errorf("replayed login = %d, want 401 — the code was not consumed", got)
	}
}

// A database that is down is not a wrong code. Answering 401 would tell someone
// their code was rejected when it was never read.
func TestLoginHandlerStoreErrorIsNotARejection(t *testing.T) {
	user := &model.User{UserID: "u1", Email: "a@b.c", Name: "Alice"}
	mux := http.NewServeMux()
	New(Config{
		Users:      &mock.MockUserStore{ByEmail: map[string]*model.User{"a@b.c": user}},
		LoginCodes: &mock.MockLoginCodeStore{ConsumeErr: errors.New("database is down")},
		JWTSecret:  "test-jwt-secret",
	}).Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/login",
		bytes.NewBufferString(`{"email":"a@b.c","otp":"code-1"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("token")) {
		t.Error("a store failure issued a token")
	}
}
