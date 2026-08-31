package team_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/mock"
	"github.com/gougoujiang/buildmax/internal/service/team"
)

// owner plus one member, which is the shape every rule here is about.
func newTeam(t *testing.T) (*team.Service, string, string, string) {
	t.Helper()
	teams := &mock.MockTeamStore{}
	users := &mock.MockUserStore{}
	ctx := context.Background()

	owner, err := users.CreateUser(ctx, "owner@example.com", "free")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	member, err := users.CreateUser(ctx, "member@example.com", "free")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	tm, err := teams.CreateTeam(ctx, "acme", owner.ID, "free")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if _, err := teams.AddTeamMember(ctx, tm.ID, member.ID, coreteam.RoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	return &team.Service{Teams: teams, Users: users}, tm.ID, owner.ID, member.ID
}

// addMemberWithRole seats an account in the team at a role the service itself
// will not grant through InviteMember, which is the only way to test what
// that role may do.
func addMemberWithRole(t *testing.T, s *team.Service, teamID, email, role string) string {
	t.Helper()
	ctx := context.Background()
	users, ok := s.Users.(*mock.MockUserStore)
	if !ok {
		t.Fatalf("fixture user store is %T", s.Users)
	}
	teams, ok := s.Teams.(*mock.MockTeamStore)
	if !ok {
		t.Fatalf("fixture team store is %T", s.Teams)
	}
	user, err := users.CreateUser(ctx, email, "free")
	if err != nil {
		t.Fatalf("CreateUser %s: %v", email, err)
	}
	if _, err := teams.AddTeamMember(ctx, teamID, user.ID, role); err != nil {
		t.Fatalf("AddTeamMember %s: %v", role, err)
	}
	return user.ID
}

// createAccount registers an account with no team, which is InviteMember's
// precondition: the address must already exist.
func createAccount(t *testing.T, s *team.Service, email string) string {
	t.Helper()
	users, ok := s.Users.(*mock.MockUserStore)
	if !ok {
		t.Fatalf("fixture user store is %T", s.Users)
	}
	user, err := users.CreateUser(context.Background(), email, "free")
	if err != nil {
		t.Fatalf("CreateUser %s: %v", email, err)
	}
	return user.ID
}

// The owner check existed twice, once per mutating handler. These two cases are
// what those copies were each meant to enforce.
func TestOnlyOwnersMayInviteOrRemove(t *testing.T) {
	s, teamID, _, memberID := newTeam(t)
	ctx := context.Background()
	createAccount(t, s, "new@example.com")

	_, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: teamID, ActorID: memberID, Email: "new@example.com"})
	if !errors.Is(err, team.ErrOnlyOwnerOrAdminCanInvite) {
		t.Errorf("invite by a member: %v, want ErrOnlyOwnerOrAdminCanInvite", err)
	}
	if kind, _ := apierr.KindOf(err); kind != apierr.KindForbidden {
		t.Errorf("kind = %q, want forbidden", kind)
	}

	err = s.RemoveMember(ctx, team.RemoveMemberCmd{TeamID: teamID, ActorID: memberID, TargetUserID: memberID})
	if !errors.Is(err, team.ErrOnlyOwnerCanRemove) {
		t.Errorf("remove by a member: %v, want ErrOnlyOwnerCanRemove", err)
	}
}

// TestAdminMayInviteAtMemberRoleOnly is the half the role matrix in core/team
// cannot show on its own: Allows answers about the caller's own role, not
// about the role they are trying to grant. An admin holds ActionInviteTeamMember
// but the service still refuses the one escalation it would otherwise permit --
// staffing the team with a peer admin.
func TestAdminMayInviteAtMemberRoleOnly(t *testing.T) {
	s, teamID, _, _ := newTeam(t)
	ctx := context.Background()
	adminID := addMemberWithRole(t, s, teamID, "admin@example.com", coreteam.RoleAdmin)
	createAccount(t, s, "member2@example.com")
	createAccount(t, s, "admin2@example.com")

	inv, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: teamID, ActorID: adminID, Email: "member2@example.com"})
	if err != nil {
		t.Fatalf("admin inviting at member role: %v", err)
	}
	if inv.Role != coreteam.RoleMember {
		t.Errorf("role = %q, want member", inv.Role)
	}

	_, _, err = s.InviteMember(ctx, team.InviteMemberCmd{
		TeamID: teamID, ActorID: adminID, Email: "admin2@example.com", Role: coreteam.RoleAdmin,
	})
	if !errors.Is(err, team.ErrOnlyOwnerCanInviteAdmin) {
		t.Errorf("admin inviting at admin role: %v, want ErrOnlyOwnerCanInviteAdmin", err)
	}
}

