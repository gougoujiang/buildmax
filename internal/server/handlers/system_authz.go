package handlers

import (
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// requireSystemAdmin authorizes a deployment-scoped route.
//
// It is deliberately a sibling of authorizeTeamAction rather than a branch
// inside it. A system grant is not an argument to a team check: an
// administrator reaching a team's issues, artifacts, or traces must pass the
// same membership test as anyone else. Merging the two would make that
// boundary depend on nobody ever passing the grant down — see
// docs/design/system-administration.md section 5.2, and the test that fails if
// this changes.
func (h *Handler) requireSystemAdmin(w http.ResponseWriter, r *http.Request) (userID string, ok bool) {
	// Authentication first, then the store. The order is the opposite of
	// withUserAndStore on purpose: checking the store first would answer an
	// anonymous caller with "system administration not configured", which tells
	// them something about the deployment before they have proved anything
	// about themselves.
	userID, ok = h.requireActiveUser(w, r)
	if !ok {
		return "", false
	}
	if !h.requireStore(w, h.cfg.SystemGrantStore, "system administration not configured") {
		return "", false
	}
	roles, err := h.cfg.SystemGrantStore.ActiveSystemRoles(r.Context(), userID)
	if err != nil {
		// A store failure denies. An authorization check that fails open on a
		// database error is the one bug in this file worth being paranoid
		// about.
		httputil.WriteInternalError(w, err, "handler error", "handler", "require_system_admin", "user_id", userID)
		return "", false
	}
	for _, role := range roles {
		if role == model.SystemRoleAdmin {
			return userID, true
		}
	}

	// A refusal here is recorded with an empty team, because the route was not
	// team-scoped. It is the same action a refused team request writes: a
	// denial is what shows someone probing at a boundary, and which boundary
	// they probed is in the target.
	h.cfg.Audit.Denied(r.Context(), userID, "", deniedRouteName(r))
	// 403 rather than 404. Hiding the existence of /api/admin is not
	// achievable — the routes are in an open-source routes.go and in the Portal
	// bundle — and pretending otherwise would cost a correct status code for no
	// secrecy. What must not leak is data, and this response carries none.
	httputil.WriteJSONError(w, http.StatusForbidden, "forbidden")
	return "", false
}

// systemRoleAdmin keeps admin_system.go from importing the model package for
// one constant.
func systemRoleAdmin() string { return model.SystemRoleAdmin }

// deniedRouteName names the refused route for the audit trail.
//
// The registered pattern rather than the request path: the path carries ids a
// caller chose, and the trail should record which door was tried, not what the
// caller typed into it.
func deniedRouteName(r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}
	return r.Method + " " + r.URL.Path
}
