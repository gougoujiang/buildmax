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
	// AuditUserLogout records a session being revoked on purpose. It is the
	// counterpart to AuditUserLogin: together they bound when a session could
	// have been used.
	AuditUserLogout = "user.logout"
	// AuditRefreshReuse records a refresh token presented after it had already
	// been exchanged. It means the credential existed in two places, and the
	// session was revoked in response. Unlike the actions above this is not a
	// user's intent — it is the server reporting what it saw.
	AuditRefreshReuse = "auth.refresh_reuse"
	// AuditPasswordSet records an account's password being set or changed. The
	// event says that it happened, never what it became.
	AuditPasswordSet = "user.password_set"
	// AuditUserCreated records an account coming into existence, and
	// AuditLoginCodeIssued records a way into one being minted. They are
	// separate because they are separate decisions: creating an account gives
	// nobody access until a code or a password follows.
	AuditUserCreated     = "user.created"
	AuditLoginCodeIssued = "user.login_code_issued"
	// AuditUserDisabled and AuditUserEnabled record an account's access being
	// stopped and restored. Disabling is not deletion: nothing is removed, and
	// enabling reverses the state and nothing else.
	AuditUserDisabled = "user.disabled"
	AuditUserEnabled  = "user.enabled"
	// AuditSessionsRevoked records every live session of one account being
	// retired at once. It is separate from user.logout, which is a person
	// ending their own.
	AuditSessionsRevoked = "user.sessions_revoked"
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
	// AuditAccessDenied records a refused request. This is the one action
	// written on failure rather than success: a denial is what shows someone
	// probing at a boundary. TeamID is empty when the refused route was
	// deployment-scoped rather than team-scoped.
	AuditAccessDenied = "access.denied"
	// AuditSystemAdminGranted and AuditSystemAdminRevoked record deployment
	// authority changing hands. They are not team-scoped, so TeamID is empty.
	//
	// These are the two actions in this list where a dropped write costs the
	// most: a grant that was made and not recorded is exactly what an
	// investigation needs. The write is still best-effort, for the reason in
	// internal/service/audit — see docs/design/system-administration.md
	// section 9.
	AuditSystemAdminGranted = "system.admin_granted"
	AuditSystemAdminRevoked = "system.admin_revoked"
)

// AuditActorOperator is the ActorID for an action taken by an operator command
// rather than by a signed-in user. The command runs on the machine that holds
// the database credentials and has no session to name, so the record names the
// binary. That is less than naming a person and more than recording nothing.
const AuditActorOperator = "buildmax-server"

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

// AuditFilter narrows an audit search. Every field is optional; the zero value
// matches everything.
//
// There is no free-text field and there will not be one. The trail holds who
// did what to which object, and a search across it is a search over those —
// adding a text query would invite a `Detail LIKE` scan over a column whose
// whole purpose is to stay small and structured.
type AuditFilter struct {
	// TeamID matches events scoped to one team. It does not match the events
	// that have no team, such as a login or a grant — see WithoutTeam.
	TeamID string
	// WithoutTeam matches only the deployment-scoped events. It exists because
	// an empty TeamID already means "any team", so there would otherwise be no
	// way to ask for the ones a team-scoped reader can never see.
	WithoutTeam bool
	ActorID     string
	Action      string
	// Since and Until bound created_at, inclusive and exclusive respectively.
	// Zero means unbounded.
	Since int64
	Until int64
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
	//
	// It stays alongside SearchAuditEvents rather than being replaced by it:
	// a team owner asks a narrower question, and giving that reader the wider
	// method is how a team-scoped route acquires a deployment-scoped answer.
	ListAuditEvents(ctx context.Context, teamID string, limit, offset int) ([]AuditEvent, int, error)
	// SearchAuditEvents returns events across every team, newest first. It is
	// the deployment-scoped read, and only /api/admin routes may reach it.
	SearchAuditEvents(ctx context.Context, filter AuditFilter, limit, offset int) ([]AuditEvent, int, error)
}