// An owner may invite at either grantable role.
func TestOwnerMayInviteAtEitherRole(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)
	ctx := context.Background()
	createAccount(t, s, "future-admin@example.com")

	inv, _, err := s.InviteMember(ctx, team.InviteMemberCmd{
		TeamID: teamID, ActorID: ownerID, Email: "future-admin@example.com", Role: coreteam.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("owner inviting at admin role: %v", err)
	}
	if inv.Role != coreteam.RoleAdmin {
		t.Errorf("role = %q, want admin", inv.Role)
	}
}

// A stranger is refused for the same reason a member is: not an owner or
// admin here.
func TestNonMemberMayNotInvite(t *testing.T) {
	s, teamID, _, _ := newTeam(t)
	createAccount(t, s, "new@example.com")

	_, _, err := s.InviteMember(context.Background(), team.InviteMemberCmd{
		TeamID: teamID, ActorID: "u_stranger", Email: "new@example.com",
	})

	if !errors.Is(err, team.ErrOnlyOwnerOrAdminCanInvite) {
		t.Errorf("err = %v, want ErrOnlyOwnerOrAdminCanInvite", err)
	}
}

func TestOwnerCannotRemoveThemselves(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)

	err := s.RemoveMember(context.Background(), team.RemoveMemberCmd{
		TeamID: teamID, ActorID: ownerID, TargetUserID: ownerID,
	})

	if !errors.Is(err, team.ErrCannotRemoveSelf) {
		t.Fatalf("err = %v, want ErrCannotRemoveSelf", err)
	}
}

func TestRemovingSomeoneNotInTheTeamIsNotFound(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)

	err := s.RemoveMember(context.Background(), team.RemoveMemberCmd{
		TeamID: teamID, ActorID: ownerID, TargetUserID: "u_absent",
	})

	if !errors.Is(err, team.ErrMemberNotFound) {
		t.Fatalf("err = %v, want ErrMemberNotFound", err)
	}
	if kind, _ := apierr.KindOf(err); kind != apierr.KindNotFound {
		t.Errorf("kind = %q, want not_found", kind)
	}
}

func TestOwnerRemovesAMember(t *testing.T) {
	s, teamID, ownerID, memberID := newTeam(t)
	ctx := context.Background()

	if err := s.RemoveMember(ctx, team.RemoveMemberCmd{TeamID: teamID, ActorID: ownerID, TargetUserID: memberID}); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}

	members, err := s.ListMembers(ctx, teamID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	for _, m := range members {
		if m.Membership.UserID == memberID {
			t.Fatal("the member is still on the roster")
		}
	}
}

// Only member and admin are grantable: owner moves through SetMemberRole
// instead of an invitation.
func TestOwnerRoleIsNotGrantableThroughInvitation(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)
	createAccount(t, s, "member@example2.com")

	_, _, err := s.InviteMember(context.Background(), team.InviteMemberCmd{
		TeamID: teamID, ActorID: ownerID, Email: "member@example2.com", Role: coreteam.RoleOwner,
	})
	if !errors.Is(err, team.ErrUnsupportedRole) {
		t.Errorf("role owner: %v, want ErrUnsupportedRole", err)
	}
}

