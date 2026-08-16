package audit

import (
	"context"
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/mock"
)

// TestRecorderSurvivesAMissingStore covers the two configurations that must not
// panic: no recorder at all, and a recorder with nothing behind it. A
// deployment without a database still has to serve.
func TestRecorderSurvivesAMissingStore(t *testing.T) {
	var nilRecorder *Recorder
	nilRecorder.Record(context.Background(), model.AuditEvent{Action: model.AuditUserLogin})
	nilRecorder.UserAction(context.Background(), "u", "tm", model.AuditUserLogin, "", "", "")
	nilRecorder.Denied(context.Background(), "u", "tm", "route")

	empty := NewRecorder(nil)
	empty.Record(context.Background(), model.AuditEvent{Action: model.AuditUserLogin})
}

// TestRecordDoesNotFailTheAction pins the failure policy, which is a real
// trade rather than an oversight: a failed audit write is logged and dropped,
// so a logging outage does not become an authentication outage. It also means
// this is a record of what happened while the database was reachable, not a
// guarantee that every action was recorded.
func TestRecordDoesNotFailTheAction(t *testing.T) {
	store := &mock.MockAuditStore{Err: errors.New("database is gone")}
	r := NewRecorder(store)

	// No return value to check: the signature is the assertion. If Record ever
	// grows an error a caller must handle, this test stops compiling and the
	// trade above has to be made again on purpose.
	r.Record(context.Background(), model.AuditEvent{Action: model.AuditUserLogin, ActorID: "u_1"})

	if len(store.Events) != 0 {
		t.Errorf("a failed write must not be recorded as success: %+v", store.Events)
	}
}

func TestUserActionAndDenied(t *testing.T) {
	store := &mock.MockAuditStore{}
	r := NewRecorder(store)

	r.UserAction(context.Background(), "u_1", "tm_1", model.AuditTeamMemberAdded, "user", "u_2", "admin")
	r.Denied(context.Background(), "u_3", "tm_1", "manage_agents")

	if len(store.Events) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(store.Events), store.Events)
	}
	added := store.Events[0]
	if added.ActorType != model.AuditActorUser || added.ActorID != "u_1" || added.TargetID != "u_2" || added.Detail != "admin" {
		t.Errorf("member-added event wrong: %+v", added)
	}
	denied := store.Events[1]
	if denied.Action != model.AuditAccessDenied || denied.TargetType != "route" || denied.TargetID != "manage_agents" {
		t.Errorf("denial event wrong: %+v", denied)
	}
}
