package team

import (
	"errors"
	"net/http"
	"strings"
	"time"

	coreaudit "github.com/gougoujiang/buildmax/internal/core/audit"
	coreidentity "github.com/gougoujiang/buildmax/internal/core/identity"
	coreteam "github.com/gougoujiang/buildmax/internal/core/team"
	"github.com/gougoujiang/buildmax/internal/server/httputil"
	teamsvc "github.com/gougoujiang/buildmax/internal/service/team"
)

type teamResponse struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	PersonalForUserID *string   `json:"personal_for_user_id,omitempty"`
	CreatedBy         string    `json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type teamMemberResponse struct {
	TeamID    string    `json:"team_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UserName  *string   `json:"user_name,omitempty"`
	UserEmail *string   `json:"user_email,omitempty"`
}

type inviteMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type createTeamRequest struct {
	Name string `json:"name"`
}

// invitationResponse never carries a code: §5.1 of
// docs/design/team-membership-lifecycle.md mints one only for an account
// created by the invitation itself, never for one that already existed, and
// team-scoped invitation only ever targets an account that already exists.
type invitationResponse struct {
	ID        string    `json:"id"`
	TeamID    string    `json:"team_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	InvitedBy string    `json:"invited_by"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

func invitationToResponse(inv coreteam.Invitation) invitationResponse {
	return invitationResponse{
		ID:        inv.ID,
		TeamID:    inv.TeamID,
		UserID:    inv.UserID,
		Role:      inv.Role,
		InvitedBy: inv.InvitedBy,
		ExpiresAt: inv.ExpiresAt,
		CreatedAt: inv.CreatedAt,
	}
}

func invitationsToResponse(list []coreteam.Invitation) []invitationResponse {
	out := make([]invitationResponse, len(list))
	for i := range list {
		out[i] = invitationToResponse(list[i])
	}
	return out
}

func teamToResponse(team coreteam.Team) teamResponse {
	return teamResponse{
		ID:                team.ID,
		Name:              team.Name,
		PersonalForUserID: team.PersonalForUserID,
		CreatedBy:         team.CreatedBy,
		CreatedAt:         team.CreatedAt,
		UpdatedAt:         team.UpdatedAt,
	}
}

func teamMemberToResponse(member coreteam.Member, user *coreidentity.User) teamMemberResponse {
	resp := teamMemberResponse{
		TeamID:    member.TeamID,
		UserID:    member.UserID,
		Role:      member.Role,
		CreatedAt: member.CreatedAt,
	}
	if user != nil {
		if user.Name != "" {
			resp.UserName = &user.Name
		}
		if user.Email != "" {
			resp.UserEmail = &user.Email
		}
	}
	return resp
}

func (h *Handler) listTeamsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	list, err := h.cfg.Teams.ListTeamsByUser(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_teams", "user_id", userID)
		return
	}
	out := make([]teamResponse, len(list))
	for i := range list {
		out[i] = teamToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createTeamHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	var req createTeamRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	team, err := h.cfg.Teams.CreateTeam(r.Context(), name, userID, h.cfg.DefaultQuotaTier)
	if err != nil {
		httputil.WriteInternalError(w, err, "handler error", "handler", "create_team", "user_id", userID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, teamToResponse(*team))
}

func (h *Handler) teamService() *teamsvc.Service {
	return h.teams
}

func newTeamService(cfg Config) *teamsvc.Service {
	return &teamsvc.Service{Teams: cfg.Teams, Users: cfg.Users, LoginCodes: cfg.LoginCodes}
}

func (h *Handler) listTeamMembersHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	teamID, ok := httputil.PathValue(w, r, "team_id")
	if !ok {
		return
	}
	if _, resolvedTeamID, ok := h.guard().ExplicitTeam(w, r, userID, teamID); !ok || resolvedTeamID == "" {
		return
	}
	members, err := h.teamService().ListMembers(r.Context(), teamID)
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_team_members", "user_id", userID, "team_id", teamID)
		return
	}
	out := make([]teamMemberResponse, len(members))
	for i := range members {
		out[i] = teamMemberToResponse(members[i].Membership, members[i].User)
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

// inviteMemberHandler serves POST /api/teams/{team_id}/invitations.
//
// It replaces the old instant-add route outright rather than living beside
// it: per docs/design/team-membership-lifecycle.md §5.1, a second route that
// could also add a member would be the same duplicated authority §1 of that
// document argues against, one layer down.
func (h *Handler) inviteMemberHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	teamID, ok := httputil.PathValue(w, r, "team_id")
	if !ok {
		return
	}
	if _, resolvedTeamID, ok := h.guard().ExplicitTeam(w, r, userID, teamID); !ok || resolvedTeamID == "" {
		return
	}
	var req inviteMemberRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	inv, user, err := h.teamService().InviteMember(r.Context(), teamsvc.InviteMemberCmd{
		TeamID:  teamID,
		ActorID: userID,
		Email:   req.Email,
		Role:    req.Role,
	})
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "invite_team_member", "user_id", userID, "team_id", teamID)
		return
	}
	h.cfg.Audit.UserAction(r.Context(), userID, teamID, coreaudit.TeamMemberInvited, "user", user.ID, inv.Role)
	httputil.WriteJSON(w, http.StatusCreated, invitationToResponse(*inv))
}

