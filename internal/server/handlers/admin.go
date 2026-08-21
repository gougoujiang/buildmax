package handlers

import (
	"net/http"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// AdminMeResponse describes the caller's deployment-scoped authority.
//
// It is what Portal asks before deciding whether an administration area exists
// for this person. A 403 is the expected answer for almost everyone and is not
// an error — hiding the navigation is presentation, and this route is the
// server's half of the same question.
type AdminMeResponse struct {
	UserID string   `json:"user_id"`
	Roles  []string `json:"roles"`
	// Grants are the caller's own active grants, so the area can show when the
	// authority was given and by whom. Other people's grants are not here;
	// that is GET /api/admin/grants.
	Grants []model.SystemGrant `json:"grants"`
}

// adminMeHandler serves GET /api/admin/me.
func (h *Handler) adminMeHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().SystemAdmin(w, r)
	if !ok {
		return
	}
	roles, err := h.cfg.SystemGrantStore.ActiveSystemRoles(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_me", "user_id", userID)
		return
	}
	// Filtering the full active list in memory is fine here and is expected to
	// stay fine: the set of people who can operate a deployment is small by
	// construction, and a per-user query would be a store method with one
	// caller.
	all, err := h.cfg.SystemGrantStore.ListSystemGrants(r.Context(), false)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_me", "user_id", userID)
		return
	}
	mine := make([]model.SystemGrant, 0, 1)
	for _, g := range all {
		if g.UserID == userID {
			mine = append(mine, g)
		}
	}
	httputil.WriteJSON(w, http.StatusOK, AdminMeResponse{UserID: userID, Roles: roles, Grants: mine})
}
