package handlers

import (
	"context"
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

type teamAction string

const (
	actionManageTeamMembers   teamAction = "manage_team_members"
	actionManageAgents        teamAction = "manage_agents"
	actionManageWorkflows     teamAction = "manage_workflows"
	actionAssignIssueWorkflow teamAction = "assign_issue_workflow"
	actionRunWorkflow         teamAction = "run_workflow"
	actionReadAuditTrail      teamAction = "read_audit_trail"
	actionCommentIssue        teamAction = "comment_issue"
	// actionModerateIssueComments covers deleting a comment the caller did not
	// write. Editing another author's comment is permitted to nobody, so it is
	// not an action here — see internal/service/issue.
	actionModerateIssueComments teamAction = "moderate_issue_comments"
)

func (h *Handler) authorizeTeamAction(w http.ResponseWriter, r *http.Request, userID, teamID string, action teamAction) (string, bool) {
	if !h.requireStore(w, h.cfg.TeamStore, "teams not configured") {
		return "", false
	}
	members, err := h.cfg.TeamStore.ListTeamMembers(r.Context(), teamID)
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
		h.cfg.Audit.Denied(r.Context(), userID, teamID, string(action))
		httputil.WriteJSONError(w, http.StatusForbidden, "forbidden")
		return "", false
	}
	if !isRoleAllowed(role, action) {
		h.cfg.Audit.Denied(r.Context(), userID, teamID, string(action))
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
func (h *Handler) memberAllows(ctx context.Context, userID, teamID string, action teamAction) bool {
	if h.cfg.TeamStore == nil {
		return false
	}
	members, err := h.cfg.TeamStore.ListTeamMembers(ctx, teamID)
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

func isRoleAllowed(role string, action teamAction) bool {
	switch action {
	case actionManageTeamMembers, actionReadAuditTrail, actionModerateIssueComments:
		return role == model.TeamRoleOwner
	case actionManageAgents, actionManageWorkflows, actionAssignIssueWorkflow:
		return role == model.TeamRoleOwner || role == model.TeamRoleAdmin
	case actionRunWorkflow, actionCommentIssue:
		return role == model.TeamRoleOwner || role == model.TeamRoleAdmin || role == model.TeamRoleMember
	default:
		return false
	}
}