func TestInviteRequiresAnExistingAccount(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)
	ctx := context.Background()

	_, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: teamID, ActorID: ownerID, Email: ""})
	if !errors.Is(err, team.ErrEmailRequired) {
		t.Errorf("empty email: %v, want ErrEmailRequired", err)
	}

	_, _, err = s.InviteMember(ctx, team.InviteMemberCmd{TeamID: teamID, ActorID: ownerID, Email: "nobody@example.com"})
	if !errors.Is(err, team.ErrInviteeAccountRequired) {
		t.Errorf("unknown email: %v, want ErrInviteeAccountRequired", err)
	}
	if kind, _ := apierr.KindOf(err); kind != apierr.KindInvalid {
		t.Errorf("kind = %q, want invalid", kind)
	}
}

func TestCannotInviteSomeoneAlreadyOnTheTeam(t *testing.T) {
	s, teamID, ownerID, memberID := newTeam(t)
	user, err := s.Users.(*mock.MockUserStore).GetUser(context.Background(), memberID)
	if err != nil || user == nil {
		t.Fatalf("resolve member: %v", err)
	}

	_, _, err = s.InviteMember(context.Background(), team.InviteMemberCmd{
		TeamID: teamID, ActorID: ownerID, Email: user.Email,
	})
	if !errors.Is(err, team.ErrAlreadyMember) {
		t.Errorf("err = %v, want ErrAlreadyMember", err)
	}
}

func TestInvitingTwiceIsAConflict(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)
	createAccount(t, s, "twice@example.com")
	ctx := context.Background()

	if _, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: teamID, ActorID: ownerID, Email: "twice@example.com"}); err != nil {
		t.Fatalf("first invite: %v", err)
	}
	_, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: teamID, ActorID: ownerID, Email: "twice@example.com"})
	if !errors.Is(err, team.ErrInvitationAlreadyPending) {
		t.Errorf("err = %v, want ErrInvitationAlreadyPending", err)
	}
}

// An address is matched however it was typed.
func TestEmailIsNormalisedBeforeLookup(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)
	createAccount(t, s, "casey@example.com")

	_, user, err := s.InviteMember(context.Background(), team.InviteMemberCmd{
		TeamID: teamID, ActorID: ownerID, Email: "  Casey@Example.COM  ",
	})

	if err != nil {
		t.Fatalf("InviteMember: %v", err)
	}
	if user.Email != "casey@example.com" {
		t.Errorf("Email = %q", user.Email)
	}
}

func TestListMembersResolvesAccounts(t *testing.T) {
	s, teamID, _, _ := newTeam(t)

	members, err := s.ListMembers(context.Background(), teamID)

	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %d, want 2", len(members))
	}
	for _, m := range members {
		if m.User == nil || m.User.ID != m.Membership.UserID {
			t.Errorf("membership %s has no resolved account", m.Membership.UserID)
		}
	}
}

// The whole invitation lifecycle: invite, accept, and the membership it
// produces.
func TestInviteThenAcceptCreatesMembership(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)
	inviteeID := createAccount(t, s, "invitee@example.com")
	ctx := context.Background()

	inv, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: teamID, ActorID: ownerID, Email: "invitee@example.com"})
	if err != nil {
		t.Fatalf("InviteMember: %v", err)
	}

	accepted, err := s.AcceptInvitation(ctx, team.AcceptInvitationCmd{InvitationID: inv.ID, ActorID: inviteeID})
	if err != nil {
		t.Fatalf("AcceptInvitation: %v", err)
	}
	if accepted.AcceptedAt == nil {
		t.Fatal("accepted invitation has no AcceptedAt")
	}

	members, err := s.ListMembers(ctx, teamID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	found := false
	for _, m := range members {
		if m.Membership.UserID == inviteeID {
			found = true
		}
	}
	if !found {
		t.Fatal("invitee is not on the roster after accepting")
	}

	pending, err := s.Teams.ListPendingInvitationsByUser(ctx, inviteeID, time.Now().UTC())
	if err != nil {
		t.Fatalf("ListPendingInvitationsByUser: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending invitations after accepting = %d, want 0", len(pending))
	}
}

