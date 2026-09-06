package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientListSystemGrants(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/grants" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("include_revoked") != "true" {
			t.Errorf("include_revoked not forwarded: %s", r.URL.RawQuery)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"grants":[{"id":"sg_1","user_id":"u_1","role":"system_admin","granted_by":"u_0","granted_at":"2026-09-06T00:00:00Z","email":"a@b.com"}]}`))
	}))
	defer srv.Close()

	grants, err := NewClient(srv.URL).ListSystemGrants(context.Background(), "tok", true)
	if err != nil {
		t.Fatalf("ListSystemGrants: %v", err)
	}
	if len(grants) != 1 || grants[0].Email != "a@b.com" || !grants[0].Active() {
		t.Fatalf("unexpected grants: %+v", grants)
	}
}

func TestClientGrantSystemRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/grants" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["user_id"] != "u_1" {
			t.Errorf("user_id = %q", body["user_id"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"sg_1","user_id":"u_1","role":"system_admin","granted_by":"u_0","granted_at":"2026-09-06T00:00:00Z","email":"a@b.com"}`))
	}))
	defer srv.Close()

	grant, err := NewClient(srv.URL).GrantSystemRole(context.Background(), "tok", "u_1", "")
	if err != nil {
		t.Fatalf("GrantSystemRole: %v", err)
	}
	if grant.Role != "system_admin" || grant.Email != "a@b.com" {
		t.Fatalf("unexpected grant: %+v", grant)
	}
}

func TestClientRevokeSystemRoleSurfacesLastHolder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/grants/u_1" || r.Method != http.MethodDelete {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"this is the deployment's last system_admin; revoke it with ` + "`buildmax-server admin revoke <email>`" + `"}`))
	}))
	defer srv.Close()

	err := NewClient(srv.URL).RevokeSystemRole(context.Background(), "tok", "u_1", "")
	if err == nil {
		t.Fatal("expected the last-holder refusal to surface")
	}
	if got := err.Error(); got != "server 409: this is the deployment's last system_admin; revoke it with `buildmax-server admin revoke <email>`" {
		t.Fatalf("unexpected error: %q", got)
	}
}

func TestClientRevokeSystemRoleSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).RevokeSystemRole(context.Background(), "tok", "u_1", ""); err != nil {
		t.Fatalf("RevokeSystemRole: %v", err)
	}
}

func TestClientFindAccountByEmail(t *testing.T) {
	// The server search is a substring match; two accounts come back and the
	// client must pick the exact address, not the first row.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "sam@b.com" {
			t.Errorf("q = %q", r.URL.Query().Get("q"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[{"id":"u_2","email":"samuel@b.com"},{"id":"u_1","email":"sam@b.com"}],"total":2}`))
	}))
	defer srv.Close()

	acct, err := NewClient(srv.URL).FindAccountByEmail(context.Background(), "tok", "sam@b.com")
	if err != nil {
		t.Fatalf("FindAccountByEmail: %v", err)
	}
	if acct.ID != "u_1" {
		t.Fatalf("resolved to %+v, want the exact-match account u_1", acct)
	}
}

func TestClientFindAccountByEmailNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[{"id":"u_2","email":"samuel@b.com"}],"total":1}`))
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL).FindAccountByEmail(context.Background(), "tok", "sam@b.com"); err == nil {
		t.Fatal("expected no-match to be an error, not the substring row")
	}
}
