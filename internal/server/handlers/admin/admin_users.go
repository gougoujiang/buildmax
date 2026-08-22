package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gougoujiang/buildmax/internal/core/model"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
)

// AdminUser is one account as an administrator sees it.
//
// It is a response struct rather than model.User on purpose. A row struct
// serialized straight out is how a password hash reaches a client, and this is
// the surface where that would matter most — see the secret assertion in
// system_authz_matrix_test.go.
type AdminUser struct {
	ID                string  `json:"id"`
	Email             string  `json:"email"`
	Name              string  `json:"name,omitempty"`
	QuotaTier         string  `json:"quota_tier,omitempty"`
	HasPassword       bool    `json:"has_password"`
	DisabledAt        *int64  `json:"disabled_at,omitempty"`
	LastLoginAt       *int64  `json:"last_login_at,omitempty"`
	LastLoginPlatform *string `json:"last_login_platform,omitempty"`
	CreatedAt         int64   `json:"created_at"`
}

func toAdminUser(u model.User) AdminUser {
	return AdminUser{
		ID:                u.ID,
		Email:             u.Email,
		Name:              u.Name,
		QuotaTier:         u.QuotaTier,
		HasPassword:       u.HasPassword,
		DisabledAt:        u.DisabledAt,
		LastLoginAt:       u.LastLoginAt,
		LastLoginPlatform: u.LastLoginPlatform,
		CreatedAt:         u.CreatedAt,
	}
}

// AdminUsersResponse is a page of accounts.
type AdminUsersResponse struct {
	Users []AdminUser `json:"users"`
	Total int         `json:"total"`
}

// AdminUserDetail adds what an operator needs when acting on one account: which
// teams it can reach, and how many live sessions it has.
type AdminUserDetail struct {
	AdminUser
	Teams []AdminUserTeam `json:"teams"`
	// SessionCount counts live login chains, not tokens. It is what "signed in
	// on two machines" means.
	SessionCount int `json:"session_count"`
	// SystemRoles are the deployment-scoped roles this account holds.
	SystemRoles []string `json:"system_roles"`
}