// Only the invited account may accept -- a valid-looking id belonging to
// somebody else is refused the same way a nonexistent one is.
func TestOnlyTheInviteeMayAccept(t *testing.T) {
	s, teamID, ownerID, memberID := newTeam(t)
	createAccount(t, s, "invitee@example.com")
	ctx := context.Background()

	inv, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: teamID, ActorID: ownerID, Email: "invitee@example.com"})
	if err != nil {
		t.Fatalf("InviteMember: %v", err)
	}

	_, err = s.AcceptInvitation(ctx, team.AcceptInvitationCmd{InvitationID: inv.ID, ActorID: memberID})
	if !errors.Is(err, team.ErrInvitationNotFound) {
		t.Errorf("err = %v, want ErrInvitationNotFound", err)
	}
}

func TestAcceptingAnExpiredInvitationIsRefused(t *testing.T) {
	teams := &mock.MockTeamStore{}
	users := &mock.MockUserStore{}
	ctx := context.Background()
	owner, err := users.CreateUser(ctx, "owner@example.com", "free")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	invitee, err := users.CreateUser(ctx, "invitee@example.com", "free")
	if err != nil {
		t.Fatalf("CreateUser invitee: %v", err)
	}
	tm, err := teams.CreateTeam(ctx, "acme", owner.ID, "free")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	s := &team.Service{Teams: teams, Users: users, Now: func() time.Time { return past }}
	inv, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: tm.ID, ActorID: owner.ID, Email: "invitee@example.com"})
	if err != nil {
		t.Fatalf("InviteMember: %v", err)
	}

	live := &team.Service{Teams: teams, Users: users, Now: func() time.Time { return past.Add(coreteam.InvitationTTLDefault + time.Second) }}
	_, err = live.AcceptInvitation(ctx, team.AcceptInvitationCmd{InvitationID: inv.ID, ActorID: invitee.ID})
	if !errors.Is(err, team.ErrInvitationExpired) {
		t.Errorf("err = %v, want ErrInvitationExpired", err)
	}
}

func TestOwnerRevokesAPendingInvitation(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)
	inviteeID := createAccount(t, s, "invitee@example.com")
	ctx := context.Background()

	inv, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: teamID, ActorID: ownerID, Email: "invitee@example.com"})
	if err != nil {
		t.Fatalf("InviteMember: %v", err)
	}
	if err := s.RevokeInvitation(ctx, team.RevokeInvitationCmd{TeamID: teamID, InvitationID: inv.ID, ActorID: ownerID}); err != nil {
		t.Fatalf("RevokeInvitation: %v", err)
	}

	_, err = s.AcceptInvitation(ctx, team.AcceptInvitationCmd{InvitationID: inv.ID, ActorID: inviteeID})
	if !errors.Is(err, team.ErrInvitationNotPending) {
		t.Errorf("accepting a revoked invitation: %v, want ErrInvitationNotPending", err)
	}
}

func TestOnlyOwnerOrAdminMayRevoke(t *testing.T) {
	s, teamID, ownerID, memberID := newTeam(t)
	createAccount(t, s, "invitee@example.com")
	ctx := context.Background()

	inv, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: teamID, ActorID: ownerID, Email: "invitee@example.com"})
	if err != nil {
		t.Fatalf("InviteMember: %v", err)
	}
	err = s.RevokeInvitation(ctx, team.RevokeInvitationCmd{TeamID: teamID, InvitationID: inv.ID, ActorID: memberID})
	if !errors.Is(err, team.ErrOnlyOwnerOrAdminCanInvite) {
		t.Errorf("err = %v, want ErrOnlyOwnerOrAdminCanInvite", err)
	}
}