// listTeamInvitationsHandler serves GET /api/teams/{team_id}/invitations.
// Reading who has been invited is the same authority as sending or revoking
// an invitation.
func (h *Handler) listTeamInvitationsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	teamID, ok := httputil.PathValue(w, r, "team_id")
	if !ok {
		return
	}
	if _, resolvedTeamID, ok := h.guard().ExplicitTeam(w, r, userID, teamID); !ok || resolvedTeamID == "" {
		return
	}
	invitations, err := h.teamService().ListTeamInvitations(r.Context(), teamID, userID)
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_team_invitations", "user_id", userID, "team_id", teamID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, invitationsToResponse(invitations))
}

// revokeInvitationHandler serves DELETE
// /api/teams/{team_id}/invitations/{invitation_id}.
func (h *Handler) revokeInvitationHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	teamID, ok := httputil.PathValue(w, r, "team_id")
	if !ok {
		return
	}
	invitationID, ok := httputil.PathValue(w, r, "invitation_id")
	if !ok {
		return
	}
	if _, resolvedTeamID, ok := h.guard().ExplicitTeam(w, r, userID, teamID); !ok || resolvedTeamID == "" {
		return
	}
	err := h.teamService().RevokeInvitation(r.Context(), teamsvc.RevokeInvitationCmd{
		TeamID:       teamID,
		InvitationID: invitationID,
		ActorID:      userID,
	})
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "revoke_invitation", "user_id", userID, "team_id", teamID)
		return
	}
	h.cfg.Audit.UserAction(r.Context(), userID, teamID, coreaudit.InvitationRevoked, "team_invitation", invitationID, "")
	w.WriteHeader(http.StatusNoContent)
}

// listMyInvitationsHandler serves GET /api/invitations. It is not
// team-scoped -- it answers "what is pending for me", across every team --
// so unlike every other route in this file it takes no team_id and checks no
// team membership.
func (h *Handler) listMyInvitationsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	invitations, err := h.teamService().ListMyInvitations(r.Context(), userID)
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "list_my_invitations", "user_id", userID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, invitationsToResponse(invitations))
}

// acceptInvitationHandler serves POST /api/invitations/{invitation_id}/accept.
//
// It takes no code: per docs/design/team-membership-lifecycle.md §5.1, the
// caller already reached this session on their own, so this is authorized by
// "this is my own pending row," not by proving anything a second time.
func (h *Handler) acceptInvitationHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	invitationID, ok := httputil.PathValue(w, r, "invitation_id")
	if !ok {
		return
	}
	accepted, err := h.teamService().AcceptInvitation(r.Context(), teamsvc.AcceptInvitationCmd{
		InvitationID: invitationID,
		ActorID:      userID,
	})
	if err != nil {
		// Best-effort: an expired invitation still names a team worth
		// recording against, and a failed lookup here should not turn an
		// already-refused request into an internal error.
		if errors.Is(err, teamsvc.ErrInvitationExpired) {
			if inv, lookupErr := h.cfg.Teams.GetInvitation(r.Context(), invitationID); lookupErr == nil && inv != nil {
				h.cfg.Audit.UserAction(r.Context(), userID, inv.TeamID, coreaudit.InvitationExpired, "team_invitation", invitationID, "")
			}
		}
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "accept_invitation", "user_id", userID)
		return
	}
	h.cfg.Audit.UserAction(r.Context(), userID, accepted.TeamID, coreaudit.InvitationAccepted, "team", accepted.TeamID, accepted.Role)
	httputil.WriteJSON(w, http.StatusOK, invitationToResponse(*accepted))
}

