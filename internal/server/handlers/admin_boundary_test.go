package handlers

// What an administrator still cannot do, and what disabling still stops.
//
// Each of these drives an admin route and then a team-scoped, login, or webhook
// route, so it spans two packages. They live with the routes that must stay
// shut rather than with the ones that grant, because that is the side a
// regression would appear on.

import (
	"context"
	"github.com/gougoujiang/buildmax/internal/service/conversation"
	convchannel "github.com/gougoujiang/buildmax/internal/service/conversation/channel"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

const (
	adminUser      = "u_boundary_admin"
	boundaryTeam   = "tm_boundary"
	boundarySecret = "boundary-secret"
)

// boundaryFixture registers the whole surface -- admin routes and team routes on
// one mux -- because that is the only way to assert that passing through one
// does not open the other.
type boundaryFixture struct {
	mux    *http.ServeMux
	users  *mock.MockUserStore
	codes  *mock.MockLoginCodeStore
	keys   *mock.MockUserWebhookKeyStore
	target *model.User
}

func newBoundaryFixture(t *testing.T) *boundaryFixture {
	t.Helper()
	users := &mock.MockUserStore{}
	target, err := users.CreateUser(context.Background(), "target@example.com", "free")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	grants := &mock.MockSystemGrantStore{}
	grants.GrantForTest(adminUser, model.SystemRoleAdmin)
	audits := &mock.MockAuditStore{}
	codes := &mock.MockLoginCodeStore{}
	keys := &mock.MockUserWebhookKeyStore{}

	// A team the administrator is not a member of. Membership, not the grant,
	// is what these routes must ask for.
	teams := &mock.MockTeamStore{
		Teams:   []model.Team{{TeamID: boundaryTeam, Name: "Boundary", CreatedBy: target.UserID}},
		Members: []model.TeamMember{{TeamID: boundaryTeam, UserID: target.UserID, Role: model.TeamRoleOwner}},
	}

	h := NewHandler(Config{
		JWTSecret:           boundarySecret,
		UserStore:           users,
		SystemGrantStore:    grants,
		TeamStore:           teams,
		AuditStore:          audits,
		UserWebhookKeyStore: keys,
		LoginCodeStore:      codes,
		RefreshTokenStore:   &mock.MockRefreshTokenStore{},
		IssueStore:          &mock.MockIssueStore{},
		ConversationStore:   &mock.MockConversationStore{},
		WebhookEngine:       stubTurnEngine{},
		Audit:               audit.NewRecorder(audits),
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return &boundaryFixture{mux: mux, users: users, codes: codes, keys: keys, target: target}
}

func (f *boundaryFixture) do(t *testing.T, method, path, userID, body string) *httptest.ResponseRecorder {
	t.Helper()
	if body == "" {
		body = "{}"
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(userID, boundarySecret))
	}
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func TestAdminTeamRoutesAreNotAWayIntoATeam(t *testing.T) {
	f := newBoundaryFixture(t)
	for _, path := range []string{
		"/api/teams/tm_shared/issues",
		"/api/teams/tm_shared/conversations",
		"/api/teams/tm_shared/audit-events",
	} {
		if got := f.do(t, "GET", path, adminUser, "").Code; got != http.StatusForbidden {
			t.Errorf("%s for a non-member administrator got %d, want 403", path, got)
		}
	}
}
func TestDisableStopsTheAccessTokenOnTheNextRequest(t *testing.T) {
	f := newBoundaryFixture(t)

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
func TestDisabledAccountCannotLogIn(t *testing.T) {
	f := newBoundaryFixture(t)
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

func TestSystemGrantIsNotATeamKey(t *testing.T) {
	grants := &mock.MockSystemGrantStore{}
	grants.GrantForTest(adminUser, model.SystemRoleAdmin)

	mux := matrixMuxWithGrants(t, grants)
	for _, c := range teamRoutes {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			if got := requestAs(t, mux, c, matrixTeam, adminUser); got != http.StatusForbidden {
				t.Errorf("a system administrator who is not a member got %d, want 403", got)
			}
		})
	}
}

// TestAdminDenialIsRecorded: a refused admin request is written to the trail
// with no team, because the route was not team-scoped. A denial is what shows
// someone probing at a boundary, and an admin boundary is worth seeing probed.

// TestDisabledAccountCannotLogIn covers the front door, and the reason the
// check is after the credential verifies rather than before it.
func TestDisabledAccountsWebhookKeyIsRefused(t *testing.T) {
	f := newBoundaryFixture(t)
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

// stubTurnEngine satisfies the webhook route's engine dependency. It is never
// reached: the requests in this file are refused before a turn is processed,
// which is the point.
type stubTurnEngine struct{}

func (stubTurnEngine) Process(_ context.Context, _, _ string, _ convchannel.Turn) (conversation.ConversationResult, error) {
	return conversation.ConversationResult{}, nil
}