// TestRevokePermissionComesFromThePathTeam pins the fix for a real ordering
// bug: checking the invitation's own team before the caller's permission
// would let an id that resolves nowhere (a typo, or an id spent trying
// another team) skip the permission check and answer "not found" instead of
// "forbidden" to a caller who was never authorized to ask in the first
// place -- see RevokeInvitation.
func TestRevokePermissionComesFromThePathTeam(t *testing.T) {
	s, teamID, ownerID, memberID := newTeam(t)
	ctx := context.Background()
	teams := s.Teams.(*mock.MockTeamStore)
	otherTeam, err := teams.CreateTeam(ctx, "other", ownerID, "free")
	if err != nil {
		t.Fatalf("CreateTeam other: %v", err)
	}

	// A member has no invite permission at all, so even an id that resolves
	// nowhere must still read as forbidden, not "not found".
	err = s.RevokeInvitation(ctx, team.RevokeInvitationCmd{TeamID: teamID, InvitationID: "inv_absent", ActorID: memberID})
	if !errors.Is(err, team.ErrOnlyOwnerOrAdminCanInvite) {
		t.Errorf("member revoking a nonexistent id: %v, want ErrOnlyOwnerOrAdminCanInvite", err)
	}

	// An invitation that belongs to a different team than the one named in
	// the path reads as not found -- an owner of teamID has no standing to
	// revoke otherTeam's invitations by guessing at otherTeam's own path.
	createAccount(t, s, "elsewhere@example.com")
	otherInv, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: otherTeam.ID, ActorID: ownerID, Email: "elsewhere@example.com"})
	if err != nil {
		t.Fatalf("InviteMember otherTeam: %v", err)
	}
	err = s.RevokeInvitation(ctx, team.RevokeInvitationCmd{TeamID: teamID, InvitationID: otherInv.ID, ActorID: ownerID})
	if !errors.Is(err, team.ErrInvitationNotFound) {
		t.Errorf("revoking another team's invitation through this team's path: %v, want ErrInvitationNotFound", err)
	}
}

func TestListMyInvitationsAcrossTeams(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)
	inviteeID := createAccount(t, s, "busy@example.com")
	ctx := context.Background()

	teams := s.Teams.(*mock.MockTeamStore)
	otherTeam, err := teams.CreateTeam(ctx, "other", ownerID, "free")
	if err != nil {
		t.Fatalf("CreateTeam other: %v", err)
	}

	if _, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: teamID, ActorID: ownerID, Email: "busy@example.com"}); err != nil {
		t.Fatalf("InviteMember teamID: %v", err)
	}
	if _, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: otherTeam.ID, ActorID: ownerID, Email: "busy@example.com"}); err != nil {
		t.Fatalf("InviteMember otherTeam: %v", err)
	}

	pending, err := s.ListMyInvitations(ctx, inviteeID)
	if err != nil {
		t.Fatalf("ListMyInvitations: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2", len(pending))
	}
}