func (h *Handler) removeTeamMemberHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	teamID, ok := httputil.PathValue(w, r, "team_id")
	if !ok {
		return
	}
	targetUserID, ok := httputil.PathValue(w, r, "user_id")
	if !ok {
		return
	}
	if _, resolvedTeamID, ok := h.guard().ExplicitTeam(w, r, userID, teamID); !ok || resolvedTeamID == "" {
		return
	}
	err := h.teamService().RemoveMember(r.Context(), teamsvc.RemoveMemberCmd{
		TeamID:       teamID,
		ActorID:      userID,
		TargetUserID: targetUserID,
	})
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "remove_team_member", "team_id", teamID, "member_user_id", targetUserID)
		return
	}
	h.cfg.Audit.UserAction(r.Context(), userID, teamID, coreaudit.TeamMemberRemoved, "user", targetUserID, "")
	w.WriteHeader(http.StatusNoContent)
}

type setMemberRoleRequest struct {
	Role string `json:"role"`
}

// memberRoleResponse is deliberately smaller than teamMemberResponse: a role
// change reports what changed, not the whole roster row, and building the
// latter would mean a second read this handler has no other reason to make.
type memberRoleResponse struct {
	TeamID string `json:"team_id"`
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

type memberLoginCodeResponse struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// setMemberRoleHandler serves PATCH /api/teams/{team_id}/members/{user_id}.
//
// Setting role to owner is ownership transfer: the caller is demoted to
// admin in the same call, unilaterally and immediately -- see
// docs/design/team-membership-lifecycle.md §5.2-§5.3.
func (h *Handler) setMemberRoleHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	teamID, ok := httputil.PathValue(w, r, "team_id")
	if !ok {
		return
	}
	targetUserID, ok := httputil.PathValue(w, r, "user_id")
	if !ok {
		return
	}
	if _, resolvedTeamID, ok := h.guard().ExplicitTeam(w, r, userID, teamID); !ok || resolvedTeamID == "" {
		return
	}
	var req setMemberRoleRequest
	if !httputil.DecodeJSONBody(w, r, &req) {
		return
	}
	err := h.teamService().SetMemberRole(r.Context(), teamsvc.SetMemberRoleCmd{
		TeamID:       teamID,
		ActorID:      userID,
		TargetUserID: targetUserID,
		Role:         req.Role,
	})
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "set_member_role", "user_id", userID, "team_id", teamID)
		return
	}
	role := strings.TrimSpace(req.Role)
	// Transfer gets its own action distinct from a role change, even though
	// it is one call, so an investigation asking "did ownership ever move"
	// need not infer it from two member_role_changed rows.
	if role == coreteam.RoleOwner {
		h.cfg.Audit.UserAction(r.Context(), userID, teamID, coreaudit.OwnershipTransferred, "user", targetUserID, role)
	} else {
		h.cfg.Audit.UserAction(r.Context(), userID, teamID, coreaudit.MemberRoleChanged, "user", targetUserID, role)
	}
	httputil.WriteJSON(w, http.StatusOK, memberRoleResponse{TeamID: teamID, UserID: targetUserID, Role: role})
}

// issueMemberLoginCodeHandler serves POST
// /api/teams/{team_id}/members/{user_id}/login-code.
//
// This does not replace system-administration.md's system_admin route --
// that one still exists, still works deployment-wide, and is what recovers
// an owner who has no co-owner and no admin left in their own team. This
// route only removes the dependency on a system_admin existing at all for
// the common case of one member locked out of an otherwise healthy team. See
// docs/design/team-membership-lifecycle.md §5.4.
func (h *Handler) issueMemberLoginCodeHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.guard().UserAndStore(w, r, h.cfg.Teams, "teams not configured")
	if !ok {
		return
	}
	teamID, ok := httputil.PathValue(w, r, "team_id")
	if !ok {
		return
	}
	targetUserID, ok := httputil.PathValue(w, r, "user_id")
	if !ok {
		return
	}
	if _, resolvedTeamID, ok := h.guard().ExplicitTeam(w, r, userID, teamID); !ok || resolvedTeamID == "" {
		return
	}
	code, expiresAt, err := h.teamService().IssueMemberLoginCode(r.Context(), teamsvc.IssueMemberLoginCodeCmd{
		TeamID:       teamID,
		ActorID:      userID,
		TargetUserID: targetUserID,
	})
	if err != nil {
		if httputil.WriteServiceError(w, err) {
			return
		}
		httputil.WriteInternalError(w, err, "handler error", "handler", "issue_member_login_code", "user_id", userID, "team_id", teamID)
		return
	}
	// The code itself is never recorded, here or in the trail -- it exists in
	// this response and nowhere else, matching the admin route's own rule.
	h.cfg.Audit.UserAction(r.Context(), userID, teamID, coreaudit.TeamMemberLoginCodeIssued, "user", targetUserID, "")
	httputil.WriteJSON(w, http.StatusOK, memberLoginCodeResponse{Code: code, ExpiresAt: expiresAt})
}
