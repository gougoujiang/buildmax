package db

import (
	"testing"
	"time"

	corespace "github.com/gougoujiang/buildmax/internal/core/space"
)

// The invitation store is three joins, a multi-table transaction, and two
// expiry comparisons the database evaluates rather than Go. None of that can be
// proven against a mock: a join that names the wrong column, a transaction that
// leaves an accepted invitation without its member row, and a DATETIME compared
// in the wrong zone all pass a fake and fail a server.

func pendingInvitation(t *testing.T, s *Store, spaceID, userID, role, inviter string) *corespace.Invitation {
	t.Helper()
	inv, err := s.CreateInvitation(t.Context(), spaceID, userID, role, inviter,
		time.Now().UTC().Add(corespace.InvitationTTLDefault))
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}
	t.Cleanup(func() {
		_ = s.db.Delete(&spaceInvitationRow{}, "public_id = ?", inv.ID).Error
	})
	return inv
}

func TestInvitationAcceptCreatesTheMembership(t *testing.T) {
	s, ctx := newTestStore(t)
	owner := newTestUser(t, s, "invite-owner")
	invitee := newTestUser(t, s, "invite-invitee")
	spaceID := newTestSpace(t, s, owner)

	inv := pendingInvitation(t, s, spaceID, invitee, corespace.RoleAdmin, owner)
	if inv.SpaceID != spaceID || inv.UserID != invitee || inv.InvitedBy != owner {
		t.Fatalf("CreateInvitation returned %+v, want the handles it was given", inv)
	}

	// Both list queries read through the joins, so a wrong one shows up as an
	// invitation that exists but cannot be found from either end.
	bySpace, err := s.ListPendingInvitationsBySpace(ctx, spaceID, time.Now().UTC())
	if err != nil {
		t.Fatalf("ListPendingInvitationsBySpace: %v", err)
	}
	if len(bySpace) != 1 || bySpace[0].ID != inv.ID {
		t.Fatalf("ListPendingInvitationsBySpace = %+v, want the one pending invitation", bySpace)
	}
	byUser, err := s.ListPendingInvitationsByUser(ctx, invitee, time.Now().UTC())
	if err != nil {
		t.Fatalf("ListPendingInvitationsByUser: %v", err)
	}
	if len(byUser) != 1 || byUser[0].ID != inv.ID {
		t.Fatalf("ListPendingInvitationsByUser = %+v, want the one pending invitation", byUser)
	}

	accepted, err := s.AcceptInvitation(ctx, inv.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("AcceptInvitation: %v", err)
	}
	if accepted == nil || accepted.AcceptedAt == nil {
		t.Fatalf("AcceptInvitation = %+v, want an accepted invitation", accepted)
	}

	members, err := s.ListSpaceMembers(ctx, spaceID)
	if err != nil {
		t.Fatalf("ListSpaceMembers: %v", err)
	}
	var got *corespace.Member
	for i := range members {
		if members[i].UserID == invitee {
			got = &members[i]
		}
	}
	if got == nil {
		t.Fatal("accepting created no membership; the transaction did not carry both writes")
	}
	if got.Role != corespace.RoleAdmin {
		t.Errorf("member role = %q, want the invited role %q", got.Role, corespace.RoleAdmin)
	}

	// Accepted is no longer pending, from either end.
	if left, err := s.ListPendingInvitationsByUser(ctx, invitee, time.Now().UTC()); err != nil || len(left) != 0 {
		t.Errorf("ListPendingInvitationsByUser after accept = %+v (err %v), want none", left, err)
	}
}

