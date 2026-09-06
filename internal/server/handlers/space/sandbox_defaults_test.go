package space

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corespace "github.com/gougoujiang/buildmax/internal/core/space"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

const sandboxDefaultsSecret = "sandbox-defaults-test-secret"

func newSandboxDefaultsFixture(t *testing.T) (*http.ServeMux, string) {
	t.Helper()
	spaceID := "tm_1"
	spaces := &mock.MockSpaceStore{
		Spaces: []corespace.Space{{ID: spaceID, Name: "Space", CreatedBy: "u_owner"}},
		Members: []corespace.Member{
			{SpaceID: spaceID, UserID: "u_owner", Role: corespace.RoleOwner},
			{SpaceID: spaceID, UserID: "u_member", Role: corespace.RoleMember},
		},
	}
	h := New(Config{JWTSecret: sandboxDefaultsSecret, Spaces: spaces})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, spaceID
}

func callSandboxDefaults(t *testing.T, mux *http.ServeMux, method, userID, spaceID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/spaces/"+spaceID+"/sandbox-defaults", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(userID, sandboxDefaultsSecret))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestSandboxDefaultsRoundTripsAndValidates(t *testing.T) {
	mux, spaceID := newSandboxDefaultsFixture(t)

	rec := callSandboxDefaults(t, mux, http.MethodGet, "u_member", spaceID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get before any default set: status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got sandboxDefaultsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.NetworkTier != "" || got.FilesystemTier != "" {
		t.Errorf("defaults before any set = %+v, want empty", got)
	}

	rec = callSandboxDefaults(t, mux, http.MethodPut, "u_owner", spaceID, `{"sandbox_network_tier":"registries","sandbox_filesystem_tier":"workspace"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner set: status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	rec = callSandboxDefaults(t, mux, http.MethodGet, "u_member", spaceID, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.NetworkTier != "registries" || got.FilesystemTier != "workspace" {
		t.Errorf("defaults after set = %+v, want registries/workspace", got)
	}

	rec = callSandboxDefaults(t, mux, http.MethodPut, "u_member", spaceID, `{"sandbox_network_tier":"open"}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("member set: status = %d, want 403", rec.Code)
	}

	rec = callSandboxDefaults(t, mux, http.MethodPut, "u_owner", spaceID, `{"sandbox_network_tier":"not-a-tier"}`)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Errorf("invalid tier: status = %d, want a 4xx refusal", rec.Code)
	}
}
