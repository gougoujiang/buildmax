package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"buildmax/internal/infra/db"
	"buildmax/internal/testutil"
)

func TestOtpRequestHandler(t *testing.T) {
	userExists := &db.User{UserID: "u1", Email: "a@b.c", Name: "Alice"}

	tests := []struct {
		name        string
		userStore   db.UserStore
		body        string
		wantStatus  int
		wantBodyHas string
	}{
		{
			name:        "signup new user returns 200",
			userStore:   &testutil.MockUserStore{ByEmail: map[string]*db.User{}},
			body:        `{"email":"new@example.com","intent":"signup"}`,
			wantStatus:  http.StatusOK,
			wantBodyHas: "otp_sent",
		},
		{
			name:        "signup existing email returns 409",
			userStore:   &testutil.MockUserStore{ByEmail: map[string]*db.User{"a@b.c": userExists}},
			body:        `{"email":"a@b.c","intent":"signup"}`,
			wantStatus:  http.StatusConflict,
			wantBodyHas: "email already registered",
		},
		{
			name:        "login unknown email returns 404",
			userStore:   &testutil.MockUserStore{ByEmail: map[string]*db.User{}},
			body:        `{"email":"nobody@example.com","intent":"login"}`,
			wantStatus:  http.StatusNotFound,
			wantBodyHas: "user not found",
		},
		{
			name:        "login existing user returns 200",
			userStore:   &testutil.MockUserStore{ByEmail: map[string]*db.User{"a@b.c": userExists}},
			body:        `{"email":"a@b.c","intent":"login"}`,
			wantStatus:  http.StatusOK,
			wantBodyHas: "otp_sent",
		},
		{
			name:        "default intent signup creates user",
			userStore:   &testutil.MockUserStore{ByEmail: map[string]*db.User{}},
			body:        `{"email":"default@example.com"}`,
			wantStatus:  http.StatusOK,
			wantBodyHas: "otp_sent",
		},
		{
			name:        "empty email returns 400",
			userStore:   &testutil.MockUserStore{ByEmail: map[string]*db.User{}},
			body:        `{"email":"","intent":"signup"}`,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "email required",
		},
		{
			name:       "invalid body returns 400",
			userStore:  &testutil.MockUserStore{ByEmail: map[string]*db.User{}},
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			NewHandler(Config{UserStore: tt.userStore, JWTSecret: "", DefaultQuotaTier: "free_trial"}).Register(mux)
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
