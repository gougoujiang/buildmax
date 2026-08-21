package access

import (
	"context"
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

type Action string

const (
	ActionManageTeamMembers   Action = "manage_team_members"
	ActionManageAgents        Action = "manage_agents"
	ActionManageWorkflows     Action = "manage_workflows"
	ActionAssignIssueWorkflow Action = "assign_issue_workflow"
	ActionRunWorkflow         Action = "run_workflow"
	ActionReadAuditTrail      Action = "read_audit_trail"
	ActionCommentIssue        Action = "comment_issue"
	// ActionModerateIssueComments covers deleting a comment the caller did not
	// write. Editing another author's comment is permitted to nobody, so it is
	// not an action here — see internal/service/issue.
	ActionModerateIssueComments Action = "moderate_issue_comments"
)

func (g *Guard) TeamAction(w http.ResponseWriter, r *http.Request, userID, teamID string, action Action) (string, bool) {
	if !httputil.RequireStore(w, g.Teams, "teams not configured") {
		return "", false
	}
	members, err := g.Teams.ListTeamMembers(r.Context(), teamID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "authorize_team_action", "user_id", userID, "team_id", teamID, "action", string(action))
		return "", false
	}
	role := ""
	for i := range members {
		if members[i].UserID == userID {
			role = members[i].Role
			break
		}
	}
	if role == "" {
		g.denied(r, userID, teamID, string(action))
		httputil.WriteJSONError(w, http.StatusForbidden, "forbidden")
		return "", false
	}
	if !isRoleAllowed(role, action) {
		g.denied(r, userID, teamID, string(action))
		httputil.WriteJSONError(w, http.StatusForbidden, "forbidden")
		return "", false
	}
	return role, true
}

// memberAllows reports whether the caller's role in the team permits the
// action.
//
// Unlike authorizeTeamAction it writes no response and records no denial. It
// answers a question a handler asks before it knows whether the permission is
// needed at all — deleting a comment requires it only when the comment is
// someone else's — so a false here is not a refused request.
func (g *Guard) MemberAllows(ctx context.Context, userID, teamID string, action Action) bool {
	if g.Teams == nil {
		return false
	}
	members, err := g.Teams.ListTeamMembers(ctx, teamID)
	if err != nil {
		return false
	}
	for i := range members {
		if members[i].UserID == userID {
			return isRoleAllowed(members[i].Role, action)
		}
	}
	return false
}

func isRoleAllowed(role string, action Action) bool {
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
