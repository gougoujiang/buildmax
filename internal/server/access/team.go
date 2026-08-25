package access

import (
	"context"
	"net/http"

	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

func (g *Guard) TeamAction(w http.ResponseWriter, r *http.Request, userID, teamID string, action coreteam.Action) (string, bool) {
	if !httputil.RequireStore(w, g.Teams, "teams not configured") {
		return "", false
	}
	members, err := g.Teams.ListTeamMembers(r.Context(), teamID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "authorize_team_action", "user_id", userID, "team_id", teamID, "action", string(action))
		return "", false
	}
	// EffectiveRole never answers "", so an empty role here means the loop
	// found nobody: this caller is not in the team at all. A member row that
	// never got a role is a member, which is the reading Guard.teamRole has
	// always used and now the only one.
	role := ""
	for i := range members {
		if members[i].UserID == userID {
			role = coreteam.EffectiveRole(members[i].Role)
			break
		}
	}
	if role == "" {
		g.denied(r, userID, teamID, string(action))
		httputil.WriteJSONError(w, http.StatusForbidden, "forbidden")
		return "", false
	}
	if !coreteam.Allows(role, action) {
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
func (g *Guard) MemberAllows(ctx context.Context, userID, teamID string, action coreteam.Action) bool {
	if g.Teams == nil {
		return false
	}
	members, err := g.Teams.ListTeamMembers(ctx, teamID)
	if err != nil {
		return false
	}
	for i := range members {
		if members[i].UserID == userID {
			return coreteam.Allows(coreteam.EffectiveRole(members[i].Role), action)
		}
	}
	return false
}
