package team

import (
	"testing"

	"github.com/gougoujiang/buildmax/internal/core/model"
)

// TestAllowsMatrix is the whole rule, written out. A change to Allows that
// nobody meant to make has to edit this table to pass, which is the point: the
// two enforcers drive real requests and real commands, and neither of them can
// show the rule as one piece.
func TestAllowsMatrix(t *testing.T) {
	const (
		owner  = model.TeamRoleOwner
		admin  = model.TeamRoleAdmin
		member = model.TeamRoleMember
	)
	want := map[Action]map[string]bool{
		ActionManageTeamMembers:     {owner: true, admin: false, member: false},
		ActionReadAuditTrail:        {owner: true, admin: false, member: false},
		ActionModerateIssueComments: {owner: true, admin: false, member: false},
		ActionManageAgents:          {owner: true, admin: true, member: false},
		ActionManageWorkflows:       {owner: true, admin: true, member: false},
		ActionAssignIssueWorkflow:   {owner: true, admin: true, member: false},
		ActionRunWorkflow:           {owner: true, admin: true, member: true},
		ActionCommentIssue:          {owner: true, admin: true, member: true},
	}
	for _, action := range Actions() {
		roles, ok := want[action]
		if !ok {
			t.Errorf("%s is an action with no row in this table", action)
			continue
		}
		for role, allowed := range roles {
			if got := Allows(role, action); got != allowed {
				t.Errorf("Allows(%q, %q) = %v, want %v", role, action, got, allowed)
			}
		}
	}
	if len(want) != len(Actions()) {
		t.Errorf("the table has %d rows for %d actions", len(want), len(Actions()))
	}
}

// TestEffectiveRoleReadsAnUnsetRoleAsMember pins the answer to a question the
// two enforcers used to answer differently: the HTTP guard read a membership
// row with no role as "not a member" and refused everything, while its own
// teamRole helper read it as plain membership.
//
// Member is the reading, because the row is what says somebody belongs and
// member is the least the three roles can mean. Nothing can write such a row
// today -- the team service defaults an unset role before storing one -- so
// this is what a legacy row gets, not a path anything takes now.
func TestEffectiveRoleReadsAnUnsetRoleAsMember(t *testing.T) {
	if got := EffectiveRole(""); got != model.TeamRoleMember {
		t.Errorf("EffectiveRole(\"\") = %q, want %q", got, model.TeamRoleMember)
	}
	for _, role := range []string{model.TeamRoleOwner, model.TeamRoleAdmin, model.TeamRoleMember} {
		if got := EffectiveRole(role); got != role {
			t.Errorf("EffectiveRole(%q) = %q, want it unchanged", role, got)
		}
	}
	// A role nobody implements is not rewritten into one that works: Allows
	// refuses it, and turning it into member here would be a silent grant.
	if got := EffectiveRole("root"); got != "root" {
		t.Errorf("EffectiveRole(%q) = %q, want it unchanged", "root", got)
	}
}

// TestEffectiveRoleGrantsOnlyMemberLevelActions is the consequence, stated so a
// widening of what member may do cannot quietly widen what an unset role may do
// without this failing too.
func TestEffectiveRoleGrantsOnlyMemberLevelActions(t *testing.T) {
	unset := EffectiveRole("")
	for _, action := range Actions() {
		want := Allows(model.TeamRoleMember, action)
		if got := Allows(unset, action); got != want {
			t.Errorf("Allows(unset, %q) = %v, want %v (what a member gets)", action, got, want)
		}
	}
	if Allows(unset, ActionManageTeamMembers) {
		t.Error("a row with no role may not change membership")
	}
	if Allows(unset, ActionManageAgents) {
		t.Error("a row with no role may not manage agents")
	}
}

// TestAllowsRefusesWhatItDoesNotKnow pins the direction an unknown value fails
// in. A role or an action nobody wrote a rule for is a rule nobody wrote, and
// answering true would turn a typo into an escalation.
func TestAllowsRefusesWhatItDoesNotKnow(t *testing.T) {
	for _, action := range Actions() {
		for _, role := range []string{"", "  ", "Owner", "OWNER", "root", "system_admin"} {
			if Allows(role, action) {
				t.Errorf("Allows(%q, %q) = true; only the three team roles are roles", role, action)
			}
		}
	}
	for _, action := range []Action{"", "manage", "manage_team_members ", "ManageTeamMembers", "delete_team"} {
		for _, role := range []string{model.TeamRoleOwner, model.TeamRoleAdmin, model.TeamRoleMember} {
			if Allows(role, action) {
				t.Errorf("Allows(%q, %q) = true; an unknown action is not permitted to anyone", role, action)
			}
		}
	}
}
