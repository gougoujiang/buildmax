package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"buildmax/internal/store"
)

// mockUserStore is an in-memory UserStore for tests.
type mockUserStore struct {
	userByEmail map[string]*store.User
}

func (m *mockUserStore) UserByEmail(ctx context.Context, email string) (*store.User, error) {
	return m.userByEmail[email], nil
}

func TestLoginHandler(t *testing.T) {
	secret := "test-jwt-secret"
	userExists := &store.User{UserID: "u1", Email: "a@b.c", Name: "Alice"}

	tests := []struct {
		name           string
		userStore      store.UserStore
		jwtSecret      string
		body           string
		wantStatus     int
		wantBodyHas    string
		wantBodyNotHas string
	}{
		{
			name:       "user found returns 200 and token",
			userStore:  &mockUserStore{userByEmail: map[string]*store.User{"a@b.c": userExists}},
			jwtSecret:  secret,
			body:       `{"email":"a@b.c"}`,
			wantStatus: http.StatusOK,
			wantBodyHas: "token",
		},
		{
			name:       "user not found returns 401",
			userStore:  &mockUserStore{userByEmail: map[string]*store.User{}},
			jwtSecret:  secret,
			body:       `{"email":"nobody@example.com"}`,
			wantStatus: http.StatusUnauthorized,
			wantBodyHas: "user not found",
		},
		{
			name:       "invalid body returns 400",
			userStore:  &mockUserStore{userByEmail: map[string]*store.User{}},
			jwtSecret:  secret,
			body:       `{`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty email returns 400",
			userStore:  &mockUserStore{userByEmail: map[string]*store.User{}},
			jwtSecret:  secret,
			body:       `{"email":""}`,
			wantStatus: http.StatusBadRequest,
			wantBodyHas: "email required",
		},
		{
			name:       "no UserStore returns 503",
			userStore:  nil,
			jwtSecret:  secret,
			body:       `{"email":"a@b.c"}`,
			wantStatus: http.StatusServiceUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(Config{
				UserStore: tt.userStore,
				JWTSecret: tt.jwtSecret,
			})
			req := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
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