func TestOwnerPromotesAndDemotesAMember(t *testing.T) {
	s, teamID, ownerID, memberID := newTeam(t)
	ctx := context.Background()

	before, err := s.ListMembers(ctx, teamID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	var createdAt time.Time
	for _, m := range before {
		if m.Membership.UserID == memberID {
			createdAt = m.Membership.CreatedAt
		}
	}

	if err := s.SetMemberRole(ctx, team.SetMemberRoleCmd{TeamID: teamID, ActorID: ownerID, TargetUserID: memberID, Role: coreteam.RoleAdmin}); err != nil {
		t.Fatalf("promote to admin: %v", err)
	}
	if err := s.SetMemberRole(ctx, team.SetMemberRoleCmd{TeamID: teamID, ActorID: ownerID, TargetUserID: memberID, Role: coreteam.RoleMember}); err != nil {
		t.Fatalf("demote to member: %v", err)
	}

	after, err := s.ListMembers(ctx, teamID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("member count changed = %d, want %d (a role change is not a remove/add pair)", len(after), len(before))
	}
	for _, m := range after {
		if m.Membership.UserID == memberID {
			if m.Membership.Role != coreteam.RoleMember {
				t.Errorf("final role = %q, want member", m.Membership.Role)
			}
			if !m.Membership.CreatedAt.Equal(createdAt) {
				t.Errorf("CreatedAt changed from %v to %v; a role change must not read as a fresh join", createdAt, m.Membership.CreatedAt)
			}
		}
	}
}

func TestOnlyOwnerMayChangeRole(t *testing.T) {
	s, teamID, _, memberID := newTeam(t)
	ctx := context.Background()
	adminID := addMemberWithRole(t, s, teamID, "admin@example.com", coreteam.RoleAdmin)

	for _, actor := range []string{adminID, memberID} {
		err := s.SetMemberRole(ctx, team.SetMemberRoleCmd{TeamID: teamID, ActorID: actor, TargetUserID: memberID, Role: coreteam.RoleAdmin})
		if !errors.Is(err, team.ErrOnlyOwnerCanChangeRole) {
			t.Errorf("actor %s: %v, want ErrOnlyOwnerCanChangeRole", actor, err)
		}
	}
}

func TestOwnerTransfersOwnership(t *testing.T) {
	s, teamID, ownerID, memberID := newTeam(t)
	ctx := context.Background()

	if err := s.SetMemberRole(ctx, team.SetMemberRoleCmd{TeamID: teamID, ActorID: ownerID, TargetUserID: memberID, Role: coreteam.RoleOwner}); err != nil {
		t.Fatalf("transfer: %v", err)
	}

	members, err := s.ListMembers(ctx, teamID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	roles := map[string]string{}
	for _, m := range members {
		roles[m.Membership.UserID] = m.Membership.Role
	}
	if roles[memberID] != coreteam.RoleOwner {
		t.Errorf("new owner role = %q, want owner", roles[memberID])
	}
	if roles[ownerID] != coreteam.RoleAdmin {
		t.Errorf("former owner role = %q, want admin", roles[ownerID])
	}

	// Reversible: the new owner can transfer straight back.
	if err := s.SetMemberRole(ctx, team.SetMemberRoleCmd{TeamID: teamID, ActorID: memberID, TargetUserID: ownerID, Role: coreteam.RoleOwner}); err != nil {
		t.Fatalf("transfer back: %v", err)
	}
}

func TestCannotTransferToSelf(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)

	err := s.SetMemberRole(context.Background(), team.SetMemberRoleCmd{
		TeamID: teamID, ActorID: ownerID, TargetUserID: ownerID, Role: coreteam.RoleOwner,
	})
	if !errors.Is(err, team.ErrCannotTransferToSelf) {
		t.Errorf("err = %v, want ErrCannotTransferToSelf", err)
	}
}

func TestSoleOwnerCannotDemoteThemselvesWithoutTransferring(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)

	err := s.SetMemberRole(context.Background(), team.SetMemberRoleCmd{
		TeamID: teamID, ActorID: ownerID, TargetUserID: ownerID, Role: coreteam.RoleAdmin,
	})
	if !errors.Is(err, team.ErrCannotDemoteLastOwner) {
		t.Errorf("err = %v, want ErrCannotDemoteLastOwner", err)
	}
}

func TestIssueMemberLoginCodeForOwnTeamMember(t *testing.T) {
	s, teamID, ownerID, memberID := newTeam(t)
	s.LoginCodes = &mock.MockLoginCodeStore{}

	code, expiresAt, err := s.IssueMemberLoginCode(context.Background(), team.IssueMemberLoginCodeCmd{
		TeamID: teamID, ActorID: ownerID, TargetUserID: memberID,
	})
	if err != nil {
		t.Fatalf("IssueMemberLoginCode: %v", err)
	}
	if code == "" {
		t.Error("code is empty")
	}
	if !expiresAt.After(time.Now().UTC()) {
		t.Error("expiresAt is not in the future")
	}
}

func TestIssueMemberLoginCodeRefusedForNonOwner(t *testing.T) {
	s, teamID, _, memberID := newTeam(t)
	s.LoginCodes = &mock.MockLoginCodeStore{}
	adminID := addMemberWithRole(t, s, teamID, "admin@example.com", coreteam.RoleAdmin)

	_, _, err := s.IssueMemberLoginCode(context.Background(), team.IssueMemberLoginCodeCmd{
		TeamID: teamID, ActorID: adminID, TargetUserID: memberID,
	})
	if !errors.Is(err, team.ErrOnlyOwnerCanIssueMemberLoginCode) {
		t.Errorf("err = %v, want ErrOnlyOwnerCanIssueMemberLoginCode", err)
	}
}