// A second accept must not re-run the membership write. The invitation is the
// record that it already happened.
func TestInvitationAcceptIsIdempotent(t *testing.T) {
	s, ctx := newTestStore(t)
	owner := newTestUser(t, s, "invite-twice-owner")
	invitee := newTestUser(t, s, "invite-twice-invitee")
	spaceID := newTestSpace(t, s, owner)

	inv := pendingInvitation(t, s, spaceID, invitee, corespace.RoleMember, owner)
	if _, err := s.AcceptInvitation(ctx, inv.ID, time.Now().UTC()); err != nil {
		t.Fatalf("first accept: %v", err)
	}
	again, err := s.AcceptInvitation(ctx, inv.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("second accept: %v", err)
	}
	if again != nil {
		t.Errorf("second accept = %+v, want nil for an invitation already spent", again)
	}
	members, err := s.ListSpaceMembers(ctx, spaceID)
	if err != nil {
		t.Fatalf("ListSpaceMembers: %v", err)
	}
	seen := 0
	for _, m := range members {
		if m.UserID == invitee {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("invitee holds %d memberships, want exactly 1", seen)
	}
}

func TestInvitationRevokeStopsAcceptance(t *testing.T) {
	s, ctx := newTestStore(t)
	owner := newTestUser(t, s, "revoke-owner")
	invitee := newTestUser(t, s, "revoke-invitee")
	spaceID := newTestSpace(t, s, owner)

	inv := pendingInvitation(t, s, spaceID, invitee, corespace.RoleMember, owner)
	if err := s.RevokeInvitation(ctx, inv.ID, time.Now().UTC()); err != nil {
		t.Fatalf("RevokeInvitation: %v", err)
	}
	if pending, err := s.ListPendingInvitationsByUser(ctx, invitee, time.Now().UTC()); err != nil || len(pending) != 0 {
		t.Errorf("a revoked invitation is still pending: %+v (err %v)", pending, err)
	}
	accepted, err := s.AcceptInvitation(ctx, inv.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("AcceptInvitation: %v", err)
	}
	if accepted != nil {
		t.Fatal("a revoked invitation was accepted")
	}
	members, err := s.ListSpaceMembers(ctx, spaceID)
	if err != nil {
		t.Fatalf("ListSpaceMembers: %v", err)
	}
	for _, m := range members {
		if m.UserID == invitee {
			t.Fatal("a revoked invitation created a membership")
		}
	}
}

// Expiry is decided by comparing a stored DATETIME, which carries no zone. The
// driver session and the server have to agree on how to read it -- utcDSN is
// where that is pinned, and this is the test that would notice if it stopped
// holding.
func TestInvitationExpiryIsRefusedAndUnlisted(t *testing.T) {
	s, ctx := newTestStore(t)
	owner := newTestUser(t, s, "expiry-owner")
	invitee := newTestUser(t, s, "expiry-invitee")
	spaceID := newTestSpace(t, s, owner)

	past := time.Now().UTC().Add(-time.Minute)
	inv, err := s.CreateInvitation(ctx, spaceID, invitee, corespace.RoleMember, owner, past)
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}
	t.Cleanup(func() { _ = s.db.Delete(&spaceInvitationRow{}, "public_id = ?", inv.ID).Error })

	if pending, err := s.ListPendingInvitationsByUser(ctx, invitee, time.Now().UTC()); err != nil || len(pending) != 0 {
		t.Errorf("an expired invitation is listed as pending: %+v (err %v)", pending, err)
	}
	accepted, err := s.AcceptInvitation(ctx, inv.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("AcceptInvitation: %v", err)
	}
	if accepted != nil {
		t.Error("an expired invitation was accepted")
	}
}

// One account can hold invitations from unrelated spaces at once, and accepting
// one must not touch the other. See design/space-membership-lifecycle.md §3.
func TestInvitationsFromTwoSpacesAreIndependent(t *testing.T) {
	s, ctx := newTestStore(t)
	owner := newTestUser(t, s, "two-spaces-owner")
	invitee := newTestUser(t, s, "two-spaces-invitee")
	first := newTestSpace(t, s, owner)
	second := newTestSpace(t, s, owner)

	firstInv := pendingInvitation(t, s, first, invitee, corespace.RoleMember, owner)
	secondInv := pendingInvitation(t, s, second, invitee, corespace.RoleAdmin, owner)

	pending, err := s.ListPendingInvitationsByUser(ctx, invitee, time.Now().UTC())
	if err != nil {
		t.Fatalf("ListPendingInvitationsByUser: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want both invitations", len(pending))
	}

	if _, err := s.AcceptInvitation(ctx, firstInv.ID, time.Now().UTC()); err != nil {
		t.Fatalf("AcceptInvitation: %v", err)
	}
	left, err := s.ListPendingInvitationsByUser(ctx, invitee, time.Now().UTC())
	if err != nil {
		t.Fatalf("ListPendingInvitationsByUser: %v", err)
	}
	if len(left) != 1 || left[0].ID != secondInv.ID {
		t.Fatalf("after accepting one, pending = %+v, want only the other space's", left)
	}
	// And the space that was not accepted gained no member.
	members, err := s.ListSpaceMembers(ctx, second)
	if err != nil {
		t.Fatalf("ListSpaceMembers: %v", err)
	}
	for _, m := range members {
		if m.UserID == invitee {
			t.Fatal("accepting one space's invitation joined the other space too")
		}
	}
}

// Ownership transfer is one transaction precisely so a space is never read with
// two owners or none. Both rows are asserted, not just the new owner's.
func TestTransferOwnershipMovesExactlyOneOwner(t *testing.T) {
	s, ctx := newTestStore(t)
	owner := newTestUser(t, s, "transfer-owner")
	successor := newTestUser(t, s, "transfer-successor")
	spaceID := newTestSpace(t, s, owner)

	if _, err := s.AddSpaceMember(ctx, spaceID, successor, corespace.RoleMember); err != nil {
		t.Fatalf("AddSpaceMember: %v", err)
	}
	if err := s.TransferOwnership(ctx, spaceID, owner, successor); err != nil {
		t.Fatalf("TransferOwnership: %v", err)
	}

	members, err := s.ListSpaceMembers(ctx, spaceID)
	if err != nil {
		t.Fatalf("ListSpaceMembers: %v", err)
	}
	roles := make(map[string]string, len(members))
	owners := 0
	for _, m := range members {
		roles[m.UserID] = m.Role
		if m.Role == corespace.RoleOwner {
			owners++
		}
	}
	if owners != 1 {
		t.Errorf("space has %d owners after a transfer, want exactly 1: %+v", owners, roles)
	}
	if roles[successor] != corespace.RoleOwner {
		t.Errorf("successor role = %q, want owner", roles[successor])
	}
	if roles[owner] != corespace.RoleAdmin {
		t.Errorf("former owner role = %q, want admin", roles[owner])
	}
}