// AdminUserTeam names a team the account belongs to and its role there. It
// carries no team content — see docs/design/system-administration.md section 7.
type AdminUserTeam struct {
	TeamID string `json:"team_id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

// AdminCreateUserRequest is the body for POST /api/admin/users.
type AdminCreateUserRequest struct {
	Email string `json:"email"`
}

// AdminLoginCodeResponse carries a login code, which is shown once.
type AdminLoginCodeResponse struct {
	Code      string `json:"code"`
	ExpiresAt int64  `json:"expires_at"`
}

// AdminSessionsRevokedResponse reports how many tokens a revocation retired.
type AdminSessionsRevokedResponse struct {
	Revoked int64 `json:"revoked"`
}

// listAdminUsersHandler serves GET /api/admin/users.
func (h *Handler) listAdminUsersHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.guard().SystemAdmin(w, r); !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.Users, "accounts not configured") {
		return
	}
	limit, offset := httputil.LimitOffset(r.URL.Query(), "limit", "offset", httputil.BulkPageDefault, httputil.BulkPageMax)
	users, total, err := h.cfg.Users.ListUsers(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), limit, offset)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_list_users")
		return
	}
	out := make([]AdminUser, 0, len(users))
	for _, u := range users {
		out = append(out, toAdminUser(u))
	}
	httputil.WriteJSON(w, http.StatusOK, AdminUsersResponse{Users: out, Total: total})
}

// getAdminUserHandler serves GET /api/admin/users/{user_id}.
func (h *Handler) getAdminUserHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.guard().SystemAdmin(w, r); !ok {
		return
	}
	user, ok := h.adminTargetUser(w, r)
	if !ok {
		return
	}
	detail := AdminUserDetail{AdminUser: toAdminUser(*user), Teams: []AdminUserTeam{}, SystemRoles: []string{}}

	// Team names and roles, not team contents. An administrator learns that the
	// account can reach a team, never what is in it.
	if h.cfg.Teams != nil {
		teams, err := h.cfg.Teams.ListTeamsByUser(r.Context(), user.ID)
		if err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "admin_get_user", "user_id", user.ID)
			return
		}
		for _, team := range teams {
			role := ""
			members, err := h.cfg.Teams.ListTeamMembers(r.Context(), team.ID)
			if err != nil {
				httputil.WriteInternalError(w, err, "handler error", "handler", "admin_get_user", "team_id", team.ID)
				return
			}
			for _, m := range members {
				if m.UserID == user.ID {
					role = m.Role
					break
				}
			}
			detail.Teams = append(detail.Teams, AdminUserTeam{TeamID: team.ID, Name: team.Name, Role: role})
		}
	}
	if h.cfg.RefreshTokens != nil {
		count, err := h.cfg.RefreshTokens.CountUserSessions(r.Context(), user.ID, time.Now().Unix())
		if err != nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "admin_get_user", "sessions")
			return
		}
		detail.SessionCount = count
	}
	roles, err := h.cfg.Grants.ActiveSystemRoles(r.Context(), user.ID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_get_user", "roles")
		return
	}
	detail.SystemRoles = append(detail.SystemRoles, roles...)

	httputil.WriteJSON(w, http.StatusOK, detail)
}

// createAdminUserHandler serves POST /api/admin/users.
//
// It returns the account and no credential. Issuing a way in is a second,
// separately audited call — the same split `buildmax-server user create` makes,
// for the same reason: creating an account and minting access to it are
// different decisions.
func (h *Handler) createAdminUserHandler(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.guard().SystemAdmin(w, r)
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.Users, "accounts not configured") {
		return
	}
	var req AdminCreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" || !strings.Contains(email, "@") {
		httputil.WriteJSONError(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	user, err := h.cfg.Users.CreateUser(r.Context(), email, h.cfg.DefaultQuotaTier)
	if err != nil {
		if errors.Is(err, model.ErrEmailExists) {
			httputil.WriteJSONError(w, http.StatusConflict, "email already registered")
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_create_user")
		return
	}
	h.recordAdminUserAction(r, actorID, model.AuditUserCreated, user.ID, "")
	httputil.WriteJSON(w, http.StatusCreated, toAdminUser(*user))
}

// issueAdminLoginCodeHandler serves POST /api/admin/users/{user_id}/login-code.
func (h *Handler) issueAdminLoginCodeHandler(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.guard().SystemAdmin(w, r)
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.LoginCodes, "login codes not configured") {
		return
	}
	user, ok := h.adminTargetUser(w, r)
	if !ok {
		return
	}
	// A code for an account that cannot use it would be a way in that opens
	// nothing, and an operator would reasonably read the success as "they can
	// sign in now".
	if user.Disabled() {
		httputil.WriteJSONError(w, http.StatusConflict, "the account is disabled; enable it before issuing a code")
		return
	}
	code, expiresAt, err := h.cfg.LoginCodes.CreateLoginCode(r.Context(), user.ID, model.LoginCodeTTLDefault)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_login_code", "user_id", user.ID)
		return
	}
	h.recordAdminUserAction(r, actorID, model.AuditLoginCodeIssued, user.ID, "")
	// The code itself is never recorded, here or in the trail. The event says
	// one was issued; the plaintext exists in this response and nowhere else.
	httputil.WriteJSON(w, http.StatusOK, AdminLoginCodeResponse{Code: code, ExpiresAt: expiresAt})
}

// setAdminUserDisabledHandler serves the disable and enable routes.
func (h *Handler) setAdminUserDisabledHandler(disable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actorID, ok := h.guard().SystemAdmin(w, r)
		if !ok {
			return
		}
		user, ok := h.adminTargetUser(w, r)
		if !ok {
			return
		}
		// An administrator who disables their own account locks themselves out
		// mid-request and cannot undo it, because the next call is refused
		// too. The recovery would be the operator command, for a mistake that
		// is easy to make and pointless to allow.
		if disable && user.ID == actorID {
			httputil.WriteJSONError(w, http.StatusConflict, "an administrator cannot disable their own account")
			return
		}

		var disabledAt *int64
		action := model.AuditUserEnabled
		if disable {
			now := time.Now().Unix()
			disabledAt = &now
			action = model.AuditUserDisabled
		}
		if err := h.cfg.Users.SetUserDisabled(r.Context(), user.ID, disabledAt); err != nil {
			if errors.Is(err, model.ErrUserNotFound) {
				httputil.WriteJSONError(w, http.StatusNotFound, "account not found")
				return
			}
			httputil.WriteInternalError(w, err, "handler error", "handler", "admin_set_user_disabled", "user_id", user.ID)
			return
		}

		revoked := int64(0)
		if disable && h.cfg.RefreshTokens != nil {
			// The stored half of every login, retired now rather than left to
			// expire. The access token cannot be revoked at all; what stops it
			// is requireActiveUser refusing on the next request.
			n, err := h.cfg.RefreshTokens.RevokeUserSessions(r.Context(), user.ID, time.Now().Unix())
			if err != nil {
				httputil.WriteInternalError(w, err, "handler error", "handler", "admin_set_user_disabled", "revoke_sessions")
				return
			}
			revoked = n
		}
		h.recordAdminUserAction(r, actorID, action, user.ID, "")

		updated, err := h.cfg.Users.GetUser(r.Context(), user.ID)
		if err != nil || updated == nil {
			httputil.WriteInternalError(w, err, "handler error", "handler", "admin_set_user_disabled", "reload")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, struct {
			AdminUser
			SessionsRevoked int64 `json:"sessions_revoked"`
		}{AdminUser: toAdminUser(*updated), SessionsRevoked: revoked})
	}
}

// revokeAdminUserSessionsHandler serves DELETE /api/admin/users/{user_id}/sessions.
func (h *Handler) revokeAdminUserSessionsHandler(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.guard().SystemAdmin(w, r)
	if !ok {
		return
	}
	if !httputil.RequireStore(w, h.cfg.RefreshTokens, "sessions not configured") {
		return
	}
	user, ok := h.adminTargetUser(w, r)
	if !ok {
		return
	}
	n, err := h.cfg.RefreshTokens.RevokeUserSessions(r.Context(), user.ID, time.Now().Unix())
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_revoke_sessions", "user_id", user.ID)
		return
	}
	h.recordAdminUserAction(r, actorID, model.AuditSessionsRevoked, user.ID, "")
	httputil.WriteJSON(w, http.StatusOK, AdminSessionsRevokedResponse{Revoked: n})
}

// adminTargetUser resolves the {user_id} an admin route acts on.
func (h *Handler) adminTargetUser(w http.ResponseWriter, r *http.Request) (*model.User, bool) {
	if !httputil.RequireStore(w, h.cfg.Users, "accounts not configured") {
		return nil, false
	}
	userID, ok := httputil.PathValue(w, r, "user_id")
	if !ok {
		return nil, false
	}
	user, err := h.cfg.Users.GetUser(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "admin_target_user", "user_id", userID)
		return nil, false
	}
	if user == nil {
		httputil.WriteJSONError(w, http.StatusNotFound, "account not found")
		return nil, false
	}
	return user, true
}

// recordAdminUserAction writes an account action taken through the admin API.
//
// Unlike the operator command's events, these name a person: the caller proved
// who they are, so the trail says so rather than naming the binary.
func (h *Handler) recordAdminUserAction(r *http.Request, actorID, action, targetUserID, detail string) {
	h.cfg.Audit.Record(r.Context(), model.AuditEvent{
		ActorType:  model.AuditActorUser,
		ActorID:    actorID,
		Action:     action,
		TargetType: "user",
		TargetID:   targetUserID,
		Detail:     detail,
	})
}
