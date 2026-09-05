package audit

import (
	"context"
	"time"
)

// Audit actions. These strings are persisted, so they are permanent: renaming
// one rewrites history for every reader that filters on it.
const (
	// UserLogin records a successful login. Failures are not recorded
	// here — a failed login says nothing about who the actor was, and
	// recording attempts keyed by a supplied email would turn the audit log
	// into a place to write arbitrary strings.
	UserLogin = "user.login"
	// UserLogout records a session being revoked on purpose. It is the
	// counterpart to UserLogin: together they bound when a session could
	// have been used.
	UserLogout = "user.logout"
	// RefreshReuse records a refresh token presented after it had already
	// been exchanged. It means the credential existed in two places, and the
	// session was revoked in response. Unlike the actions above this is not a
	// user's intent — it is the server reporting what it saw.
	RefreshReuse = "auth.refresh_reuse"
	// PasswordSet records an account's password being set or changed. The
	// event says that it happened, never what it became.
	PasswordSet = "user.password_set"
	// UserCreated records an account coming into existence, and
	// LoginCodeIssued records a way into one being minted. They are
	// separate because they are separate decisions: creating an account gives
	// nobody access until a code or a password follows.
	UserCreated     = "user.created"
	LoginCodeIssued = "user.login_code_issued"
	// UserDisabled and UserEnabled record an account's access being
	// stopped and restored. Disabling is not deletion: nothing is removed, and
	// enabling reverses the state and nothing else.
	UserDisabled = "user.disabled"
	UserEnabled  = "user.enabled"
	// SessionsRevoked records every live session of one account being
	// retired at once. It is separate from user.logout, which is a person
	// ending their own.
	SessionsRevoked = "user.sessions_revoked"
	// TeamMemberAdded and TeamMemberRemoved record changes to who
	// can reach a team's resources.
	TeamMemberAdded   = "team.member_added"
	TeamMemberRemoved = "team.member_removed"
	// TeamMemberInvited, InvitationAccepted, InvitationRevoked, and
	// InvitationExpired record a pending membership offer's whole life. Unlike
	// a failed login, an invitation names a specific, already-resolved account
	// before anyone acts on it, so every outcome is worth recording -- see
	// docs/design/team-membership-lifecycle.md §5.1 and §8.
	TeamMemberInvited  = "team.member_invited"
	InvitationAccepted = "team.invitation_accepted"
	InvitationRevoked  = "team.invitation_revoked"
	InvitationExpired  = "team.invitation_expired"
	// MemberRoleChanged and OwnershipTransferred record promotion, demotion,
	// and the transfer that results from setting a target's role to owner.
	// Transfer gets its own action distinct from a role change, even though it
	// is implemented as one call, because an investigation asking "did
	// ownership ever move" should not have to infer it from two
	// member_role_changed rows. See
	// docs/design/team-membership-lifecycle.md §5.2-§5.3, §8 M3.
	MemberRoleChanged    = "team.member_role_changed"
	OwnershipTransferred = "team.ownership_transferred"
	// TeamMemberLoginCodeIssued is distinct from the deployment-scoped
	// user.login_code_issued so a reader of the team's own trail (owner-only)
	// sees it without needing system_admin visibility into the deployment-wide
	// trail. See docs/design/team-membership-lifecycle.md §5.4, §8 M4.
	TeamMemberLoginCodeIssued = "team.member_login_code_issued"
	// ModelCreated, ModelEnabled, and ModelDisabled record
	// changes to which models a deployment will call. The catalog holds
	// provider credentials, so a change to it is a change to what the
	// deployment can spend and where prompts go.
	ModelCreated  = "llm_model.created"
	ModelEnabled  = "llm_model.enabled"
	ModelDisabled = "llm_model.disabled"
	// AccessDenied records a refused request. This is the one action
	// written on failure rather than success: a denial is what shows someone
	// probing at a boundary. TeamID is empty when the refused route was
	// deployment-scoped rather than team-scoped.
	AccessDenied = "access.denied"
	// SystemAdminGranted and SystemAdminRevoked record deployment
	// authority changing hands. They are not team-scoped, so TeamID is empty.
	//
	// These are the two actions in this list where a dropped write costs the
	// most: a grant that was made and not recorded is exactly what an
	// investigation needs. The write is still best-effort, for the reason in
	// internal/service/audit — see docs/design/system-administration.md
	// section 9.
	SystemAdminGranted = "system.admin_granted"
	SystemAdminRevoked = "system.admin_revoked"
	// ArtifactCreated and ArtifactDeleted record a durable file
	// entering and leaving a team's keeping. They are metadata-only by
	// construction: the target is the artifact ID, and neither the storage key, the
	// content, nor an uploader-supplied description belongs in the trail.
	ArtifactCreated = "artifact.created"
	ArtifactDeleted = "artifact.deleted"
	// ArtifactExpired is the tombstone nobody asked for: retention applied an
	// artifact's own ExpiresAt. It names the artifact, because it is what a
	// reader finds instead of the artifact.deleted they would otherwise expect.
	ArtifactExpired = "artifact.expired"
	// ArtifactsPurged records a sweep reclaiming the objects of already
	// tombstoned artifacts, with the count and the bytes. It is per sweep
	// rather than per artifact: the tombstone that authorized each one is
	// already in the trail, and this says only that the bytes are now gone.
	ArtifactsPurged = "artifact.purged"
	// ArtifactShareCreated and ArtifactShareRevoked record a public link to an
	// artifact being opened and withdrawn. Metadata-only like the artifact
	// events: the target is the artifact ID and the detail is the share ID —
	// the token never appears, because a trail that held it would hand a reader
	// the very credential the link is.
	ArtifactShareCreated = "artifact.share_created"
	ArtifactShareRevoked = "artifact.share_revoked"
	// The plugin actions record changes to what a deployment's members can
	// install. A release is instructions that cause tool use, processes that
	// start with someone's credentials, and hooks that run local programs, so
	// publishing one is a change to what every machine that installs it will
	// do. The detail names the version and a digest prefix — never package
	// contents or configuration values.
	PluginCreated    = "plugin.created"
	PluginUpdated    = "plugin.updated"
	PluginArchived   = "plugin.archived"
	PluginUnarchived = "plugin.unarchived"
	PluginPublished  = "plugin.published"
	PluginYanked     = "plugin.yanked"

	// A team activating a release is the record that answers "why did this run
	// have this capability". The pin moves and the suspension are separate
	// actions because each is a different decision about a team's runs.
	PluginActivated    = "plugin.activated"
	PluginPinMoved     = "plugin.pin_moved"
	PluginSuspended    = "plugin.suspended"
	PluginResumed      = "plugin.resumed"
	TeamPluginCuration = "team.plugin_curation_set"
	// TeamSandboxDefaultsSet records a team's default sandbox tiers changing --
	// the tiers an agent that declares neither inherits. See
	// docs/design/agent-sandbox-policy.md §9 M3.
	TeamSandboxDefaultsSet = "team.sandbox_defaults_set"
	// EventsExported records the trail itself being read out in bulk.
	// Reading every recorded action is a sensitive action, and an export that
	// left no trace would be the one way to consult the record without
	// appearing in it.
	EventsExported = "audit.exported"
	// EventsPruned records events expiring under the deployment's
	// retention window, naming the cutoff and how many rows went.
	//
	// It is what separates a gap that policy created from evidence that was
	// lost: without it, a trail that starts on a Tuesday is indistinguishable
	// from a trail somebody truncated. Recording the deletion in the same
	// table it deletes from is deliberate — the event survives its own sweep
	// until the window moves past it, and by then a later one says the same.
	EventsPruned = "audit.pruned"
	// QuotaThresholdReached records a team crossing a share of its quota,
	// and QuotaExceeded records work being refused because the limit was
	// reached. The first is a warning nobody was blocked by; the second is the
	// block. They are separate actions because they call for different
	// responses — one is a heads-up, the other is work not happening.
	//
	// Both are written at most once per limit per period, so a team that keeps
	// submitting does not turn its own trail into a log of retries.
	QuotaThresholdReached = "quota.threshold_reached"
	QuotaExceeded         = "quota.exceeded"
)

