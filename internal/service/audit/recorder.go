// Package audit records that a sensitive action happened.
//
// It answers a governance question — who changed access, who changed which
// models a deployment will call, who was refused — and deliberately not an
// operational one. Run diagnostics belong to the durable run trace and per-call
// accounting to the llm_call ledger; duplicating either here would give the
// same fact two retention policies and two chances to disagree.
package audit

import (
	"context"
	"log/slog"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// Recorder writes audit events. A nil Recorder, or one with no store, discards
// them, so a deployment without a database still runs.
type Recorder struct {
	Store model.AuditStore
}

// NewRecorder returns a Recorder writing to store.
func NewRecorder(store model.AuditStore) *Recorder {
	return &Recorder{Store: store}
}

// Record appends one event.
//
// A failed write is logged at error and dropped rather than failing the action
// that caused it. That is a real limitation, not an oversight: it means this is
// a record of what happened when the database was reachable, not a guarantee
// that every action was recorded, and a deployment that needs the stronger
// property has to make the write part of the same transaction as the action.
// The weaker choice is deliberate for now — refusing a login because an audit
// insert failed turns a logging outage into an authentication outage.
//
// The error is logged with the action and actor, so a dropped event leaves a
// trace somewhere even when the table did not get one.
func (r *Recorder) Record(ctx context.Context, event model.AuditEvent) {
	if r == nil || r.Store == nil {
		return
	}
	if err := r.Store.RecordAuditEvent(ctx, event); err != nil {
		slog.Error("audit event not recorded",
			"err", err,
			"action", event.Action,
			"actor_type", event.ActorType,
			"actor_id", event.ActorID,
			"team_id", event.TeamID,
		)
	}
}

// UserAction records an action a signed-in user performed on a team resource.
func (r *Recorder) UserAction(ctx context.Context, userID, teamID, action, targetType, targetID, detail string) {
	r.Record(ctx, model.AuditEvent{
		TeamID:     teamID,
		ActorType:  model.AuditActorUser,
		ActorID:    userID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Detail:     detail,
	})
}

// Denied records a refused team-scoped request.
//
// The route is passed as the target rather than the full URL: a URL carries
// query strings, and this table is not a place for values a caller chose.
func (r *Recorder) Denied(ctx context.Context, userID, teamID, route string) {
	r.Record(ctx, model.AuditEvent{
		TeamID:     teamID,
		ActorType:  model.AuditActorUser,
		ActorID:    userID,
		Action:     model.AuditAccessDenied,
		TargetType: "route",
		TargetID:   route,
	})
}
