package model

import "context"

// Audit actions. These strings are persisted, so they are permanent: renaming
// one rewrites history for every reader that filters on it.
const (
	// AuditUserLogin records a successful login. Failures are not recorded
	// here — a failed login says nothing about who the actor was, and
	// recording attempts keyed by a supplied email would turn the audit log
	// into a place to write arbitrary strings.
	AuditUserLogin = "user.login"
	// AuditTeamMemberAdded and AuditTeamMemberRemoved record changes to who
	// can reach a team's resources.
	AuditTeamMemberAdded   = "team.member_added"
	AuditTeamMemberRemoved = "team.member_removed"
	// AuditModelCreated, AuditModelEnabled, and AuditModelDisabled record
	// changes to which models a deployment will call. The catalog holds
	// provider credentials, so a change to it is a change to what the
	// deployment can spend and where prompts go.
	AuditModelCreated  = "llm_model.created"
	AuditModelEnabled  = "llm_model.enabled"
	AuditModelDisabled = "llm_model.disabled"
	// AuditAccessDenied records a refused team-scoped request. This is the one
	// action written on failure rather than success: a denial is what shows
	// someone probing at a boundary.
	AuditAccessDenied = "access.denied"
)

// Audit actor kinds.
const (
	AuditActorUser   = "user"
	AuditActorWorker = "worker"
	AuditActorSystem = "system"
)

// AuditEvent is one recorded action.
//
// It deliberately carries no prompts, no generated content, no tool output, and
// no credentials — only who did what to which object. Run diagnostics live in
// the durable run trace and per-call accounting in the llm_call ledger; this is
// the record that a meaningful action occurred, which is a different question
// with a different retention answer.
type AuditEvent struct {
	ID           uint   `json:"-"`
	AuditEventID string `json:"audit_event_id"`
	// TeamID is empty for actions that are not team-scoped, such as a login.
	TeamID     string `json:"team_id,omitempty"`
	ActorType  string `json:"actor_type"`
	ActorID    string `json:"actor_id"`
	Action     string `json:"action"`
	TargetType string `json:"target_type,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	// Detail is a short, non-sensitive note — a role name, a model alias. It
	// is not a place for request bodies.
	Detail    string `json:"detail,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// AuditStore persists audit events.
//
// Record takes no error-returning contract the caller must handle at the call
// site by design: see the recorder in internal/service/audit for why a failed
// write is logged rather than propagated, and what that costs.
type AuditStore interface {
	// RecordAuditEvent appends one event. Events are append-only; there is no
	// update or delete, because a record that can be edited is not evidence.
	RecordAuditEvent(ctx context.Context, in AuditEvent) error
	// ListAuditEvents returns a team's events, newest first.
	ListAuditEvents(ctx context.Context, teamID string, limit, offset int) ([]AuditEvent, int, error)
}