func TestIssueMemberLoginCodeRefusedOutsideTheCallersTeam(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)
	s.LoginCodes = &mock.MockLoginCodeStore{}

	_, _, err := s.IssueMemberLoginCode(context.Background(), team.IssueMemberLoginCodeCmd{
		TeamID: teamID, ActorID: ownerID, TargetUserID: "u_stranger",
	})
	if !errors.Is(err, team.ErrMemberNotFound) {
		t.Errorf("err = %v, want ErrMemberNotFound", err)
	}
}

func TestIssueMemberLoginCodeRefusedForDisabledAccount(t *testing.T) {
	s, teamID, ownerID, memberID := newTeam(t)
	s.LoginCodes = &mock.MockLoginCodeStore{}
	users := s.Users.(*mock.MockUserStore)
	disabledAt := time.Now().UTC()
	if err := users.SetUserDisabled(context.Background(), memberID, &disabledAt); err != nil {
		t.Fatalf("SetUserDisabled: %v", err)
	}

	_, _, err := s.IssueMemberLoginCode(context.Background(), team.IssueMemberLoginCodeCmd{
		TeamID: teamID, ActorID: ownerID, TargetUserID: memberID,
	})
	if !errors.Is(err, team.ErrTargetAccountDisabled) {
		t.Errorf("err = %v, want ErrTargetAccountDisabled", err)
	}
}

// A member may not lower or raise the bar every undeclared agent in the team
// runs under; an admin may.
func TestSetSandboxDefaults_MemberForbiddenAdminAllowed(t *testing.T) {
	s, teamID, _, memberID := newTeam(t)
	ctx := context.Background()
	adminID := addMemberWithRole(t, s, teamID, "admin@example.com", coreteam.RoleAdmin)

	err := s.SetSandboxDefaults(ctx, team.SetSandboxDefaultsCmd{
		TeamID: teamID, ActorID: memberID, NetworkTier: "registries", FilesystemTier: "workspace",
	})
	if !errors.Is(err, team.ErrOnlyOwnerOrAdminCanSetSandboxDefaults) {
		t.Errorf("member setting defaults: %v, want ErrOnlyOwnerOrAdminCanSetSandboxDefaults", err)
	}

	err = s.SetSandboxDefaults(ctx, team.SetSandboxDefaultsCmd{
		TeamID: teamID, ActorID: adminID, NetworkTier: "registries", FilesystemTier: "workspace",
	})
	if err != nil {
		t.Fatalf("admin setting defaults: %v", err)
	}
	got, err := s.Teams.GetTeam(ctx, teamID)
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if got.DefaultSandboxNetworkTier != "registries" || got.DefaultSandboxFilesystemTier != "workspace" {
		t.Errorf("team defaults = %+v, want registries/workspace", got)
	}
}

// An unrecognized tier is refused even when the caller may otherwise change
// the setting.
func TestSetSandboxDefaults_InvalidTierRejected(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)
	err := s.SetSandboxDefaults(context.Background(), team.SetSandboxDefaultsCmd{
		TeamID: teamID, ActorID: ownerID, NetworkTier: "not-a-tier",
	})
	if !errors.Is(err, team.ErrInvalidSandboxTier) {
		t.Errorf("err = %v, want ErrInvalidSandboxTier", err)
	}
}

func TestMissingStoresAreReported(t *testing.T) {
	ctx := context.Background()

	if _, err := (&team.Service{}).ListMembers(ctx, "tm_1"); !errors.Is(err, team.ErrTeamsNotConfigured) {
		t.Errorf("no team store: %v", err)
	}
	// A team store with no user store cannot resolve an email to an account.
	s := &team.Service{Teams: &mock.MockTeamStore{}}
	_, _, err := s.InviteMember(ctx, team.InviteMemberCmd{TeamID: "tm_1", ActorID: "u_1", Email: "a@b.c"})
	if !errors.Is(err, team.ErrUsersNotConfigured) {
		t.Errorf("no user store: %v, want ErrUsersNotConfigured", err)
	}
	if kind, _ := apierr.KindOf(err); kind != apierr.KindNotConfigured {
		t.Errorf("kind = %q, want not_configured", kind)
	}
}
