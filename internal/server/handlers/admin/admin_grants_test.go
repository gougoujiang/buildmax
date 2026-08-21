package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// TestGrantRoundTripThroughTheAPI: an administrator can hand the authority to
// someone else and take it back, and both ends of that are in the trail naming
// who did it.
func TestGrantRoundTripThroughTheAPI(t *testing.T) {
	f := newDisableFixture(t)

	created := f.do(t, "POST", "/api/admin/grants", adminUser, `{"user_id":"`+f.target.UserID+`"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("grant got %d: %s", created.Code, created.Body.String())
	}
	var grant AdminGrant
	if err := json.Unmarshal(created.Body.Bytes(), &grant); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if grant.GrantedBy != adminUser {
		t.Errorf("granted_by = %q, want the calling administrator", grant.GrantedBy)
	}
	if grant.Email != f.target.Email {
		t.Errorf("email = %q, want %q", grant.Email, f.target.Email)
	}

	// The new administrator can reach the area immediately: the check is a
	// live read, not something resolved when their token was signed.
	if got := f.do(t, "GET", "/api/admin/me", f.target.UserID, "").Code; got != http.StatusOK {
		t.Errorf("the new administrator got %d on /me, want 200", got)
	}

	// Granting twice is a conflict rather than a second row.
	if got := f.do(t, "POST", "/api/admin/grants", adminUser, `{"user_id":"`+f.target.UserID+`"}`).Code; got != http.StatusConflict {
		t.Errorf("re-grant got %d, want 409", got)
	}

	if got := f.do(t, "DELETE", "/api/admin/grants/"+f.target.UserID, adminUser, "").Code; got != http.StatusNoContent {
		t.Errorf("revoke got %d, want 204", got)
	}
	if got := f.do(t, "GET", "/api/admin/me", f.target.UserID, "").Code; got != http.StatusForbidden {
		t.Errorf("a revoked administrator got %d, want 403", got)
	}

	// The grant, the revocation, and then the refused request the revoked
	// administrator made — which is the trail doing its job, not noise: a
	// denial is what shows someone reaching for authority they no longer have.
	want := []string{model.AuditSystemAdminGranted, model.AuditSystemAdminRevoked, model.AuditAccessDenied}
	got := f.actions()
	if len(got) != len(want) {
		t.Fatalf("actions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("action %d = %q, want %q", i, got[i], want[i])
		}
	}
	for _, e := range f.audits.Events[:2] {
		if e.ActorID != adminUser || e.ActorType != model.AuditActorUser {
			t.Errorf("a grant made through the API names the administrator: %+v", e)
		}
	}
	if denied := f.audits.Events[2]; denied.ActorID != f.target.UserID {
		t.Errorf("the denial should name whoever was refused, got %+v", denied)
	}
}

// TestAPIRefusesToRevokeTheLastGrant is the asymmetry in section 6: nobody
// should be able to leave a deployment unadministerable by clicking. The
// operator command allows it, because it is the recovery path and its caller
// already holds the database credentials.
func TestAPIRefusesToRevokeTheLastGrant(t *testing.T) {
	f := newDisableFixture(t)

	// The fixture starts with two administrators. Revoking one is fine.
	if got := f.do(t, "DELETE", "/api/admin/grants/u_second_admin", adminUser, "").Code; got != http.StatusNoContent {
		t.Fatalf("revoking the second administrator got %d, want 204", got)
	}
	// Revoking the last one is not, including one's own.
	rec := f.do(t, "DELETE", "/api/admin/grants/"+adminUser, adminUser, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("revoking the last grant got %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "buildmax-server admin revoke") {
		t.Errorf("the refusal should name the command that can do it, got %s", rec.Body.String())
	}
	if got := f.do(t, "GET", "/api/admin/me", adminUser, "").Code; got != http.StatusOK {
		t.Errorf("the last administrator lost access anyway: %d", got)
	}
}

// TestGrantToAnUnknownAccountIsRefused: granting does not create an account,
// for the same reason the operator command does not.
func TestGrantToAnUnknownAccountIsRefused(t *testing.T) {
	f := newDisableFixture(t)
	if got := f.do(t, "POST", "/api/admin/grants", adminUser, `{"user_id":"u_nobody"}`).Code; got != http.StatusNotFound {
		t.Errorf("got %d, want 404", got)
	}
	if got := f.do(t, "POST", "/api/admin/grants", adminUser, `{"user_id":"`+f.target.UserID+`","role":"system_observer"}`).Code; got != http.StatusBadRequest {
		t.Errorf("an undefined role got %d, want 400", got)
	}
}

// TestRevokingARoleNobodyHoldsIs404 rather than a silent success: an operator
// who mistypes a user id should not be told the authority was removed.
func TestRevokingARoleNobodyHoldsIs404(t *testing.T) {
	f := newDisableFixture(t)
	if got := f.do(t, "DELETE", "/api/admin/grants/u_nobody", adminUser, "").Code; got != http.StatusNotFound {
		t.Errorf("got %d, want 404", got)
	}
}
