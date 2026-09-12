package access

import (
	"context"
	"net/http"

	corespace "github.com/icloudbb/buildmax/internal/core/space"
	"github.com/icloudbb/buildmax/internal/server/httputil"
)

func (g *Guard) SpaceAction(w http.ResponseWriter, r *http.Request, userID, spaceID string, action corespace.Action) (string, bool) {
	if !httputil.RequireStore(w, g.Spaces, "spaces not configured") {
		return "", false
	}
	members, err := g.Spaces.ListSpaceMembers(r.Context(), spaceID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "authorize_space_action", "user_id", userID, "space_id", spaceID, "action", string(action))
		return "", false
	}
	// EffectiveRoleOf answers "" only when the caller is not in the space at
	// all: EffectiveRole reads a member row with no stored role as a member,
	// never as "".
	role := corespace.EffectiveRoleOf(members, userID)
	if role == "" {
		g.denied(r, userID, spaceID, string(action))
		httputil.WriteJSONError(w, http.StatusForbidden, "forbidden")
		return "", false
	}
	if !corespace.Allows(role, action) {
		g.denied(r, userID, spaceID, string(action))
		httputil.WriteJSONError(w, http.StatusForbidden, "forbidden")
		return "", false
	}
	return role, true
}

// memberAllows reports whether the caller's role in the space permits the
// action.
//
// Unlike authorizeSpaceAction it writes no response and records no denial. It
// answers a question a handler asks before it knows whether the permission is
// needed at all — deleting a comment requires it only when the comment is
// someone else's — so a false here is not a refused request.
func (g *Guard) MemberAllows(ctx context.Context, userID, spaceID string, action corespace.Action) bool {
	if g.Spaces == nil {
		return false
	}
	members, err := g.Spaces.ListSpaceMembers(ctx, spaceID)
	if err != nil {
		return false
	}
	role := corespace.EffectiveRoleOf(members, userID)
	return role != "" && corespace.Allows(role, action)
}
