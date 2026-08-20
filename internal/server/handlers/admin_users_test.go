package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/service/conversation"
	convchannel "github.com/gougoujiang/buildmax/internal/service/conversation/channel"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

// stubTurnEngine satisfies the webhook route's engine dependency. It is never
// reached: the requests in this file are refused before a turn is processed,
// which is the point.
type stubTurnEngine struct{}

func (stubTurnEngine) Process(_ context.Context, _, _ string, _ convchannel.Turn) (conversation.ConversationResult, error) {
	return conversation.ConversationResult{}, nil
}

// disableFixture is a deployment with one administrator and one ordinary
// account, wired with the stores disablement actually touches.
type disableFixture struct {
	mux      *http.ServeMux
	users    *mock.MockUserStore
	sessions *mock.MockRefreshTokenStore
	codes    *mock.MockLoginCodeStore
	keys     *mock.MockUserWebhookKeyStore
	audits   *mock.MockAuditStore
	admin    *model.User
	target   *model.User
}

func newDisableFixture(t *testing.T) *disableFixture {
	t.Helper()
	users := &mock.MockUserStore{}
	grants := &mock.MockSystemGrantStore{}
	grants.GrantForTest(adminUser, model.SystemRoleAdmin)
	// Two, so revoking one is not the last-grant case.
	grants.GrantForTest("u_second_admin", model.SystemRoleAdmin)

	f := &disableFixture{
		users:    users,
		sessions: &mock.MockRefreshTokenStore{},
		codes:    &mock.MockLoginCodeStore{},
		keys:     &mock.MockUserWebhookKeyStore{},
		audits:   &mock.MockAuditStore{},
	}
	f.admin = seedUser(t, users, adminUser, "admin@example.com")
	f.target = seedUser(t, users, "u_target", "target@example.com")

	h := NewHandler(Config{
		JWTSecret:           matrixSecret,
		SystemGrantStore:    grants,
		UserStore:           users,
		TeamStore:           &mock.MockTeamStore{},
		LoginCodeStore:      f.codes,
		RefreshTokenStore:   f.sessions,
		UserWebhookKeyStore: f.keys,
		// Present so the webhook route reaches its credential check rather
		// than answering "not configured" first.
		WebhookEngine: stubTurnEngine{},
		AuditStore:    f.audits,
		Audit:         audit.NewRecorder(f.audits),
		WorkspacesDir: t.TempDir(),
	})
	f.mux = http.NewServeMux()
	h.Register(f.mux)
	return f
}

func (f *disableFixture) do(t *testing.T, method, path, userID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(userID, matrixSecret))
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func (f *disableFixture) actions() []string {
	out := make([]string, 0, len(f.audits.Events))
	for _, e := range f.audits.Events {
		out = append(out, e.Action)
	}
	return out
}

// TestDisableStopsTheAccessTokenOnTheNextRequest is the claim that makes
// disablement mean anything.
//
// The access token is a signed JWT the server never stores, so it cannot be
// retired — waiting it out would make "disable" mean "in about a week". The
// check lives where the identity is resolved, and this test is what says the
// token stops working now rather than at expiry.
func TestDisableStopsTheAccessTokenOnTheNextRequest(t *testing.T) {
	f := newDisableFixture(t)

	// The target can reach an authenticated route before the disable. The
	// route's own answer does not matter; not being refused does.
	if got := f.do(t, "GET", "/api/webhook-keys", f.target.UserID, "").Code; got == http.StatusForbidden {
		t.Fatalf("setup: an enabled account was refused with %d", got)
	}

	rec := f.do(t, "POST", "/api/admin/users/"+f.target.UserID+"/disable", adminUser, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("disable got %d: %s", rec.Code, rec.Body.String())
	}

	after := f.do(t, "GET", "/api/webhook-keys", f.target.UserID, "")
	if after.Code != http.StatusForbidden {
		t.Errorf("a disabled account's access token got %d, want 403", after.Code)
	}
	if !strings.Contains(after.Body.String(), "account_disabled") {
		t.Errorf("the refusal should say why, got %s", after.Body.String())
	}

	// And enabling brings it back. Nothing else is restored — that is what
	// section 8 means by "enabling reverses the state and nothing else".
	if got := f.do(t, "POST", "/api/admin/users/"+f.target.UserID+"/enable", adminUser, "").Code; got != http.StatusOK {
		t.Fatalf("enable got %d", got)
	}
	if got := f.do(t, "GET", "/api/webhook-keys", f.target.UserID, "").Code; got == http.StatusForbidden {
		t.Errorf("an enabled account is still refused: %d", got)
	}
}

