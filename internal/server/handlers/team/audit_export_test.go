package team

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/audit"
	"github.com/gougoujiang/buildmax/internal/testsupport"
)

// teamAuditExport drives the team-scoped export as one user.
//
// It builds its own mux rather than borrowing the authorization matrix's,
// because this test is about what the response body contains and the matrix's
// mux is deliberately seeded with an empty trail.
func teamAuditExport(t *testing.T, audits *mock.MockAuditStore, teamID, userID string) *httptest.ResponseRecorder {
	t.Helper()
	teams := &mock.MockTeamStore{
		Teams: []coreteam.Team{
			{ID: matrixTeam, Name: "Matrix", CreatedBy: matrixOwner},
			{ID: matrixOther, Name: "Other", CreatedBy: matrixOutside},
		},
		Members: []coreteam.Member{
			{TeamID: matrixTeam, UserID: matrixOwner, Role: coreteam.RoleOwner},
			{TeamID: matrixOther, UserID: matrixOutside, Role: coreteam.RoleOwner},
		},
	}
	h := New(Config{
		JWTSecret: matrixSecret,
		Teams:     teams,
		Users:     &mock.MockUserStore{},
		Audits:    audits,
		Audit:     audit.NewRecorder(audits),
	})
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest("GET", "/api/teams/"+teamID+"/audit-events/export?format=jsonl", nil)
	req.Header.Set("Authorization", "Bearer "+testsupport.SignJWT(userID, matrixSecret))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// exportBody runs an admin export and returns the raw response.
func TestTeamAuditExportStaysInsideTheTeam(t *testing.T) {
	audits := &mock.MockAuditStore{Events: []model.AuditEvent{
		{ID: "ae_1", TeamID: matrixTeam, ActorType: model.AuditActorUser, ActorID: matrixOwner, Action: model.AuditTeamMemberAdded, CreatedAt: time.Unix(100, 0).UTC()},
		{ID: "ae_2", TeamID: matrixOther, ActorType: model.AuditActorUser, ActorID: "u_elsewhere", Action: model.AuditTeamMemberAdded, CreatedAt: time.Unix(200, 0).UTC()},
		{ID: "ae_3", ActorType: model.AuditActorUser, ActorID: matrixOwner, Action: model.AuditUserLogin, CreatedAt: time.Unix(300, 0).UTC()},
	}}
	rec := teamAuditExport(t, audits, matrixTeam, matrixOwner)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}

	count := 0
	for line := range strings.SplitSeq(strings.TrimSpace(rec.Body.String()), "\n") {
		if line == "" {
			continue
		}
		var event model.AuditEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse jsonl: %v", err)
		}
		if event.TeamID != matrixTeam {
			t.Errorf("exported an event scoped to %q", event.TeamID)
		}
		count++
	}
	// One event, not three: the other team's is out of scope, and so is the
	// login, which has no team and belongs to the deployment-scoped read.
	if count != 1 {
		t.Fatalf("exported %d events, want 1", count)
	}
}
