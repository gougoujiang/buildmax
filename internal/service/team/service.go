// Package team owns membership rules: who is in a team, who may change that,
// and what a member may be.
//
// The handlers held these before, and wrote the owner check out twice --
// fetching the roster, scanning it for the caller, comparing the role -- once
// to add a member and once to remove one. A rule about who may change a team is
// exactly the thing that must not exist in two copies.
package team

import (
	"context"
	"strings"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/core/model"
)

var (
	ErrTeamsNotConfigured = apierr.New(apierr.KindNotConfigured, "teams not configured")
	ErrUsersNotConfigured = apierr.New(apierr.KindNotConfigured, "users not configured")
	ErrOnlyOwnerCanAdd    = apierr.New(apierr.KindForbidden, "only team owners can add members")
	ErrOnlyOwnerCanRemove = apierr.New(apierr.KindForbidden, "only team owners can remove members")
	ErrEmailRequired      = apierr.New(apierr.KindInvalid, "email is required")
	ErrUnsupportedRole    = apierr.New(apierr.KindInvalid, "only member role is supported")
	ErrUserDoesNotExist   = apierr.New(apierr.KindInvalid, "user does not exist")
	ErrCannotRemoveSelf   = apierr.New(apierr.KindInvalid, "owners cannot remove themselves")
	ErrMemberNotFound     = apierr.New(apierr.KindNotFound, "team member not found")
)

type Service struct {
	Teams model.TeamStore
	Users model.UserStore
}

// Member pairs a membership with the account behind it. User is nil when the
// deployment has no user store to resolve it against.
type Member struct {
	Membership model.TeamMember
	User       *model.User
}

type AddMemberCmd struct {
	TeamID string
	// ActorID is the caller, who must be an owner.
	ActorID string
	Email   string
	Role    string
}

type RemoveMemberCmd struct {
	TeamID       string
	ActorID      string
	TargetUserID string
}

// ListMembers returns the roster with each member's account resolved.
func (s *Service) ListMembers(ctx context.Context, teamID string) ([]Member, error) {
	if s.Teams == nil {
		return nil, ErrTeamsNotConfigured
	}
	memberships, err := s.Teams.ListTeamMembers(ctx, teamID)
	if err != nil {
		return nil, err
	}
	out := make([]Member, len(memberships))
	for i := range memberships {
		out[i] = Member{Membership: memberships[i]}
		if s.Users == nil {
			continue
		}
		user, err := s.Users.GetUser(ctx, memberships[i].UserID)
		if err != nil {
			return nil, err
		}
		out[i].User = user
	}
	return out, nil
}

func (s *Service) AddMember(ctx context.Context, cmd AddMemberCmd) (*model.TeamMember, *model.User, error) {
	if s.Teams == nil {
		return nil, nil, ErrTeamsNotConfigured
	}
	if s.Users == nil {
		return nil, nil, ErrUsersNotConfigured
	}
	if err := s.requireOwner(ctx, cmd.TeamID, cmd.ActorID, ErrOnlyOwnerCanAdd); err != nil {
		return nil, nil, err
	}

	email := strings.TrimSpace(strings.ToLower(cmd.Email))
	if email == "" {
		return nil, nil, ErrEmailRequired
	}
	role := strings.TrimSpace(cmd.Role)
	if role == "" {
		role = model.TeamRoleMember
	}
	// Owner and admin are not grantable here. Adding someone as an owner from
	// the same call that adds a member would make an escalation look like a
	// routine invitation.
	if role != model.TeamRoleMember {
		return nil, nil, ErrUnsupportedRole
	}

	user, err := s.Users.UserByEmail(ctx, email)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		return nil, nil, ErrUserDoesNotExist
	}
	member, err := s.Teams.AddTeamMember(ctx, cmd.TeamID, user.ID, role)
	if err != nil {
		return nil, nil, err
	}
	return member, user, nil
}

func (s *Service) RemoveMember(ctx context.Context, cmd RemoveMemberCmd) error {
	if s.Teams == nil {
		return ErrTeamsNotConfigured
	}
	members, err := s.Teams.ListTeamMembers(ctx, cmd.TeamID)
	if err != nil {
		return err
	}
	if !hasRole(members, cmd.ActorID, model.TeamRoleOwner) {
		return ErrOnlyOwnerCanRemove
	}
	// An owner removing themselves could leave a team nobody can administer.
	if cmd.TargetUserID == cmd.ActorID {
		return ErrCannotRemoveSelf
	}
	if !isMember(members, cmd.TargetUserID) {
		return ErrMemberNotFound
	}
	return s.Teams.RemoveTeamMember(ctx, cmd.TeamID, cmd.TargetUserID)
}

func (s *Service) requireOwner(ctx context.Context, teamID, userID string, refusal error) error {
	members, err := s.Teams.ListTeamMembers(ctx, teamID)
	if err != nil {
		return err
	}
	if !hasRole(members, userID, model.TeamRoleOwner) {
		return refusal
	}
	return nil
}

func hasRole(members []model.TeamMember, userID, role string) bool {
	for i := range members {
		if members[i].UserID == userID {
			return members[i].Role == role
		}
	}
	return false
}

func isMember(members []model.TeamMember, userID string) bool {
	for i := range members {
		if members[i].UserID == userID {
			return true
		}
	}
	return false
}