// TestDisableRevokesSessionsAndRefusesRefresh covers the credential the server
// can actually retire, and the one route that would otherwise mint a new access
// token for a disabled account.
func TestDisableRevokesSessionsAndRefusesRefresh(t *testing.T) {
	f := newDisableFixture(t)
	plaintext, _, err := f.sessions.CreateRefreshToken(t.Context(), model.NewRefreshToken{
		UserID: f.target.UserID, SessionID: "as_one", Platform: "portal", TTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}

	rec := f.do(t, "POST", "/api/admin/users/"+f.target.UserID+"/disable", adminUser, "")
	var body struct {
		SessionsRevoked int64 `json:"sessions_revoked"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if body.SessionsRevoked != 1 {
		t.Errorf("sessions_revoked = %d, want 1", body.SessionsRevoked)
	}

	refresh := f.do(t, "POST", "/api/token/refresh", "", `{"refresh_token":"`+plaintext+`"}`)
	if refresh.Code == http.StatusOK {
		t.Errorf("a disabled account refreshed into a new access token: %s", refresh.Body.String())
	}
}

// TestDisabledAccountCannotLogIn covers the front door, and the reason the
// check is after the credential verifies rather than before it.
func TestDisabledAccountCannotLogIn(t *testing.T) {
	f := newDisableFixture(t)
	f.codes.Codes = map[string]*mock.MockLoginCode{
		"code-1": {UserID: f.target.UserID, ExpiresAt: time.Now().Add(time.Hour).Unix()},
	}
	f.users.DisableForTest(f.target.UserID, 1)

	rec := f.do(t, "POST", "/api/login", "", `{"email":"`+f.target.Email+`","otp":"code-1"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("login got %d, want 403: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "account_disabled") {
		t.Errorf("someone who proved the account is theirs should hear why, got %s", rec.Body.String())
	}

	// A wrong credential on the same disabled account still gets the generic
	// answer, so the endpoint does not become a way to ask which addresses are
	// registered.
	wrong := f.do(t, "POST", "/api/login", "", `{"email":"`+f.target.Email+`","otp":"not-a-code"}`)
	if strings.Contains(wrong.Body.String(), "account_disabled") {
		t.Errorf("a wrong code revealed the account state: %s", wrong.Body.String())
	}
}

// TestDisabledAccountsWebhookKeyIsRefused: a webhook key is a credential the
// account holds, so disabling refuses it too — without revoking the key, so
// enabling brings the integration back.
func TestDisabledAccountsWebhookKeyIsRefused(t *testing.T) {
	f := newDisableFixture(t)
	f.keys.Keys = map[string]string{"whk-plain": f.target.UserID}
	f.users.DisableForTest(f.target.UserID, 1)

	req := httptest.NewRequest("POST", "/api/webhook", strings.NewReader(`{"message":"hi"}`))
	req.Header.Set("X-Webhook-Key", "whk-plain")
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("webhook with a disabled owner's key got %d, want 403: %s", rec.Code, rec.Body.String())
	}
	if len(f.keys.Keys) != 1 {
		t.Errorf("the key itself must not be revoked: %+v", f.keys.Keys)
	}
}

// TestAdminCannotDisableThemselves: the mistake is easy to make, impossible to
// undo through the API, and pointless to allow.
func TestAdminCannotDisableThemselves(t *testing.T) {
	f := newDisableFixture(t)
	rec := f.do(t, "POST", "/api/admin/users/"+adminUser+"/disable", adminUser, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("self-disable got %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if f.users.ByID[adminUser].Disabled() {
		t.Error("the administrator disabled themselves anyway")
	}
}

// TestLoginCodeForADisabledAccountIsRefused: issuing a way in that opens
// nothing would read to the operator as "they can sign in now".
func TestLoginCodeForADisabledAccountIsRefused(t *testing.T) {
	f := newDisableFixture(t)
	f.users.DisableForTest(f.target.UserID, 1)

	rec := f.do(t, "POST", "/api/admin/users/"+f.target.UserID+"/login-code", adminUser, "")
	if rec.Code != http.StatusConflict {
		t.Errorf("got %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminAccountActionsAreRecorded: every privileged action names the person
// who took it, not the binary — the caller proved who they are.
func TestAdminAccountActionsAreRecorded(t *testing.T) {
	f := newDisableFixture(t)

	f.do(t, "POST", "/api/admin/users", adminUser, `{"email":"new@example.com"}`)
	f.do(t, "POST", "/api/admin/users/"+f.target.UserID+"/login-code", adminUser, "")
	f.do(t, "POST", "/api/admin/users/"+f.target.UserID+"/disable", adminUser, "")
	f.do(t, "POST", "/api/admin/users/"+f.target.UserID+"/enable", adminUser, "")
	f.do(t, "DELETE", "/api/admin/users/"+f.target.UserID+"/sessions", adminUser, "")

	want := []string{
		model.AuditUserCreated,
		model.AuditLoginCodeIssued,
		model.AuditUserDisabled,
		model.AuditUserEnabled,
		model.AuditSessionsRevoked,
	}
	got := f.actions()
	if len(got) != len(want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("action %d = %q, want %q", i, got[i], want[i])
		}
	}
	for _, e := range f.audits.Events {
		if e.ActorType != model.AuditActorUser || e.ActorID != adminUser {
			t.Errorf("event should name the administrator: %+v", e)
		}
		if e.TeamID != "" {
			t.Errorf("an account action is not team-scoped: %+v", e)
		}
	}
}

// TestAdminResponsesCarryNoSecrets is item 5 of the design's authorization
// matrix. It is crude on purpose: it catches the realistic failure, which is
// someone returning a row struct instead of a response struct.
func TestAdminResponsesCarryNoSecrets(t *testing.T) {
	f := newDisableFixture(t)
	f.users.ByID[f.target.UserID].HasPassword = true

	for _, path := range []string{
		"/api/admin/me",
		"/api/admin/users",
		"/api/admin/users/" + f.target.UserID,
		"/api/admin/grants",
	} {
		rec := f.do(t, "GET", path, adminUser, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s got %d: %s", path, rec.Code, rec.Body.String())
		}
		body := strings.ToLower(rec.Body.String())
		for _, forbidden := range []string{"password_hash", "api_key", "token_hash", "secret", "code_hash"} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s leaked %q: %s", path, forbidden, rec.Body.String())
			}
		}
	}
}

// TestAdminUserDetailShowsTeamsWithoutContents: an administrator learns that an
// account can reach a team, never what is in it.
func TestAdminUserDetailShowsTeamsWithoutContents(t *testing.T) {
	f := newDisableFixture(t)
	rec := f.do(t, "GET", "/api/admin/users/"+f.target.UserID, adminUser, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	var detail AdminUserDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.UserID != f.target.UserID {
		t.Errorf("user_id = %q", detail.UserID)
	}
	if detail.Teams == nil || detail.SystemRoles == nil {
		t.Errorf("empty collections should serialize as [], not null: %+v", detail)
	}
}

// TestAdminUserRoutesOnAnUnknownAccount: 404, not a 500 and not a silent
// success on a user id nobody has.
func TestAdminUserRoutesOnAnUnknownAccount(t *testing.T) {
	f := newDisableFixture(t)
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/admin/users/u_nobody"},
		{"POST", "/api/admin/users/u_nobody/disable"},
		{"POST", "/api/admin/users/u_nobody/login-code"},
		{"DELETE", "/api/admin/users/u_nobody/sessions"},
	} {
		if got := f.do(t, tc.method, tc.path, adminUser, "").Code; got != http.StatusNotFound {
			t.Errorf("%s %s got %d, want 404", tc.method, tc.path, got)
		}
	}
}
