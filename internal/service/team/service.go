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
	"fmt"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreidentity "github.com/gougoujiang/buildmax/internal/core/identity"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
)

var (
	ErrTeamsNotConfigured = apierr.New(apierr.KindNotConfigured, "teams not configured")
	ErrUsersNotConfigured = apierr.New(apierr.KindNotConfigured, "users not configured")
	ErrOnlyOwnerCanRemove = apierr.New(apierr.KindForbidden, "only team owners can remove members")
	ErrEmailRequired      = apierr.New(apierr.KindInvalid, "email is required")
	ErrUnsupportedRole    = apierr.New(apierr.KindInvalid, "role must be member or admin")
	ErrCannotRemoveSelf   = apierr.New(apierr.KindInvalid, "owners cannot remove themselves")
	ErrMemberNotFound     = apierr.New(apierr.KindNotFound, "team member not found")

	// ErrInviteeAccountRequired refuses an invitation to an email with no
	// account. Creating one is system_admin's job, never a team-scoped call's
	// -- see docs/design/team-membership-lifecycle.md §1.
	ErrInviteeAccountRequired = apierr.New(apierr.KindInvalid,
		"no account exists for this email; ask a system administrator to create one "+
			"(POST /api/admin/users or buildmax-server user create), then invite it")
	// ErrOnlyOwnerOrAdminCanInvite covers creating, listing, and revoking
	// invitations -- the one action ActionInviteTeamMember gates.
	ErrOnlyOwnerOrAdminCanInvite = apierr.New(apierr.KindForbidden,
		"only a team owner or admin may invite, list, or revoke invitations")
	// ErrOnlyOwnerCanInviteAdmin is the role-content restriction Allows cannot
	// express on its own: admin may invite, but only at the member role.
	ErrOnlyOwnerCanInviteAdmin  = apierr.New(apierr.KindForbidden, "only a team owner may invite someone as admin")
	ErrAlreadyMember            = apierr.New(apierr.KindConflict, "already a member of this team")
	ErrInvitationAlreadyPending = apierr.New(apierr.KindConflict, "an invitation is already pending for this account")
	// ErrInvitationNotFound also covers an invitation that exists but belongs
	// to someone else: the same refusal for both, so a valid-looking id is not
	// an existence oracle for another account's invitations.
	ErrInvitationNotFound   = apierr.New(apierr.KindNotFound, "invitation not found")
	ErrInvitationNotPending = apierr.New(apierr.KindConflict, "invitation is no longer pending")
	ErrInvitationExpired    = apierr.New(apierr.KindConflict, "invitation has expired")

	// ErrOnlyOwnerCanChangeRole gates SetMemberRole -- promotion, demotion,
	// and the ownership transfer that results from setting a target to owner.
	ErrOnlyOwnerCanChangeRole = apierr.New(apierr.KindForbidden, "only a team owner may change a member's role")
	// ErrCannotTransferToSelf refuses the one request SetMemberRole cannot
	// give a meaning to: "promote someone to owner while I stay owner too" is
	// not a state this package defines.
	ErrCannotTransferToSelf = apierr.New(apierr.KindInvalid, "cannot transfer ownership to yourself")
	// ErrCannotDemoteLastOwner is the team-scoped version of the rule
	// system-administration.md §6 applies to the last system_admin grant: the
	// API refuses to leave a team with none. Transfer first.
	ErrCannotDemoteLastOwner = apierr.New(apierr.KindConflict, "the last owner cannot be demoted; transfer ownership first")
	// ErrUnsupportedMemberRole is ErrUnsupportedRole's counterpart for
	// SetMemberRole, which -- unlike an invitation -- does accept owner.
	ErrUnsupportedMemberRole = apierr.New(apierr.KindInvalid, "role must be owner, admin, or member")

	// ErrOnlyOwnerCanIssueMemberLoginCode gates the one place in this package
	// a login code is issued: a locked-out member of the caller's own team.
	ErrOnlyOwnerCanIssueMemberLoginCode = apierr.New(apierr.KindForbidden, "only a team owner may issue a login code for a member")
	ErrLoginCodesNotConfigured          = apierr.New(apierr.KindNotConfigured, "login codes not configured")
	// ErrTargetAccountDisabled mirrors the admin route's refusal: a code for
	// an account that cannot use it would be a way in that opens nothing, and
	// an owner would reasonably read success as "they can sign in now".
	ErrTargetAccountDisabled = apierr.New(apierr.KindConflict, "the account is disabled; a system administrator must enable it first")
)

