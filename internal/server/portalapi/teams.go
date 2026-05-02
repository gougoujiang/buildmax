package portalapi

import (
	"net/http"
	"strings"

	"buildmax/internal/core/model"
	"buildmax/internal/server/httputil"
)

type teamResponse struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	PersonalForUserID *string `json:"personal_for_user_id,omitempty"`
	CreatedBy         string  `json:"created_by"`
	CreatedAt         int64   `json:"created_at"`
	UpdatedAt         int64   `json:"updated_at"`
}

type teamMemberResponse struct {
	TeamID    string  `json:"team_id"`
	UserID    string  `json:"user_id"`
	Role      string  `json:"role"`
	CreatedAt int64   `json:"created_at"`
	UserName  *string `json:"user_name,omitempty"`
	UserEmail *string `json:"user_email,omitempty"`
}

type addTeamMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type createTeamRequest struct {
	Name string `json:"name"`
}

func teamToResponse(team model.Team) teamResponse {
	return teamResponse{
		ID:                team.TeamID,
		Name:              team.Name,
		PersonalForUserID: team.PersonalForUserID,
		CreatedBy:         team.CreatedBy,
		CreatedAt:         team.CreatedAt,
		UpdatedAt:         team.UpdatedAt,
	}
}

func teamMemberToResponse(member model.TeamMember, user *model.User) teamMemberResponse {
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
	userID, ok := h.withUserAndStore(w, r, h.cfg.TeamStore, "teams not configured")
	if !ok {
		return
	}
	list, err := h.cfg.TeamStore.ListTeamsByUser(r.Context(), userID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_teams", "user_id", userID)
		return
	}
	out := make([]teamResponse, len(list))
	for i := range list {
		out[i] = teamToResponse(list[i])
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) createTeamHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.withUserAndStore(w, r, h.cfg.TeamStore, "teams not configured")
	if !ok {
		return
	}

	var req createTeamRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "name is required")
		return
	}

	team, err := h.cfg.TeamStore.CreateTeam(r.Context(), name, userID, h.cfg.DefaultQuotaTier)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "create_team", "user_id", userID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, teamToResponse(*team))
}

func (h *Handler) listTeamMembersHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.withUserAndStore(w, r, h.cfg.TeamStore, "teams not configured")
	if !ok {
		return
	}
	teamID, ok := pathValueRequired(w, r, "team_id")
	if !ok {
		return
	}
	if _, resolvedTeamID, ok := h.withExplicitTeam(w, r, userID, teamID); !ok || resolvedTeamID == "" {
		return
	}
	list, err := h.cfg.TeamStore.ListTeamMembers(r.Context(), teamID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_team_members", "user_id", userID, "team_id", teamID)
		return
	}
	out := make([]teamMemberResponse, len(list))
	for i := range list {
		var user *model.User
		if h.cfg.UserStore != nil {
			found, err := h.cfg.UserStore.GetUser(r.Context(), list[i].UserID)
			if err != nil {
				httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_team_members_user", "user_id", userID, "team_id", teamID, "member_user_id", list[i].UserID)
				return
			}
			user = found
		}
		out[i] = teamMemberToResponse(list[i], user)
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) addTeamMemberHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.withUserAndStore(w, r, h.cfg.TeamStore, "teams not configured")
	if !ok {
		return
	}
	if h.cfg.UserStore == nil {
		httputil.WriteJSONError(w, http.StatusInternalServerError, "users not configured")
		return
	}
	teamID, ok := pathValueRequired(w, r, "team_id")
	if !ok {
		return
	}
	if _, resolvedTeamID, ok := h.withExplicitTeam(w, r, userID, teamID); !ok || resolvedTeamID == "" {
		return
	}

	members, err := h.cfg.TeamStore.ListTeamMembers(r.Context(), teamID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_team_members_for_add", "user_id", userID, "team_id", teamID)
		return
	}
	isOwner := false
	for i := range members {
		if members[i].UserID == userID && members[i].Role == model.TeamRoleOwner {
			isOwner = true
			break
		}
	}
	if !isOwner {
		httputil.WriteJSONError(w, http.StatusForbidden, "only team owners can add members")
		return
	}

	var req addTeamMemberRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		httputil.WriteJSONError(w, http.StatusBadRequest, "email is required")
		return
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = model.TeamRoleMember
	}
	if role != model.TeamRoleMember {
		httputil.WriteJSONError(w, http.StatusBadRequest, "only member role is supported")
		return
	}

	user, err := h.cfg.UserStore.UserByEmail(r.Context(), email)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "lookup_team_member_user", "team_id", teamID, "email", email)
		return
	}
	if user == nil {
		httputil.WriteJSONError(w, http.StatusBadRequest, "user does not exist")
		return
	}

	member, err := h.cfg.TeamStore.AddTeamMember(r.Context(), teamID, user.UserID, role)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "add_team_member", "team_id", teamID, "member_user_id", user.UserID)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, teamMemberToResponse(*member, user))
}

func (h *Handler) removeTeamMemberHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.withUserAndStore(w, r, h.cfg.TeamStore, "teams not configured")
	if !ok {
		return
	}
	teamID, ok := pathValueRequired(w, r, "team_id")
	if !ok {
		return
	}
	targetUserID, ok := pathValueRequired(w, r, "user_id")
	if !ok {
		return
	}
	if _, resolvedTeamID, ok := h.withExplicitTeam(w, r, userID, teamID); !ok || resolvedTeamID == "" {
		return
	}

	members, err := h.cfg.TeamStore.ListTeamMembers(r.Context(), teamID)
	if err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "list_team_members_for_remove", "user_id", userID, "team_id", teamID)
		return
	}
	isOwner := false
	for i := range members {
		if members[i].UserID == userID && members[i].Role == model.TeamRoleOwner {
			isOwner = true
			break
		}
	}
	if !isOwner {
		httputil.WriteJSONError(w, http.StatusForbidden, "only team owners can remove members")
		return
	}
	if targetUserID == userID {
		httputil.WriteJSONError(w, http.StatusBadRequest, "owners cannot remove themselves")
		return
	}

	found := false
	for i := range members {
		if members[i].UserID == targetUserID {
			found = true
			break
		}
	}
	if !found {
		httputil.WriteJSONError(w, http.StatusNotFound, "team member not found")
		return
	}

	if err := h.cfg.TeamStore.RemoveTeamMember(r.Context(), teamID, targetUserID); err != nil {
		httputil.WriteInternalError(w, err, "portal handler error", "handler", "remove_team_member", "team_id", teamID, "member_user_id", targetUserID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
