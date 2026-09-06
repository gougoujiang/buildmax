package identity

import (
	"context"
	"errors"
	"time"
)

// System roles are deployment-scoped: they are held by a user and attached to
// no space. A grant is an authority to operate the deployment, not a key to its
// contents — see docs/design/system-administration.md.
const (
	// SystemRoleAdmin may manage accounts, read deployment status, and search
	// the audit trail across spaces. It grants no access to any space's issues,
	// conversations, artifacts, files, or run traces: those stay behind space
	// membership, which a system grant never substitutes for.
	SystemRoleAdmin = "system_admin"
)

// ErrSystemRoleUnknown is returned when a grant names a role this build does
// not define. The role column exists so a second role can be added without a
// migration, but only roles with a caller are accepted.
var ErrSystemRoleUnknown = errors.New("unknown system role")

// ErrSystemGrantExists is returned when the user already holds an active grant
// for the role.
var ErrSystemGrantExists = errors.New("user already holds this system role")

// ErrSystemGrantLastHolder is returned by RevokeSystemRole when keepLastHolder
// is set and the grant is the deployment's last effective holder of the role.
// Removing it would lock every signed-in caller out of the admin area with no
// way back through the same door, so only the operator shell — which reaches the
// database directly and can undo the lockout — may do it.
var ErrSystemGrantLastHolder = errors.New("cannot revoke the last effective holder of this system role")

// ValidSystemRole reports whether role is one this build authorizes.
func ValidSystemRole(role string) bool {
	return role == SystemRoleAdmin
}

// SystemGrant is one deployment-scoped authority held by one user.
//
// It is a row rather than a flag on User because a flag has no granting actor,
// no timestamp, and no history — and those three are the point. Authority that
// cannot be attributed or revoked is the thing this model exists to avoid.
type SystemGrant struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	// GrantedBy is the user_id of the admin who made the grant, or
	// AuditActorOperator when it came from the operator command, which runs
	// with database credentials and no signed-in identity. It is deliberately
	// the same string the matching audit event carries in ActorID: one act
	// should not have two names across two tables.
	GrantedBy string    `json:"granted_by"`
	GrantedAt time.Time `json:"granted_at"`
	// RevokedAt is nil while the grant is active. Revoking sets it rather than
	// deleting the row: who held authority and when is the question an
	// investigation asks, and a deleted row cannot answer it.
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// Active reports whether the grant is currently in force.
func (g SystemGrant) Active() bool { return g.RevokedAt == nil }

// SystemGrantStore persists deployment-scoped role grants.
type SystemGrantStore interface {
	// ActiveSystemRoles returns the roles userID currently holds, empty for
	// almost every caller. It is on the path of every authenticated admin
	// request, so it must stay a single indexed read.
	ActiveSystemRoles(ctx context.Context, userID string) ([]string, error)
	// ListSystemGrants returns grants newest first. includeRevoked adds the
	// retired ones, which is how the trail of who held authority is read.
	ListSystemGrants(ctx context.Context, includeRevoked bool) ([]SystemGrant, error)
	// GrantSystemRole grants role to userID. It returns ErrSystemGrantExists
	// when an active grant is already there, so a caller can report "already
	// an admin" rather than silently creating a second row.
	GrantSystemRole(ctx context.Context, userID, role, grantedBy string, now time.Time) (*SystemGrant, error)
	// RevokeSystemRole revokes userID's active grant for role and reports
	// whether one was found. Revoking an absent grant is not an error: the end
	// state is what was asked for.
	//
	// When keepLastHolder is true it refuses with ErrSystemGrantLastHolder
	// rather than remove the deployment's last effective holder, and it decides
	// and revokes in one atomic step so two concurrent revokes cannot both pass
	// the check and leave the role with nobody. When false — the operator shell,
	// which can undo a lockout — it revokes unconditionally.
	RevokeSystemRole(ctx context.Context, userID, role string, now time.Time, keepLastHolder bool) (bool, error)
	// CountActiveSystemGrants counts the effective holders of role: an active
	// grant on an account that is not disabled. A disabled account cannot
	// authorize a request, so it cannot be the holder that keeps the deployment
	// reachable — see docs/design/system-administration.md section 6.
	CountActiveSystemGrants(ctx context.Context, role string) (int, error)
}
