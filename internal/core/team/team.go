package team

import (
	"context"
	coreplugin "github.com/gougoujiang/buildmax/internal/core/plugin"
	"time"
)

const (
	// RoleOwner is the initial role for the user who creates a team.
	RoleOwner = "owner"
	// RoleAdmin can manage shared automation assets but not membership ownership.
	RoleAdmin = "admin"
	// RoleMember is the basic collaboration role for invited members.
	RoleMember = "member"
	// DefaultPersonalName is the initial UX-facing name for a user's own space.
	DefaultPersonalName = "My Space"
)

// Team is the ownership and collaboration boundary for working resources.
// A user's default personal team is represented by personal_for_user_id.
type Team struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	PersonalForUserID *string `json:"personal_for_user_id,omitempty"`
	QuotaTier         string  `json:"quota_tier,omitempty"`
	// PluginCuration is who fills this team's plugin activation list; empty
	// reads as plugin.CurationOpen. See core/plugin/activation.go.
	PluginCuration coreplugin.Curation `json:"plugin_curation,omitempty"`
	// DefaultSandboxNetworkTier and DefaultSandboxFilesystemTier are the
	// config.SandboxNetworkTier / config.SandboxFilesystemTier values an agent
	// that declares neither tier inherits. Empty means the surface baseline
	// applies instead -- see docs/design/agent-sandbox-policy.md §9 M3.
	DefaultSandboxNetworkTier    string    `json:"default_sandbox_network_tier,omitempty"`
	DefaultSandboxFilesystemTier string    `json:"default_sandbox_filesystem_tier,omitempty"`
	CreatedBy                    string    `json:"created_by"`
	CreatedAt                    time.Time `json:"created_at"`
	UpdatedAt                    time.Time `json:"updated_at"`
}

// Member is one user's membership in a team.
type Member struct {
	TeamID    string    `json:"team_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// InvitationTTLDefault is how long a pending invitation stays acceptable when
// nobody accepts it. Three days rather than a login code's shorter window: an
// invitation is meant to be acted on whenever the recipient next opens
// Portal, not inside the exchange that sent it. See
// docs/design/team-membership-lifecycle.md.
const InvitationTTLDefault = 72 * time.Hour

// Invitation is a pending offer of team membership against an account that
// already exists — team-scoped invitation never creates one. See
// docs/design/team-membership-lifecycle.md §1 for why account creation and
// team membership are kept as two different authorities.
type Invitation struct {
	ID     string `json:"id"`
	TeamID string `json:"team_id"`
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	// InvitedBy is the user who sent the invitation.
	InvitedBy string    `json:"invited_by"`
	ExpiresAt time.Time `json:"expires_at"`
	// AcceptedAt and RevokedAt are mutually exclusive; both nil means still
	// pending. There is no separate status column -- these two timestamps plus
	// ExpiresAt are the whole state, the same shape user.disabled_at and
	// system_grant.revoked_at already use for "off until proven otherwise".
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Pending reports whether the invitation may still be accepted: neither
// answered nor withdrawn, and not past its offer window.
func (i Invitation) Pending(now time.Time) bool {
	return i.AcceptedAt == nil && i.RevokedAt == nil && now.Before(i.ExpiresAt)
}

// Store provides team persistence and membership lookup.
type Store interface {
	// GetTeam returns the team by team_id, or (nil, nil) when not found.
	GetTeam(ctx context.Context, teamID string) (*Team, error)
	// GetPersonalTeamByUser returns the default personal team for the user, or (nil, nil) when not found.
	GetPersonalTeamByUser(ctx context.Context, userID string) (*Team, error)
	// ListTeamsByUser returns all teams the user belongs to, ordered by created_at ASC.
	ListTeamsByUser(ctx context.Context, userID string) ([]Team, error)
	// CreateTeam creates a new team and owner membership.
	CreateTeam(ctx context.Context, name, createdBy, quotaTier string) (*Team, error)
	// AddTeamMember adds or updates a team membership.
	AddTeamMember(ctx context.Context, teamID, userID, role string) (*Member, error)
	// RemoveTeamMember removes one membership from a team.
	RemoveTeamMember(ctx context.Context, teamID, userID string) error
	// ListTeamMembers returns members of the team ordered by created_at ASC.
	ListTeamMembers(ctx context.Context, teamID string) ([]Member, error)
	// ListAllTeams returns every team newest first, with the total count. A
	// non-empty query filters on name as a substring.
	//
	// It is the one method here that ignores membership, so only
	// deployment-scoped callers may reach it. It returns teams, never their
	// contents: an administrator learns that a team exists and how large it is,
	// not what is in it.
	ListAllTeams(ctx context.Context, query string, limit, offset int) ([]Team, int, error)
	// CountTeamMembers returns member counts for the given teams, keyed by
	// team id. It exists so listing teams is two queries rather than one per
	// row.
	CountTeamMembers(ctx context.Context, teamIDs []string) (map[string]int, error)
	// SetTeamPluginCuration records who fills the team's plugin activation
	// list, or returns ErrNotFound. The value is validated above this layer.
	SetTeamPluginCuration(ctx context.Context, teamID string, mode coreplugin.Curation) error
	// SetTeamSandboxDefaults records the tiers an agent that declares neither
	// inherits, or returns ErrNotFound. The values are validated above this
	// layer, the same way SetTeamPluginCuration's mode is.
	SetTeamSandboxDefaults(ctx context.Context, teamID, networkTier, filesystemTier string) error

	// CreateInvitation creates a pending invitation for userID to join teamID
	// at role, sent by invitedBy, acceptable until expiresAt.
	CreateInvitation(ctx context.Context, teamID, userID, role, invitedBy string, expiresAt time.Time) (*Invitation, error)
	// GetInvitation returns one invitation by its handle, or (nil, nil) when
	// not found.
	GetInvitation(ctx context.Context, invitationID string) (*Invitation, error)
	// ListPendingInvitationsByTeam returns a team's still-pending invitations,
	// newest first.
	ListPendingInvitationsByTeam(ctx context.Context, teamID string, now time.Time) ([]Invitation, error)
	// ListPendingInvitationsByUser returns one account's still-pending
	// invitations across every team, newest first -- what GET /api/invitations
	// answers.
	ListPendingInvitationsByUser(ctx context.Context, userID string, now time.Time) ([]Invitation, error)
	// AcceptInvitation marks a pending invitation accepted and creates the
	// resulting team membership in the same transaction, or returns (nil, nil)
	// when the row does not exist or is no longer pending -- an invitation
	// marked accepted with no membership to show for it would be evidence of a
	// bug no caller could act on.
	AcceptInvitation(ctx context.Context, invitationID string, now time.Time) (*Invitation, error)
	// RevokeInvitation marks a pending invitation revoked. A row that does not
	// exist or is no longer pending is not an error -- withdrawing an offer
	// that already resolved itself asks for nothing this store has to refuse.
	RevokeInvitation(ctx context.Context, invitationID string, now time.Time) error

	// TransferOwnership makes toUserID the team's owner and demotes fromUserID
	// to admin, atomically -- a team must never be read with two owners or
	// none because a caller observed the change half-applied. Both must
	// already be members; the service enforces that, and the last-owner
	// invariant, before calling this. See
	// docs/design/team-membership-lifecycle.md §5.2-§5.3.
	TransferOwnership(ctx context.Context, teamID, fromUserID, toUserID string) error
}
