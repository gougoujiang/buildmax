package team_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/apierr"
	"github.com/gougoujiang/buildmax/internal/core/model"
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
	tm, err := teams.CreateTeam(ctx, "acme", owner.UserID, "free")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if _, err := teams.AddTeamMember(ctx, tm.TeamID, member.UserID, model.TeamRoleMember); err != nil {
		t.Fatalf("AddTeamMember: %v", err)
	}
	return &team.Service{Teams: teams, Users: users}, tm.TeamID, owner.UserID, member.UserID
}

// The owner check existed twice, once per mutating handler. These two cases are
// what those copies were each meant to enforce.
func TestOnlyOwnersMayAddOrRemove(t *testing.T) {
	s, teamID, _, memberID := newTeam(t)
	ctx := context.Background()

	_, _, err := s.AddMember(ctx, team.AddMemberCmd{TeamID: teamID, ActorID: memberID, Email: "new@example.com"})
	if !errors.Is(err, team.ErrOnlyOwnerCanAdd) {
		t.Errorf("add by a member: %v, want ErrOnlyOwnerCanAdd", err)
	}
	if kind, _ := apierr.KindOf(err); kind != apierr.KindForbidden {
		t.Errorf("kind = %q, want forbidden", kind)
	}

	err = s.RemoveMember(ctx, team.RemoveMemberCmd{TeamID: teamID, ActorID: memberID, TargetUserID: memberID})
	if !errors.Is(err, team.ErrOnlyOwnerCanRemove) {
		t.Errorf("remove by a member: %v, want ErrOnlyOwnerCanRemove", err)
	}
}

// A stranger is refused for the same reason a member is: not an owner here.
func TestNonMemberIsNotAnOwner(t *testing.T) {
	s, teamID, _, _ := newTeam(t)

	_, _, err := s.AddMember(context.Background(), team.AddMemberCmd{
		TeamID: teamID, ActorID: "u_stranger", Email: "new@example.com",
	})

	if !errors.Is(err, team.ErrOnlyOwnerCanAdd) {
		t.Errorf("err = %v, want ErrOnlyOwnerCanAdd", err)
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

// Only member is grantable here: adding someone as an owner from the same call
// that adds a member would make an escalation look routine.
func TestOnlyTheMemberRoleIsGrantable(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)

	for _, role := range []string{model.TeamRoleOwner, model.TeamRoleAdmin} {
		_, _, err := s.AddMember(context.Background(), team.AddMemberCmd{
			TeamID: teamID, ActorID: ownerID, Email: "member@example.com", Role: role,
		})
		if !errors.Is(err, team.ErrUnsupportedRole) {
			t.Errorf("role %q: %v, want ErrUnsupportedRole", role, err)
		}
	}
}

func TestAddRequiresAnExistingAccount(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)
	ctx := context.Background()

	_, _, err := s.AddMember(ctx, team.AddMemberCmd{TeamID: teamID, ActorID: ownerID, Email: ""})
	if !errors.Is(err, team.ErrEmailRequired) {
		t.Errorf("empty email: %v, want ErrEmailRequired", err)
	}

	_, _, err = s.AddMember(ctx, team.AddMemberCmd{TeamID: teamID, ActorID: ownerID, Email: "nobody@example.com"})
	if !errors.Is(err, team.ErrUserDoesNotExist) {
		t.Errorf("unknown email: %v, want ErrUserDoesNotExist", err)
	}
}

// An address is matched however it was typed.
func TestEmailIsNormalisedBeforeLookup(t *testing.T) {
	s, teamID, ownerID, _ := newTeam(t)

	_, user, err := s.AddMember(context.Background(), team.AddMemberCmd{
		TeamID: teamID, ActorID: ownerID, Email: "  Member@Example.COM  ",
	})

	if err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if user.Email != "member@example.com" {
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
		if m.User == nil || m.User.UserID != m.Membership.UserID {
			t.Errorf("membership %s has no resolved account", m.Membership.UserID)
		}
	}
}

func TestMissingStoresAreReported(t *testing.T) {
	ctx := context.Background()

	if _, err := (&team.Service{}).ListMembers(ctx, "tm_1"); !errors.Is(err, team.ErrTeamsNotConfigured) {
		t.Errorf("no team store: %v", err)
	}
	// A team store with no user store cannot resolve an email to an account.
	s := &team.Service{Teams: &mock.MockTeamStore{}}
	_, _, err := s.AddMember(ctx, team.AddMemberCmd{TeamID: "tm_1", ActorID: "u_1", Email: "a@b.c"})
	if !errors.Is(err, team.ErrUsersNotConfigured) {
		t.Errorf("no user store: %v, want ErrUsersNotConfigured", err)
	}
	if kind, _ := apierr.KindOf(err); kind != apierr.KindNotConfigured {
		t.Errorf("kind = %q, want not_configured", kind)
	}
}
