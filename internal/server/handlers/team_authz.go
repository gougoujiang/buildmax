package handlers

import (
	"net/http"

	"buildmax/internal/core/model"
	"buildmax/internal/server/httputil"
)

type teamAction string

const (
	actionManageTeamMembers   teamAction = "manage_team_members"
	actionManageAgents        teamAction = "manage_agents"
	actionManageWorkflows     teamAction = "manage_workflows"
	actionAssignIssueWorkflow teamAction = "assign_issue_workflow"
	actionRunWorkflow         teamAction = "run_workflow"
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
		httputil.WriteJSONError(w, http.StatusForbidden, "forbidden")
		return "", false
	}
	if !isRoleAllowed(role, action) {
		httputil.WriteJSONError(w, http.StatusForbidden, "forbidden")
		return "", false
	}
	return role, true
}

func isRoleAllowed(role string, action teamAction) bool {
	switch action {
	case actionManageTeamMembers:
		return role == model.TeamRoleOwner
	case actionManageAgents, actionManageWorkflows, actionAssignIssueWorkflow:
		return role == model.TeamRoleOwner || role == model.TeamRoleAdmin
	case actionRunWorkflow:
		return role == model.TeamRoleOwner || role == model.TeamRoleAdmin || role == model.TeamRoleMember
	default:
		return false
	}
}