type Service struct {
	Teams coreteam.Store
	Users coreidentity.UserStore
	// LoginCodes backs IssueMemberLoginCode only -- the one place in this
	// package a credential is issued, and only for a member of the caller's
	// own team. Nil leaves that route unavailable, which is what a deployment
	// with no login-code store has.
	LoginCodes coreidentity.LoginCodeStore
	// Now is the clock. Nil means time.Now. Tests set it to pin an invitation's
	// expiry rather than waiting on InvitationTTLDefault.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

// Member pairs a membership with the account behind it. User is nil when the
// deployment has no user store to resolve it against.
type Member struct {
	Membership coreteam.Member
	User       *coreidentity.User
}

type InviteMemberCmd struct {
	TeamID string
	// ActorID is the caller, who must hold ActionInviteTeamMember.
	ActorID string
	Email   string
	// Role defaults to member. Only member and admin are accepted; owner
	// moves through SetMemberRole instead -- see
	// docs/design/team-membership-lifecycle.md §5.2.
	Role string
}

type RevokeInvitationCmd struct {
	// TeamID is the path's team, checked first: a caller's permission is
	// decided from the team they named, never from a team the invitation
	// happens to resolve to. See RevokeInvitation.
	TeamID       string
	InvitationID string
	ActorID      string
}

type AcceptInvitationCmd struct {
	InvitationID string
	// ActorID is the signed-in caller. Accepting requires no code: the
	// caller already reached a session on their own, so authorization is "this
	// is my own pending row," not proving anything a second time.
	ActorID string
}

type RemoveMemberCmd struct {
	TeamID       string
	ActorID      string
	TargetUserID string
}

type SetMemberRoleCmd struct {
	TeamID       string
	ActorID      string
	TargetUserID string
	// Role is owner, admin, or member. Setting owner is ownership transfer:
	// the caller is demoted to admin in the same call -- see SetMemberRole.
	Role string
}

type IssueMemberLoginCodeCmd struct {
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

// InviteMember creates a pending invitation for an email that already has an
// account. It never creates one -- see
// docs/design/team-membership-lifecycle.md §1 for why account creation and
// team membership are kept as two different authorities.
func (s *Service) InviteMember(ctx context.Context, cmd InviteMemberCmd) (*coreteam.Invitation, *coreidentity.User, error) {
	if s.Teams == nil {
		return nil, nil, ErrTeamsNotConfigured
	}
	if s.Users == nil {
		return nil, nil, ErrUsersNotConfigured
	}

	// Permission is checked before any input is validated, matching
	// AddMember's original order: a caller who may not invite at all should
	// not learn anything about why their request would otherwise have been
	// rejected.
	members, err := s.Teams.ListTeamMembers(ctx, cmd.TeamID)
	if err != nil {
		return nil, nil, err
	}
	callerRole := roleOf(members, cmd.ActorID)
	if callerRole == "" || !coreteam.Allows(coreteam.EffectiveRole(callerRole), coreteam.ActionInviteTeamMember) {
		return nil, nil, ErrOnlyOwnerOrAdminCanInvite
	}

	email := strings.TrimSpace(strings.ToLower(cmd.Email))
	if email == "" {
		return nil, nil, ErrEmailRequired
	}
	role := strings.TrimSpace(cmd.Role)
	if role == "" {
		role = coreteam.RoleMember
	}
	if role != coreteam.RoleMember && role != coreteam.RoleAdmin {
		return nil, nil, ErrUnsupportedRole
	}
	// Admin may invite, but only at member -- inviting a peer admin is the one
	// escalation this action must not permit. See core/team/policy.go.
	if role == coreteam.RoleAdmin && coreteam.EffectiveRole(callerRole) != coreteam.RoleOwner {
		return nil, nil, ErrOnlyOwnerCanInviteAdmin
	}

	user, err := s.Users.UserByEmail(ctx, email)
	if err != nil {
		return nil, nil, err
	}
	if user == nil {
		return nil, nil, ErrInviteeAccountRequired
	}
	if isMember(members, user.ID) {
		return nil, nil, ErrAlreadyMember
	}
	now := s.now()
	pending, err := s.Teams.ListPendingInvitationsByTeam(ctx, cmd.TeamID, now)
	if err != nil {
		return nil, nil, err
	}
	for i := range pending {
		if pending[i].UserID == user.ID {
			return nil, nil, ErrInvitationAlreadyPending
		}
	}

	inv, err := s.Teams.CreateInvitation(ctx, cmd.TeamID, user.ID, role, cmd.ActorID, now.Add(coreteam.InvitationTTLDefault))
	if err != nil {
		return nil, nil, fmt.Errorf("create invitation: %w", err)
	}
	return inv, user, nil
}

// ListTeamInvitations returns a team's pending invitations. Reading who has
// been invited is the same authority as sending or revoking one.
func (s *Service) ListTeamInvitations(ctx context.Context, teamID, actorID string) ([]coreteam.Invitation, error) {
	if s.Teams == nil {
		return nil, ErrTeamsNotConfigured
	}
	members, err := s.Teams.ListTeamMembers(ctx, teamID)
	if err != nil {
		return nil, err
	}
	if !allows(members, actorID, coreteam.ActionInviteTeamMember) {
		return nil, ErrOnlyOwnerOrAdminCanInvite
	}
	return s.Teams.ListPendingInvitationsByTeam(ctx, teamID, s.now())
}

// ListMyInvitations answers GET /api/invitations: what is pending for the
// signed-in caller, across every team.
func (s *Service) ListMyInvitations(ctx context.Context, userID string) ([]coreteam.Invitation, error) {
	if s.Teams == nil {
		return nil, ErrTeamsNotConfigured
	}
	return s.Teams.ListPendingInvitationsByUser(ctx, userID, s.now())
}

// RevokeInvitation withdraws a pending invitation before it is accepted.
//
// Permission is decided from cmd.TeamID -- the path the caller actually
// named -- before the invitation is even looked up. Deciding it from the
// invitation's own team instead would let an unresolvable id (a typo, or one
// from another team) skip the permission check entirely and answer "not
// found" to a caller who was never authorized to ask.
func (s *Service) RevokeInvitation(ctx context.Context, cmd RevokeInvitationCmd) error {
	if s.Teams == nil {
		return ErrTeamsNotConfigured
	}
	members, err := s.Teams.ListTeamMembers(ctx, cmd.TeamID)
	if err != nil {
		return err
	}
	if !allows(members, cmd.ActorID, coreteam.ActionInviteTeamMember) {
		return ErrOnlyOwnerOrAdminCanInvite
	}
	inv, err := s.Teams.GetInvitation(ctx, cmd.InvitationID)
	if err != nil {
		return err
	}
	// Not found also covers an invitation that resolves but belongs to a
	// different team than the one named in the path -- the same refusal
	// either way, for the reason ErrInvitationNotFound already states.
	if inv == nil || inv.TeamID != cmd.TeamID {
		return ErrInvitationNotFound
	}
	now := s.now()
	if !inv.Pending(now) {
		return ErrInvitationNotPending
	}
	return s.Teams.RevokeInvitation(ctx, cmd.InvitationID, now)
}

// AcceptInvitation activates a pending invitation for the caller it names.
// It takes no code: the caller already reached a session on their own, so
// this is authorized by "this is my own pending row," not by proving
// anything a second time. See docs/design/team-membership-lifecycle.md §5.1.
func (s *Service) AcceptInvitation(ctx context.Context, cmd AcceptInvitationCmd) (*coreteam.Invitation, error) {
	if s.Teams == nil {
		return nil, ErrTeamsNotConfigured
	}
	inv, err := s.Teams.GetInvitation(ctx, cmd.InvitationID)
	if err != nil {
		return nil, err
	}
	// The same refusal whether the invitation does not exist or belongs to
	// someone else -- see ErrInvitationNotFound.
	if inv == nil || inv.UserID != cmd.ActorID {
		return nil, ErrInvitationNotFound
	}
	now := s.now()
	if inv.AcceptedAt != nil || inv.RevokedAt != nil {
		return nil, ErrInvitationNotPending
	}
	if !now.Before(inv.ExpiresAt) {
		return nil, ErrInvitationExpired
	}
	accepted, err := s.Teams.AcceptInvitation(ctx, cmd.InvitationID, now)
	if err != nil {
		return nil, fmt.Errorf("accept invitation: %w", err)
	}
	if accepted == nil {
		return nil, ErrInvitationNotPending
	}
	return accepted, nil
}

func (s *Service) RemoveMember(ctx context.Context, cmd RemoveMemberCmd) error {
	if s.Teams == nil {
		return ErrTeamsNotConfigured
	}
	members, err := s.Teams.ListTeamMembers(ctx, cmd.TeamID)
	if err != nil {
		return err
	}
	if !allows(members, cmd.ActorID, coreteam.ActionManageTeamMembers) {
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

// SetMemberRole promotes or demotes a member. Setting a target's role to
// owner is ownership transfer -- exposed as this one endpoint rather than a
// separate one, because "promote someone to owner while I stay owner too" is
// not a state this package defines a meaning for, so the caller is demoted to
// admin in the same call. Transfer is unilateral and immediate, not subject
// to the target's acceptance -- see
// docs/design/team-membership-lifecycle.md §5.2-§5.3.
func (s *Service) SetMemberRole(ctx context.Context, cmd SetMemberRoleCmd) error {
	if s.Teams == nil {
		return ErrTeamsNotConfigured
	}
	members, err := s.Teams.ListTeamMembers(ctx, cmd.TeamID)
	if err != nil {
		return err
	}
	if !allows(members, cmd.ActorID, coreteam.ActionChangeMemberRole) {
		return ErrOnlyOwnerCanChangeRole
	}
	role := strings.TrimSpace(cmd.Role)
	if role != coreteam.RoleOwner && role != coreteam.RoleAdmin && role != coreteam.RoleMember {
		return ErrUnsupportedMemberRole
	}
	if !isMember(members, cmd.TargetUserID) {
		return ErrMemberNotFound
	}

	if role == coreteam.RoleOwner {
		if cmd.TargetUserID == cmd.ActorID {
			return ErrCannotTransferToSelf
		}
		return s.Teams.TransferOwnership(ctx, cmd.TeamID, cmd.ActorID, cmd.TargetUserID)
	}

	// Demoting the team's only owner -- including the owner demoting
	// themselves -- must go through transfer first, or the team is left with
	// nobody who can administer it.
	if coreteam.EffectiveRole(roleOf(members, cmd.TargetUserID)) == coreteam.RoleOwner && countOwners(members) <= 1 {
		return ErrCannotDemoteLastOwner
	}
	_, err = s.Teams.AddTeamMember(ctx, cmd.TeamID, cmd.TargetUserID, role)
	return err
}

// IssueMemberLoginCode issues a login code for a locked-out member of the
// caller's own team. It removes the dependency on a system_admin existing at
// all for the common case of one member locked out of an otherwise healthy
// team -- see docs/design/team-membership-lifecycle.md §5.4. It does not
// replace system-administration.md's deployment-scoped route, which is what
// recovers an owner who has no co-owner and no admin left in their own team.
func (s *Service) IssueMemberLoginCode(ctx context.Context, cmd IssueMemberLoginCodeCmd) (string, time.Time, error) {
	if s.Teams == nil {
		return "", time.Time{}, ErrTeamsNotConfigured
	}
	if s.LoginCodes == nil {
		return "", time.Time{}, ErrLoginCodesNotConfigured
	}
	members, err := s.Teams.ListTeamMembers(ctx, cmd.TeamID)
	if err != nil {
		return "", time.Time{}, err
	}
	// ActionManageTeamMembers, not a new action: helping a locked-out member
	// back in is a membership-management act like adding or removing one.
	if !allows(members, cmd.ActorID, coreteam.ActionManageTeamMembers) {
		return "", time.Time{}, ErrOnlyOwnerCanIssueMemberLoginCode
	}
	if !isMember(members, cmd.TargetUserID) {
		return "", time.Time{}, ErrMemberNotFound
	}
	if s.Users != nil {
		user, err := s.Users.GetUser(ctx, cmd.TargetUserID)
		if err != nil {
			return "", time.Time{}, err
		}
		if user != nil && user.Disabled() {
			return "", time.Time{}, ErrTargetAccountDisabled
		}
	}
	return s.LoginCodes.CreateLoginCode(ctx, cmd.TargetUserID, coreidentity.LoginCodeTTLDefault)
}

func countOwners(members []coreteam.Member) int {
	n := 0
	for i := range members {
		if coreteam.EffectiveRole(members[i].Role) == coreteam.RoleOwner {
			n++
		}
	}
	return n
}

func allows(members []coreteam.Member, userID string, action coreteam.Action) bool {
	for i := range members {
		if members[i].UserID == userID {
			return coreteam.Allows(coreteam.EffectiveRole(members[i].Role), action)
		}
	}
	return false
}

// roleOf returns the stored role for userID, or "" when the roster has no
// membership for them. It exists alongside allows for InviteMember, which
// needs the caller's own role a second time -- to decide whether the
// requested target role is one they may grant -- not only whether the
// invite action itself is allowed.
func roleOf(members []coreteam.Member, userID string) string {
	for i := range members {
		if members[i].UserID == userID {
			return members[i].Role
		}
	}
	return ""
}

func isMember(members []coreteam.Member, userID string) bool {
	for i := range members {
		if members[i].UserID == userID {
			return true
		}
	}
	return false
}
