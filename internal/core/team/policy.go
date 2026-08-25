// Package team owns what a team's roles may do.
//
// The decision is here, and only here, because it has two enforcers: the HTTP
// guard refuses a request before a handler runs, and the team service refuses a
// command whatever called it. Defence in depth is deliberate; two definitions
// of the same rule were not, and the one nobody remembered to update would have
// been the permissive one.
//
// It imports core/model only for the role values. Those move here with the rest
// of the team domain, at which point this import goes away.
package team

import "github.com/gougoujiang/buildmax/internal/core/model"

// An Action is something a caller wants to do to a team, named at the coarseness
// the role rules actually distinguish. It is not one action per route: several
// routes share a permission, and naming the permission rather than the route is
// what stops a new route from arriving without one.
type Action string

const (
	// ActionManageTeamMembers covers adding and removing members. Granting
	// ownership is not part of it: the service refuses any role but member, so
	// an escalation cannot look like a routine invitation.
	ActionManageTeamMembers Action = "manage_team_members"
	ActionManageAgents      Action = "manage_agents"
	ActionManageWorkflows   Action = "manage_workflows"
	// ActionAssignIssueWorkflow is assigning work to a workflow, which is a
	// change to what the team automates rather than a use of it.
	ActionAssignIssueWorkflow Action = "assign_issue_workflow"
	ActionRunWorkflow         Action = "run_workflow"
	ActionReadAuditTrail      Action = "read_audit_trail"
	ActionCommentIssue        Action = "comment_issue"
	// ActionModerateIssueComments covers deleting a comment the caller did not
	// write. Editing another author's comment is permitted to nobody, so it is
	// not an action here — see internal/service/issue.
	ActionModerateIssueComments Action = "moderate_issue_comments"
)

// Actions returns every action, so a test can prove the matrix covers each one
// rather than only the ones somebody remembered.
func Actions() []Action {
	return []Action{
		ActionManageTeamMembers,
		ActionManageAgents,
		ActionManageWorkflows,
		ActionAssignIssueWorkflow,
		ActionRunWorkflow,
		ActionReadAuditTrail,
		ActionCommentIssue,
		ActionModerateIssueComments,
	}
}

// EffectiveRole is what a membership row's stored role means.
//
// A row with no role is a member. The row is what says somebody belongs to the
// team; the role only says how much they may do, and the least of the three is
// what a row that never got one has been given. Reading it as "not a member"
// instead would make a data defect look like an absent membership, and reading
// it as anything higher would let a missing value grant something.
//
// Callers normalize before asking Allows, which answers about a stated role.
func EffectiveRole(role string) string {
	if role == "" {
		return model.TeamRoleMember
	}
	return role
}

// Allows reports whether a member holding role may perform action.
//
// It answers about a stated role. An unknown role and an unknown action are
// both refused: a caller with neither has not been given permission, and
// defaulting either to true would make a typo an escalation. An empty role is
// not stated, so it is refused here too — EffectiveRole is what turns a stored
// row into the role to ask about.
func Allows(role string, action Action) bool {
	switch action {
	case ActionManageTeamMembers, ActionReadAuditTrail, ActionModerateIssueComments:
		return role == model.TeamRoleOwner
	case ActionManageAgents, ActionManageWorkflows, ActionAssignIssueWorkflow:
		return role == model.TeamRoleOwner || role == model.TeamRoleAdmin
	case ActionRunWorkflow, ActionCommentIssue:
		return role == model.TeamRoleOwner || role == model.TeamRoleAdmin || role == model.TeamRoleMember
	default:
		return false
	}
}
