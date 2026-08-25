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
	CreatedBy      string              `json:"created_by"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

// Member is one user's membership in a team.
type Member struct {
	TeamID    string    `json:"team_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
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
}
