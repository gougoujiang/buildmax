package space

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	corespace "github.com/gougoujiang/buildmax/internal/core/space"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

// spaceAuditExport drives the space-scoped export as one user.
//
// It builds its own mux rather than borrowing the authorization matrix's,
// because this test is about what the response body contains and the matrix's
// mux is deliberately seeded with an empty trail.
func spaceAuditExport(t *testing.T, audits *mock.MockAuditStore, spaceID, userID string) *httptest.ResponseRecorder {
	t.Helper()
	spaces := &mock.MockSpaceStore{
		Spaces: []corespace.Space{
			{ID: matrixSpace, Name: "Matrix", CreatedBy: matrixOwner},
			{ID: matrixOther, Name: "Other", CreatedBy: matrixOutside},
		},
		Members: []corespace.Member{
			{SpaceID: matrixSpace, UserID: matrixOwner, Role: corespace.RoleOwner},
			{SpaceID: matrixOther, UserID: matrixOutside, Role: corespace.RoleOwner},
		},
	}
	h := New(Config{
		JWTSecret: matrixSecret,
		Spaces:    spaces,
		Users:     &mock.MockUserStore{},
		Audits:    audits,
		Audit:     audit.NewRecorder(audits),
	})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest("GET", "/api/spaces/"+spaceID+"/audit-events/export?format=jsonl", nil)
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(userID, matrixSecret))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// exportBody runs an admin export and returns the raw response.
func TestSpaceAuditExportStaysInsideTheSpace(t *testing.T) {
	audits := &mock.MockAuditStore{Events: []coreaudit.Event{
		{ID: "ae_1", SpaceID: matrixSpace, ActorType: coreaudit.ActorUser, ActorID: matrixOwner, Action: coreaudit.SpaceMemberAdded, CreatedAt: time.Unix(100, 0).UTC()},
		{ID: "ae_2", SpaceID: matrixOther, ActorType: coreaudit.ActorUser, ActorID: "u_elsewhere", Action: coreaudit.SpaceMemberAdded, CreatedAt: time.Unix(200, 0).UTC()},
		{ID: "ae_3", ActorType: coreaudit.ActorUser, ActorID: matrixOwner, Action: coreaudit.UserLogin, CreatedAt: time.Unix(300, 0).UTC()},
	}}
	rec := spaceAuditExport(t, audits, matrixSpace, matrixOwner)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}

	count := 0
	for line := range strings.SplitSeq(strings.TrimSpace(rec.Body.String()), "\n") {
		if line == "" {
			continue
		}
		var event coreaudit.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse jsonl: %v", err)
		}
		if event.SpaceID != matrixSpace {
			t.Errorf("exported an event scoped to %q", event.SpaceID)
		}
		count++
	}
	// One event, not three: the other space's is out of scope, and so is the
	// login, which has no space and belongs to the deployment-scoped read.
	if count != 1 {
		t.Fatalf("exported %d events, want 1", count)
	}
}