// ActorOperator is the ActorID for an action taken by an operator command
// rather than by a signed-in user. The command runs on the machine that holds
// the database credentials and has no session to name, so the record names the
// binary. That is less than naming a person and more than recording nothing.
const ActorOperator = "buildmax-server"

// Audit actor kinds.
const (
	ActorUser   = "user"
	ActorWorker = "worker"
	ActorSystem = "system"
)

// Event is one recorded action.
//
// It deliberately carries no prompts, no generated content, no tool output, and
// no credentials — only who did what to which object. Run diagnostics live in
// the durable run trace and per-call accounting in the llm_call ledger; this is
// the record that a meaningful action occurred, which is a different question
// with a different retention answer.
type Event struct {
	ID string `json:"id"`
	// TeamID is empty for actions that are not team-scoped, such as a login.
	TeamID     string `json:"team_id,omitempty"`
	ActorType  string `json:"actor_type"`
	ActorID    string `json:"actor_id"`
	Action     string `json:"action"`
	TargetType string `json:"target_type,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	// Detail is a short, non-sensitive note — a role name, a model alias. It
	// is not a place for request bodies.
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Filter narrows an audit search. Every field is optional; the zero value
// matches everything.
//
// There is no free-text field and there will not be one. The trail holds who
// did what to which object, and a search across it is a search over those —
// adding a text query would invite a `Detail LIKE` scan over a column whose
// whole purpose is to stay small and structured.
type Filter struct {
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
	Since time.Time
	Until time.Time
}

// Cursor is a position in the trail, used to walk it in bounded pages.
//
// It exists because offset paging is wrong for an export. An export reads the
// table over many round trips while rows are still being appended at one end
// and, under a retention window, removed at the other — and either shifts every
// offset behind it, so a page boundary silently skips records. A keyset cursor
// names where the last page stopped, so the next one continues from that record
// no matter what the table did meanwhile.
//
// The zero value starts at the newest event. `CreatedAt` alone is not enough to
// resume from: microsecond resolution narrows collisions but does not remove
// them, so the event's own identity breaks the tie.
//
// That identity is the public one. The row key that actually orders a tie lives
// below the store boundary, so a store resolves this handle to it rather than
// handing a database key to a caller.
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// Zero reports whether the cursor is a fresh start rather than a resumption.
func (c Cursor) Zero() bool { return c.CreatedAt.IsZero() && c.ID == "" }

// Writer appends audit events.
//
// Most callers only ever write: an operator command, a handler recording a
// grant, the quota service noticing a limit. They take this rather than the
// full store so that recording an action never carries the ability to read the
// trail back, which is a wider permission than any of them needs.
type Writer interface {
	// RecordAuditEvent appends one event. Events are append-only; nothing here
	// updates or deletes one, because a record that can be edited is not
	// evidence.
	RecordAuditEvent(ctx context.Context, in Event) error
}

// Store persists audit events.
//
// Record takes no error-returning contract the caller must handle at the call
// site by design: see the recorder in internal/service/audit for why a failed
// write is logged rather than propagated, and what that costs.
//
// Nothing here updates or deletes an event. Retention expiry is the one thing
// that removes rows, and it lives in PruneStore rather than in this
// interface: it is a policy applied uniformly by age, not an edit anyone can
// make to a particular record, and every reader of this interface should stay
// unable to reach it.
type Store interface {
	Writer
	// ListAuditEvents returns a team's events, newest first.
	//
	// It stays alongside SearchAuditEvents rather than being replaced by it:
	// a team owner asks a narrower question, and giving that reader the wider
	// method is how a team-scoped route acquires a deployment-scoped answer.
	ListAuditEvents(ctx context.Context, teamID string, limit, offset int) ([]Event, int, error)
	// SearchAuditEvents returns events across every team, newest first. It is
	// the deployment-scoped read, and only /api/admin routes may reach it.
	SearchAuditEvents(ctx context.Context, filter Filter, limit, offset int) ([]Event, int, error)
	// ExportTeamAuditEvents returns one page of a team's events, newest first,
	// continuing from after. It answers the same question as ListAuditEvents
	// and differs only in how it pages, because an export walks the whole
	// trail rather than showing the first screen of it.
	ExportTeamAuditEvents(ctx context.Context, teamID string, after Cursor, limit int) ([]Event, error)
	// ExportAuditEvents is the deployment-scoped counterpart, and the same rule
	// applies as for SearchAuditEvents: only /api/admin routes may reach it.
	// The pair stays split for the reason ListAuditEvents gives — a team-scoped
	// route that can name its own filter is a route that can widen it.
	ExportAuditEvents(ctx context.Context, filter Filter, after Cursor, limit int) ([]Event, error)
}

// PruneStore expires audit events under a retention window.
//
// It is deliberately not part of Store. Every reader and writer of the
// trail holds that interface, and none of them should be able to remove a
// record; the one caller that legitimately can is the retention sweep, which
// takes this narrower one.
type PruneStore interface {
	// PruneAuditEvents deletes events recorded before the cutoff, at most
	// limit of them, and returns how many went. A bounded delete keeps one
	// sweep from holding the table while a backlog of an old deployment's
	// events is removed; the caller repeats until it returns fewer than limit.
	PruneAuditEvents(ctx context.Context, before time.Time, limit int) (int64, error)
	// OldestAuditEventAt reports the timestamp of the oldest event, or zero
	// when the table is empty. The sweep uses it to say what a prune actually
	// removed rather than only what it was allowed to.
	OldestAuditEventAt(ctx context.Context) (time.Time, error)
}
