package systemadmin

import (
	"context"
	"errors"
	"testing"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	coreidentity "github.com/gougoujiang/buildmax/internal/core/identity"
	"github.com/gougoujiang/buildmax/internal/mock"
)

func newService(t *testing.T) (*Service, string) {
	t.Helper()
	users := &mock.MockUserStore{}
	user, err := users.CreateUser(context.Background(), "admin@example.test", "free")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	grants := &mock.MockSystemGrantStore{}
	grants.GrantForTest(user.ID, coreidentity.SystemRoleAdmin)
	return &Service{Grants: grants, Users: users}, user.ID
}

// TestRevokingTheLastHolderTurnsOnAuthority is the rule this service exists to
// hold in one place. It used to live twice: the HTTP handler counted holders
// and refused, the shell command counted them and printed a warning.
//
// Which one applies is decided by who is asking, not by a flag the asker sets.
// A signed-in caller reaches the route through the very grant they would be
// removing, so taking the last one locks the deployment out of its own admin
// area with no way back through that door. The shell is the way back.
func TestRevokingTheLastHolderTurnsOnAuthority(t *testing.T) {
	t.Run("a signed-in caller is refused", func(t *testing.T) {
		svc, userID := newService(t)
		err := svc.Revoke(context.Background(), userID, coreidentity.SystemRoleAdmin, coreaudit.UserActor("u_someone"))
		if !errors.Is(err, ErrLastHolder) {
			t.Fatalf("err = %v, want ErrLastHolder", err)
		}
	})

	t.Run("the shell is permitted", func(t *testing.T) {
		svc, userID := newService(t)
		if err := svc.Revoke(context.Background(), userID, coreidentity.SystemRoleAdmin, coreaudit.OperatorActor()); err != nil {
			t.Fatalf("the recovery path was refused: %v", err)
		}
		remaining, err := svc.RemainingHolders(context.Background(), coreidentity.SystemRoleAdmin)
		if err != nil {
			t.Fatalf("RemainingHolders: %v", err)
		}
		if remaining != 0 {
			t.Errorf("remaining = %d, want 0", remaining)
		}
	})
}

// TestRevokingWhenSomebodyElseHoldsItIsAllowedForEither pins that the rule is
// about the last one, not about signed-in callers revoking at all.
func TestRevokingWhenSomebodyElseHoldsItIsAllowedForEither(t *testing.T) {
	svc, userID := newService(t)
	grants := svc.Grants.(*mock.MockSystemGrantStore)
	grants.GrantForTest("u_second", coreidentity.SystemRoleAdmin)

	if err := svc.Revoke(context.Background(), userID, coreidentity.SystemRoleAdmin, coreaudit.UserActor("u_second")); err != nil {
		t.Fatalf("a signed-in caller could not revoke a non-last grant: %v", err)
	}
}

// TestRevokingWhatNobodyHoldsSaysSo keeps the idempotent answer distinguishable
// from the last-holder refusal: one is nothing to do, the other is a decision.
func TestRevokingWhatNobodyHoldsSaysSo(t *testing.T) {
	svc, _ := newService(t)
	grants := svc.Grants.(*mock.MockSystemGrantStore)
	grants.GrantForTest("u_second", coreidentity.SystemRoleAdmin)

	err := svc.Revoke(context.Background(), "u_nobody", coreidentity.SystemRoleAdmin, coreaudit.UserActor("u_second"))
	if !errors.Is(err, ErrNotHeld) {
		t.Fatalf("err = %v, want ErrNotHeld", err)
	}
}

// TestGrantingToAnAccountThatIsNotThere refuses before minting authority.
func TestGrantingToAnAccountThatIsNotThere(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.Grant(context.Background(), "u_nobody", coreidentity.SystemRoleAdmin, coreaudit.OperatorActor())
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("err = %v, want ErrAccountNotFound", err)
	}
}

// TestGrantingARoleNobodyImplements refuses a role rather than storing one that
// nothing will ever check.
func TestGrantingARoleNobodyImplements(t *testing.T) {
	svc, userID := newService(t)
	_, err := svc.Grant(context.Background(), userID, "root", coreaudit.OperatorActor())
	if !errors.Is(err, ErrUnknownRole) {
		t.Fatalf("err = %v, want ErrUnknownRole", err)
	}
}
