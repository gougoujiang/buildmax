package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"buildmax/internal/core/model"
	"buildmax/internal/mock"
)

func TestLoginHandler(t *testing.T) {
	secret := "test-jwt-secret"
	userExists := &model.User{UserID: "u1", Email: "a@b.c", Name: "Alice"}

	tests := []struct {
		name           string
		userStore      model.UserStore
		jwtSecret      string
		body           string
		wantStatus     int
		wantBodyHas    string
		wantBodyNotHas string
	}{
		{
			name:        "user found with valid otp returns 200 and token",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{"a@b.c": userExists}},
			jwtSecret:   secret,
			body:        `{"email":"a@b.c","otp":"123456"}`,
			wantStatus:  http.StatusOK,
			wantBodyHas: "token",
		},
		{
			name:        "user found but wrong otp returns 401",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{"a@b.c": userExists}},
			jwtSecret:   secret,
			body:        `{"email":"a@b.c","otp":"000000"}`,
			wantStatus:  http.StatusUnauthorized,
			wantBodyHas: "invalid otp",
		},
		{
			name:        "user found but missing otp returns 400",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{"a@b.c": userExists}},
			jwtSecret:   secret,
			body:        `{"email":"a@b.c"}`,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "otp required",
		},
		{
			name:        "user not found returns 401",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			jwtSecret:   secret,
			body:        `{"email":"nobody@example.com","otp":"123456"}`,
			wantStatus:  http.StatusUnauthorized,
			wantBodyHas: "user not found",
		},
		{
			name:       "invalid body returns 400",
			userStore:  &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			jwtSecret:  secret,
			body:       `{`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:        "empty email returns 400",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			jwtSecret:   secret,
			body:        `{"email":""}`,
			wantStatus:  http.StatusBadRequest,
			wantBodyHas: "email required",
		},
		{
			name:       "no UserStore returns 503",
			userStore:  nil,
			jwtSecret:  secret,
			body:       `{"email":"a@b.c","otp":"123456"}`,
			wantStatus: http.StatusServiceUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			NewHandler(Config{
				UserStore:        tt.userStore,
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
		body        string
		wantStatus  int
		wantBodyHas string
	}{
		{
			name:        "signup new user returns 200",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			body:        `{"email":"new@example.com","intent":"signup"}`,
			wantStatus:  http.StatusOK,
			wantBodyHas: "otp_sent",
		},
		{
			name:        "signup existing email returns 409",
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
			wantBodyHas: "otp_sent",
		},
		{
			name:        "default intent signup creates user",
			userStore:   &mock.MockUserStore{ByEmail: map[string]*model.User{}},
			body:        `{"email":"default@example.com"}`,
			wantStatus:  http.StatusOK,
			wantBodyHas: "otp_sent",
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
