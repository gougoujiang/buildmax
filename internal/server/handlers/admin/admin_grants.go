package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// AdminGrantsResponse lists who can operate the deployment.
type AdminGrantsResponse struct {
	Grants []AdminGrant `json:"grants"`
}

// AdminGrant is one grant with the account it names resolved, so a list is
// readable without a second call per row.
type AdminGrant struct {
	model.SystemGrant
	Email string `json:"email,omitempty"`
}

// AdminGrantRequest is the body for POST /api/admin/grants.
type AdminGrantRequest struct {
	UserID string `json:"user_id"`
	// Role is optional and defaults to system_admin, which is the only role
	// this build accepts. It is in the body so that adding a second role later
	// is not a new route.
	Role string `json:"role,omitempty"`
}

// listAdminGrantsHandler serves GET /api/admin/grants.
func (h *Handler) listAdminGrantsHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.guard().SystemAdmin(w, r); !ok {
		return
	}
	includeRevoked := r.URL.Query().Get("include_revoked") == "true"
	grants, err := h.cfg.Grants.ListSystemGrants(r.Context(), includeRevoked)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_list_grants")
		return
	}
	out := make([]AdminGrant, 0, len(grants))
	for _, g := range grants {
		row := AdminGrant{SystemGrant: g}
		if h.cfg.Users != nil {
			// A grant outliving the account it names is not expected. Showing
			// the row without an email beats refusing to list authority.
			if user, err := h.cfg.Users.GetUser(r.Context(), g.UserID); err == nil && user != nil {
				row.Email = user.Email
			}
		}
		out = append(out, row)
	}
	httputil.WriteJSON(w, http.StatusOK, AdminGrantsResponse{Grants: out})
}

// createAdminGrantHandler serves POST /api/admin/grants.
func (h *Handler) createAdminGrantHandler(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.guard().SystemAdmin(w, r)
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.Users, "accounts not configured") {
		return
	}
	var req AdminGrantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "user_id required")
		return
	}
	role := req.Role
	if role == "" {
		role = model.SystemRoleAdmin
	}
	// Granting does not create an account, for the same reason the operator
	// command does not: creating an account and minting deployment authority
	// are two decisions.
	user, err := h.cfg.Users.GetUser(r.Context(), req.UserID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_create_grant", "user_id", req.UserID)
		return
	}
	if user == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "account not found")
		return
	}

	grant, err := h.cfg.Grants.GrantSystemRole(r.Context(), req.UserID, role, actorID, time.Now().UTC())
	switch {
	case errors.Is(err, model.ErrSystemGrantExists):
		httputil.WriteJSONError(w, http.StatusConflict, "the account already holds this role")
		return
	case errors.Is(err, model.ErrSystemRoleUnknown):
		httputil.WriteJSONError(w, http.StatusBadRequest, "unknown system role")
		return
	case err != nil:
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_create_grant", "user_id", req.UserID)
		return
	}
	h.cfg.Audit.Record(r.Context(), coreaudit.Event{
		ActorType:  coreaudit.ActorUser,
		ActorID:    actorID,
		Action:     coreaudit.SystemAdminGranted,
		TargetType: "user",
		TargetID:   req.UserID,
		Detail:     role,
	})
	httputil.WriteJSON(w, http.StatusCreated, AdminGrant{SystemGrant: *grant, Email: user.Email})
}

// deleteAdminGrantHandler serves DELETE /api/admin/grants/{user_id}.
//
// It refuses to revoke the last active grant. The operator command allows it,
// because that command is the recovery path and its caller already holds the
// database credentials — see docs/design/system-administration.md section 6.
// Nobody should be able to leave a deployment unadministerable by clicking.
func (h *Handler) deleteAdminGrantHandler(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.guard().SystemAdmin(w, r)
	if !ok {
		return
	}
	userID, ok := httputil.PathValue(w, r, "user_id")
	if !ok {
		return
	}
	role := r.URL.Query().Get("role")
	if role == "" {
		role = model.SystemRoleAdmin
	}

	remaining, err := h.cfg.Grants.CountActiveSystemGrants(r.Context(), role)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_delete_grant", "role", role)
		return
	}
	if remaining <= 1 {
		httputil.WriteJSONError(w, http.StatusConflict,
			"this is the deployment's last "+role+"; revoke it with `buildmax-server admin revoke <email>` if that is the intent")
		return
	}

	revoked, err := h.cfg.Grants.RevokeSystemRole(r.Context(), userID, role, time.Now().UTC())
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_delete_grant", "user_id", userID)
		return
	}
	if !revoked {
		httputil.WriteJSONError(w, http.StatusNotFound, "the account does not hold this role")
		return
	}
	h.cfg.Audit.Record(r.Context(), coreaudit.Event{
		ActorType:  coreaudit.ActorUser,
		ActorID:    actorID,
		Action:     coreaudit.SystemAdminRevoked,
		TargetType: "user",
		TargetID:   userID,
		Detail:     role,
	})
	w.WriteHeader(http.StatusNoContent)
}
